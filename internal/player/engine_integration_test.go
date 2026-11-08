//go:build mpv

package player

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeY4M writes an uncompressed test video (no encoder needed).
func writeY4M(t *testing.T, path string, w, h, frames int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	fmt.Fprintf(bw, "YUV4MPEG2 W%d H%d F25:1 Ip A1:1 C420jpeg\n", w, h)
	y := make([]byte, w*h)
	uv := make([]byte, w*h/4)
	for i := range uv {
		uv[i] = 128
	}
	for n := 0; n < frames; n++ {
		for i := range y {
			y[i] = byte((i + n*4) % 256)
		}
		bw.WriteString("FRAME\n")
		bw.Write(y)
		bw.Write(uv)
		bw.Write(uv)
	}
	bw.Flush()
}

type fakeSink struct {
	mu     sync.Mutex
	frames int
	size   image.Point
	shown  bool
	full   bool
}

func (s *fakeSink) Frame(img *image.RGBA) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames++
	s.size = img.Rect.Size()
	if img.Pix[3] != 255 {
		panic("pixel no opaco")
	}
}
func (s *fakeSink) Target(full bool) (int, int) {
	if full {
		return 1280, 720
	}
	return 274, 154
}
func (s *fakeSink) SetFull(full bool) { s.mu.Lock(); s.full = full; s.mu.Unlock() }
func (s *fakeSink) Show(show bool)    { s.mu.Lock(); s.shown = show; s.mu.Unlock() }

func TestEmbeddedPlayerDrawsFramesInsideTheProgram(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Karaoke.Y4M")
	writeY4M(t, video, 640, 480, 50) // 2 s, like an SD karaoke video

	m := New(MPVOptions{Fullscreen: true, MaxVolume: 100, Volume: 50, ExtraArgs: []string{"--ao=null"}})
	sink := &fakeSink{}
	if err := m.UseScreen(sink); err != nil {
		t.Fatalf("libmpv: %v", err)
	}
	defer m.Close()
	var progress int
	var pmu sync.Mutex
	m.OnProgress = func(pos, dur float64) { pmu.Lock(); progress++; pmu.Unlock() }

	done, err := m.Play(Item{Title: "x", File: filepath.Join(dir, "karaoke.y4m")}) // case differs on purpose
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	sink.mu.Lock()
	full, shown := sink.full, sink.shown
	sink.mu.Unlock()
	if !full || !shown || !m.VideoFull() {
		t.Fatalf("debe empezar en pantalla completa: full=%v shown=%v", full, shown)
	}
	// A key shrinks it into the preview box; frames then come at that size.
	if !m.SetSmallIfFull() {
		t.Fatal("no se achicó")
	}
	time.Sleep(500 * time.Millisecond)
	sink.mu.Lock()
	small := sink.size
	sink.mu.Unlock()
	if small.X > 274 || small.Y > 154 || small.X < 16 {
		t.Fatalf("cuadro en recuadro de %v", small)
	}
	if v := m.ChangeVolume(20); v != 100 {
		t.Fatalf("volumen %d", v)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("la canción no terminó")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.frames < 20 {
		t.Fatalf("solo %d cuadros", sink.frames)
	}
	if sink.shown {
		t.Fatal("el video debe ocultarse al terminar")
	}
	pmu.Lock()
	defer pmu.Unlock()
	if progress == 0 {
		t.Fatal("sin progreso")
	}
	if m.VideoPlaying() {
		t.Fatal("sigue marcado como reproduciendo")
	}
}

func TestEmbeddedPlayerSkipAndAudio(t *testing.T) {
	dir := t.TempDir()
	writeY4M(t, filepath.Join(dir, "a.y4m"), 320, 240, 250) // 10 s
	writeWAV(t, filepath.Join(dir, "b.wav"), 1)
	m := New(MPVOptions{Fullscreen: true, MaxVolume: 100, Volume: 50, ExtraArgs: []string{"--ao=null"}})
	sink := &fakeSink{}
	if err := m.UseScreen(sink); err != nil {
		t.Fatalf("libmpv: %v", err)
	}
	defer m.Close()
	j := NewJukebox(m, Options{})
	j.Add(Item{Title: "video", File: filepath.Join(dir, "a.y4m")})
	j.Add(Item{Title: "audio", File: filepath.Join(dir, "b.wav"), Audio: true})
	time.Sleep(500 * time.Millisecond)
	j.Next() // skip the long video
	time.Sleep(300 * time.Millisecond)
	if cur, _ := j.Snapshot(); cur == nil || cur.Title != "audio" {
		t.Fatalf("después de saltar suena %+v", cur)
	}
	sink.mu.Lock()
	shown := sink.shown
	sink.mu.Unlock()
	if shown || m.VideoFull() {
		t.Fatal("con música no debe verse la pantalla de video")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cur, _ := j.Snapshot(); cur == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("la cola no terminó")
}

// slowSink reads every picture later, like the screen painting it after
// the player already moved on. Freed or reused memory would crash here.
type slowSink struct {
	fakeSink
	reads chan *image.RGBA
}

func (s *slowSink) Frame(img *image.RGBA) {
	s.fakeSink.Frame(img)
	select {
	case s.reads <- img:
	default:
	}
}

func TestEmbeddedPlayerSurvivesResizesWhileScreenReads(t *testing.T) {
	dir := t.TempDir()
	writeY4M(t, filepath.Join(dir, "v.y4m"), 640, 480, 100) // 4 s
	m := New(MPVOptions{Fullscreen: true, MaxVolume: 100, Volume: 50, ExtraArgs: []string{"--ao=null"}})
	sink := &slowSink{reads: make(chan *image.RGBA, 64)}
	if err := m.UseScreen(sink); err != nil {
		t.Fatalf("libmpv: %v", err)
	}
	defer m.Close()
	stop := make(chan struct{})
	var sum int
	go func() {
		for {
			select {
			case <-stop:
				return
			case img := <-sink.reads:
				time.Sleep(40 * time.Millisecond) // the screen is slow
				for _, b := range img.Pix {
					sum += int(b)
				}
			}
		}
	}()
	done, err := m.Play(Item{Title: "v", File: filepath.Join(dir, "v.y4m")})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ { // full ↔ preview, fast
		m.SetSmall(i%2 == 0)
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("no terminó")
	}
	close(stop)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.frames < 30 {
		t.Fatalf("solo %d cuadros", sink.frames)
	}
}

// Two videos of the same size in a row (most karaokes are 640×480): the
// second one must be seen, not only heard.
func TestSecondVideoOfSameSizeIsDrawn(t *testing.T) {
	dir := t.TempDir()
	writeY4M(t, filepath.Join(dir, "a.y4m"), 640, 480, 25) // 1 s
	writeY4M(t, filepath.Join(dir, "b.y4m"), 640, 480, 75) // 3 s
	m := New(MPVOptions{Fullscreen: true, MaxVolume: 100, Volume: 50, ExtraArgs: []string{"--ao=null"}})
	sink := &fakeSink{}
	if err := m.UseScreen(sink); err != nil {
		t.Fatalf("libmpv: %v", err)
	}
	defer m.Close()
	j := NewJukebox(m, Options{})
	started := make(chan string, 2)
	j.OnStart = func(cur Item, _ *Item) { started <- cur.Title }
	j.Add(Item{Title: "a", File: filepath.Join(dir, "a.y4m")})
	j.Add(Item{Title: "b", File: filepath.Join(dir, "b.y4m")})
	<-started
	select {
	case s := <-started:
		if s != "b" {
			t.Fatalf("empezó %q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no empezó el segundo video")
	}
	sink.mu.Lock()
	before := sink.frames
	sink.mu.Unlock()
	time.Sleep(1500 * time.Millisecond)
	sink.mu.Lock()
	after, shown := sink.frames, sink.shown
	sink.mu.Unlock()
	if after-before < 15 || !shown {
		t.Fatalf("el segundo video no se ve: %d cuadros nuevos, visible=%v", after-before, shown)
	}
}
