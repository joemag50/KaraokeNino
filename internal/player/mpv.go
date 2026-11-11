package player

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DemoDuration is how long a simulated song "plays" in demo mode.
const DemoDuration = 10 * time.Second

const volumeStep = 5

// MPVOptions configure every playback.
type MPVOptions struct {
	Path           string
	Fullscreen     bool
	NormalizeAudio bool
	Demo           bool
	DemoFor        time.Duration
	Volume         int
	MaxVolume      int
	ExtraArgs      []string
}

// MPV plays songs. With libmpv installed the video is drawn inside the
// KaraokeNino screen (see engine.go): one window, our keys, our messages on
// top. Without it, an external mpv process plays fullscreen and forwards
// the keys back to us over its IPC socket.
type MPV struct {
	opts      MPVOptions
	socket    string
	inputConf string

	// OnKey receives keys pressed while the external mpv has the focus.
	OnKey func(name string)
	// OnProgress receives playback position and duration in seconds.
	OnProgress func(pos, dur float64)
	// OnOSD shows a message over the video (embedded player).
	OnOSD func(text string, d time.Duration)

	mu      sync.Mutex
	eng     *engine
	cmd     *exec.Cmd
	conn    net.Conn
	stop    chan struct{} // closes demo playback early
	volume  int
	paused  bool
	isSmall bool
	audio   bool
	playing bool
	playGen int
}

func New(opts MPVOptions) *MPV {
	if opts.Path == "" {
		opts.Path = "mpv"
	}
	if opts.DemoFor == 0 {
		opts.DemoFor = DemoDuration
	}
	if opts.MaxVolume == 0 {
		opts.MaxVolume = 100
	}
	return &MPV{
		opts:   opts,
		socket: filepath.Join(os.TempDir(), fmt.Sprintf("karaokenino-mpv-%d.sock", os.Getpid())),
		volume: min(max(opts.Volume, 0), opts.MaxVolume),
	}
}

// UseScreen makes videos play inside the program through libmpv. It
// returns an error (and keeps the external player) if libmpv is missing.
func (m *MPV) UseScreen(sink FrameSink) error {
	if m.opts.Demo {
		return fmt.Errorf("modo demo")
	}
	m.mu.Lock()
	opts := m.opts
	opts.Volume = m.volume
	m.mu.Unlock()
	e, err := newEngine(sink, opts)
	if err != nil {
		return err
	}
	e.OnProgress = func(pos, dur float64) {
		if f := m.OnProgress; f != nil {
			f(pos, dur)
		}
	}
	m.mu.Lock()
	m.eng = e
	m.mu.Unlock()
	return nil
}

// Embedded reports whether videos play inside the program.
func (m *MPV) Embedded() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.eng != nil
}

// Play stops whatever is playing and starts it. The returned channel is
// closed when playback ends, either naturally or through Stop.
func (m *MPV) Play(it Item) (<-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = false
	m.isSmall = !m.opts.Fullscreen
	m.audio = it.Audio

	if m.opts.Demo {
		m.stopLocked()
		return m.track(m.demoLocked(it)), nil
	}
	file, err := ResolvePath(it.File)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no está en el disco: %s", it.File)
		}
		return nil, err
	}
	if m.eng != nil {
		done, err := m.eng.play(it, file)
		if err != nil {
			return nil, err
		}
		if !it.Audio {
			m.eng.setFull(!m.isSmall)
		}
		return m.track(done), nil
	}
	m.stopLocked()
	return m.external(it, file)
}

// track marks the song as playing until done closes. A song that ends
// after the next one started must not mark the new one as stopped.
func (m *MPV) track(done <-chan struct{}) <-chan struct{} {
	m.playing = true
	m.playGen++
	gen := m.playGen
	out := make(chan struct{})
	go func() {
		<-done
		m.mu.Lock()
		if m.playGen == gen {
			m.playing = false
		}
		m.mu.Unlock()
		close(out)
	}()
	return out
}

func (m *MPV) demoLocked(it Item) <-chan struct{} {
	log.Printf("[demo] reproducción simulada: %s", it.Label())
	stop := make(chan struct{})
	m.stop = stop
	done := make(chan struct{})
	d := m.opts.DemoFor
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		start := time.Now()
		for {
			select {
			case <-time.After(d - time.Since(start)):
				return
			case <-stop:
				return
			case <-tick.C:
				if f := m.OnProgress; f != nil {
					f(time.Since(start).Seconds(), d.Seconds())
				}
			}
		}
	}()
	return done
}

// diskCacheArgs make mpv read ahead into memory. For local files mpv's
// default (cache=auto) keeps only ~1 s buffered, so any hiccup of the slow
// USB disk (a rescan, NTFS through FUSE, a cover being read) stutters the
// picture. 48 MB is minutes of an AVI/MPG karaoke and fits in 2 GB of RAM;
// mpv_args in config.yaml come later and can override it.
var diskCacheArgs = []string{"--cache=yes", "--demuxer-max-bytes=48MiB", "--demuxer-max-back-bytes=8MiB"}

// external plays with a separate mpv process (fallback without libmpv).
func (m *MPV) external(it Item, file string) (<-chan struct{}, error) {
	if m.inputConf == "" {
		if p, err := writeInputConf(); err == nil {
			m.inputConf = p
		}
	}
	args := []string{
		"--really-quiet", "--no-terminal", "--keep-open=no",
		"--input-ipc-server=" + m.socket,
		fmt.Sprintf("--volume=%d", m.volume),
		fmt.Sprintf("--volume-max=%d", max(m.opts.MaxVolume, 100)),
		"--osd-font-size=42", "--osd-align-y=bottom", "--title=KaraokeNino",
	}
	if m.inputConf != "" {
		args = append(args, "--no-input-default-bindings", "--input-conf="+m.inputConf)
	}
	if m.opts.NormalizeAudio {
		args = append(args, "--af=lavfi=[dynaudnorm=f=250:g=15:p=0.9]")
	}
	if it.Audio {
		args = append(args, "--no-video", "--force-window=no")
	} else {
		args = append(args, "--fullscreen", "--ontop", "--force-window=yes")
	}
	if strings.EqualFold(filepath.Ext(file), ".cdg") {
		if audio, err := ResolvePath(strings.TrimSuffix(file, filepath.Ext(file)) + ".mp3"); err == nil {
			args = append(args, "--audio-file="+audio)
		}
	}
	args = append(args, diskCacheArgs...)
	args = append(args, m.opts.ExtraArgs...)
	args = append(args, "--", file)
	cmd := exec.Command(m.opts.Path, args...)
	dieWithParent(cmd) // if KaraokeNino crashes, mpv must not keep playing alone
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	m.cmd = cmd
	m.playing = true
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.playing = false
		}
		m.mu.Unlock()
		close(done)
	}()
	go m.connect(cmd)
	return done, nil
}

// connect opens the external mpv's IPC socket and listens for events.
func (m *MPV) connect(cmd *exec.Cmd) {
	var conn net.Conn
	for i := 0; i < 50; i++ {
		if c, err := net.DialTimeout("unix", m.socket, 200*time.Millisecond); err == nil {
			conn = c
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if conn == nil {
		log.Printf("mpv ipc: no se pudo conectar a %s", m.socket)
		return
	}
	m.mu.Lock()
	if m.cmd != cmd {
		m.mu.Unlock()
		conn.Close()
		return
	}
	m.conn = conn
	m.mu.Unlock()

	m.send(conn, "observe_property", 1, "time-pos")
	m.send(conn, "observe_property", 2, "duration")
	var pos, dur float64
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var ev struct {
			Event string          `json:"event"`
			Name  string          `json:"name"`
			Data  json.RawMessage `json:"data"`
			Args  []string        `json:"args"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Event {
		case "client-message":
			if len(ev.Args) == 2 && ev.Args[0] == "kn-key" && m.OnKey != nil {
				m.OnKey(ev.Args[1])
			}
		case "property-change":
			var v float64
			if json.Unmarshal(ev.Data, &v) != nil {
				continue
			}
			if ev.Name == "time-pos" {
				pos = v
			} else if ev.Name == "duration" {
				dur = v
			}
			if m.OnProgress != nil {
				m.OnProgress(pos, dur)
			}
		}
	}
}

func (m *MPV) send(conn net.Conn, args ...any) {
	b, _ := json.Marshal(map[string]any{"command": args})
	_ = conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := conn.Write(append(b, '\n')); err != nil {
		log.Printf("mpv ipc: %v", err)
	}
}

func (m *MPV) commandLocked(args ...any) {
	if m.conn != nil {
		m.send(m.conn, args...)
	}
}

func (m *MPV) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *MPV) stopLocked() error {
	if m.stop != nil {
		close(m.stop)
		m.stop = nil
	}
	if m.eng != nil {
		m.eng.stop()
		return nil
	}
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
	if m.cmd == nil || m.cmd.Process == nil {
		m.cmd = nil
		return nil
	}
	err := m.cmd.Process.Kill()
	m.cmd = nil
	return err
}

func (m *MPV) Volume() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.volume
}

func (m *MPV) MaxVolume() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.MaxVolume
}

func (m *MPV) applyVolumeLocked() {
	if m.eng != nil {
		_ = m.eng.p.SetDouble("volume", float64(m.volume))
		return
	}
	m.commandLocked("set_property", "volume", m.volume)
}

// SetMaxVolume changes the cap (admin) and lowers the volume if needed.
func (m *MPV) SetMaxVolume(v int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opts.MaxVolume = min(max(v, 10), 130)
	if m.eng != nil {
		_ = m.eng.p.SetDouble("volume-max", float64(max(m.opts.MaxVolume, 100)))
	}
	if m.volume > m.opts.MaxVolume {
		m.volume = m.opts.MaxVolume
		m.applyVolumeLocked()
	}
	return m.opts.MaxVolume
}

// SetVolume sets the volume (0..MaxVolume) and returns it.
func (m *MPV) SetVolume(v int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volume = min(max(v, 0), m.opts.MaxVolume)
	m.applyVolumeLocked()
	return m.volume
}

// ChangeVolume adds delta steps (±1) and returns the new volume.
func (m *MPV) ChangeVolume(delta int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volume = min(max(m.volume+delta*volumeStep, 0), m.opts.MaxVolume)
	m.applyVolumeLocked()
	return m.volume
}

// TogglePause pauses or resumes the current song and reports the new state.
func (m *MPV) TogglePause() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paused = !m.paused
	if m.eng != nil {
		_ = m.eng.p.SetFlag("pause", m.paused)
	} else {
		m.commandLocked("set_property", "pause", m.paused)
	}
	return m.paused
}

// Paused reports a paused song.
func (m *MPV) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// ShowText puts a message over the video (e.g. "Siguiente: ..."). OnOSD
// gets the text as is; mpv's own OSD gets it without control characters
// (the UI marks keys with them).
func (m *MPV) ShowText(text string, d time.Duration) {
	m.mu.Lock()
	eng := m.eng
	m.commandLocked("show-text", dropControl(text), int(d/time.Millisecond))
	m.mu.Unlock()
	if eng != nil && m.OnOSD != nil {
		m.OnOSD(text, d)
	}
}

// SetSmall shrinks the video into the preview box (true) or makes it
// fullscreen (false). Only the embedded player can shrink.
func (m *MPV) SetSmall(small bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.canShrink() || m.audio || !m.playing || small == m.isSmall {
		return
	}
	m.isSmall = small
	if m.eng != nil {
		m.eng.setFull(!small)
	}
}

// SetSmallIfFull shrinks a fullscreen video and reports whether it did
// (the key that woke the menu is then consumed).
func (m *MPV) SetSmallIfFull() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.canShrink() || m.audio || !m.playing || m.isSmall {
		return false
	}
	m.isSmall = true
	if m.eng != nil {
		m.eng.setFull(false)
	}
	return true
}

// canShrink: the built-in player (or demo mode, which simulates it).
func (m *MPV) canShrink() bool { return m.eng != nil || m.opts.Demo }

// VideoFull reports a video covering the whole screen.
func (m *MPV) VideoFull() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.playing && !m.audio && !m.isSmall
}

// VideoPlaying reports a video (not audio-only) playing.
func (m *MPV) VideoPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.playing && !m.audio
}

func (m *MPV) Available() error {
	if m.Embedded() {
		return nil
	}
	if _, err := exec.LookPath(m.opts.Path); err != nil {
		return fmt.Errorf("no se encontró mpv (%q): instala mpv y libmpv2 (.deb sin internet sirve)", m.opts.Path)
	}
	return nil
}

// KillLeftovers stops mpv processes a previous KaraokeNino left playing
// (e.g. after a crash), so a restart is silent.
func KillLeftovers() {
	if _, err := exec.LookPath("pkill"); err == nil {
		_ = exec.Command("pkill", "-f", "input-ipc-server="+filepath.Join(os.TempDir(), "karaokenino-mpv-")).Run()
	}
}

// Close stops the embedded player.
func (m *MPV) Close() {
	m.mu.Lock()
	e := m.eng
	m.eng = nil
	m.mu.Unlock()
	if e != nil {
		e.close()
	}
}

// mpvKeys maps external-mpv key names to the Fyne key names the UI understands.
var mpvKeys = map[string]string{
	"KP0": "0", "KP1": "1", "KP2": "2", "KP3": "3", "KP4": "4",
	"KP5": "5", "KP6": "6", "KP7": "7", "KP8": "8", "KP9": "9",
	"KP_ENTER": "KP_Enter", "KP_DEC": ".", "KP_DEL": ".",
	"ENTER": "Return", "UP": "Up", "DOWN": "Down", "LEFT": "Left", "RIGHT": "Right",
	"PGUP": "Prior", "PGDWN": "Next", "ESC": "Escape", "BS": "BackSpace", "SPACE": "Space",
	"+": "+", "-": "-", "*": "*", "/": "/", ".": ".",
}

func writeInputConf() (string, error) {
	var b strings.Builder
	b.WriteString("# Generated by KaraokeNino: forward keys to the menu.\n")
	for k, v := range mpvKeys {
		fmt.Fprintf(&b, "%s script-message kn-key %q\n", k, v)
	}
	for c := '0'; c <= '9'; c++ {
		fmt.Fprintf(&b, "%c script-message kn-key %q\n", c, string(c))
	}
	for c := 'a'; c <= 'z'; c++ {
		fmt.Fprintf(&b, "%c script-message kn-key %q\n", c, string(c-'a'+'A'))
		fmt.Fprintf(&b, "%c script-message kn-key %q\n", c-'a'+'A', string(c-'a'+'A'))
	}
	p := filepath.Join(os.TempDir(), fmt.Sprintf("karaokenino-input-%d.conf", os.Getpid()))
	return p, os.WriteFile(p, []byte(b.String()), 0o644)
}

func dropControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' {
			return -1
		}
		return r
	}, s)
}
