package player

import (
	"fmt"
	"image"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/josegamboa/karaokenino/internal/mpvlib"
)

// FrameSink is the part of the screen that shows the video.
type FrameSink interface {
	// Frame shows a new picture. It runs on the render goroutine; the image
	// stays valid until two more frames were delivered.
	Frame(img *image.RGBA)
	// Target is the pixel size available for the video (full or preview).
	Target(full bool) (w, h int)
	// SetFull switches between the whole screen and the preview box.
	SetFull(full bool)
	// Show shows or hides the video area.
	Show(show bool)
}

// engine plays through libmpv and draws the video inside KaraokeNino.
type engine struct {
	p    *mpvlib.Player
	sink FrameSink

	OnProgress func(pos, dur float64)

	mu      sync.Mutex
	pending []chan struct{} // one per loadfile, closed on its END_FILE
	full    bool
	drawn   int       // frames drawn of the current video
	started time.Time // when the current file was loaded
	isVideo bool
	// mpv renders into buf (C memory, only this engine touches it); each
	// frame is then copied into Go memory for the screen. The screen may
	// still be drawing an older picture, so those are never freed by us —
	// Go's garbage collector frees them once nothing uses them.
	buf   *mpvlib.Frame
	pics  [3]*image.RGBA
	next  int
	quit  chan struct{}
	loops sync.WaitGroup // events, render and watchdog goroutines
}

// newEngine starts libmpv. extra are "--key=value" options from the config.
func newEngine(sink FrameSink, opts MPVOptions) (*engine, error) {
	o := [][2]string{
		{"vo", "libmpv"},
		{"hwdec", "no"}, // the software renderer needs frames in memory
		{"sw-fast", "yes"},
		{"osd-level", "0"}, // KaraokeNino draws its own messages over the video
		{"idle", "yes"},
		{"keep-open", "no"},
		{"terminal", "no"},
		{"input-default-bindings", "no"},
		{"input-vo-keyboard", "no"},
		{"audio-display", "no"}, // MP3 cover art: we show our own cover
		{"volume", fmt.Sprint(opts.Volume)},
		{"volume-max", fmt.Sprint(max(opts.MaxVolume, 100))},
	}
	if opts.NormalizeAudio {
		o = append(o, [2]string{"af", "lavfi=[dynaudnorm=f=250:g=15:p=0.9]"})
	}
	for _, a := range diskCacheArgs {
		o = append(o, parseArg(a))
	}
	for _, a := range opts.ExtraArgs {
		o = append(o, parseArg(a))
	}
	p, err := mpvlib.New(o)
	if err != nil {
		return nil, err
	}
	e := &engine{p: p, sink: sink, full: opts.Fullscreen, quit: make(chan struct{})}
	_ = p.Observe(1, "time-pos", mpvlib.FormatDouble)
	_ = p.Observe(2, "duration", mpvlib.FormatDouble)
	e.loops.Add(3)
	go e.events()
	go e.renderLoop()
	go e.watchdog()
	return e, nil
}

// parseArg turns "--hwdec=auto", "--no-osc" or "--fs" into an mpv option.
func parseArg(a string) [2]string {
	a = strings.TrimPrefix(a, "--")
	if k, v, ok := strings.Cut(a, "="); ok {
		return [2]string{k, v}
	}
	if k, ok := strings.CutPrefix(a, "no-"); ok {
		return [2]string{k, "no"}
	}
	return [2]string{a, "yes"}
}

func (e *engine) play(it Item, file string) (<-chan struct{}, error) {
	vid := "auto"
	if it.Audio {
		vid = "no"
	}
	_ = e.p.SetString("vid", vid)
	done := make(chan struct{})
	e.mu.Lock()
	e.pending = append(e.pending, done)
	e.drawn = 0
	e.started = time.Now()
	e.isVideo = !it.Audio
	e.mu.Unlock()
	log.Printf("reproductor: empieza «%s» (%s)", it.Label(), map[bool]string{true: "audio", false: "video"}[it.Audio])
	if err := e.p.Command("loadfile", file, "replace"); err != nil {
		e.mu.Lock()
		e.pending = e.pending[:len(e.pending)-1]
		e.mu.Unlock()
		return nil, err
	}
	if it.Audio {
		e.sink.Show(false) // music: the menu stays, with the cover and "Ahora suena"
	} else {
		e.sink.SetFull(e.isFull())
		e.sink.Show(true)
	}
	return done, nil
}

func (e *engine) stop() {
	_ = e.p.Command("stop")
}

func (e *engine) isFull() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.full
}

func (e *engine) setFull(full bool) {
	e.mu.Lock()
	e.full = full
	e.mu.Unlock()
	e.sink.SetFull(full)
}

// events handles end of songs, progress and video size.
func (e *engine) events() {
	defer e.loops.Done()
	var pos, dur float64
	for {
		ev := e.p.WaitEvent(-1)
		switch ev.ID {
		case mpvlib.EventShutdown:
			return
		case mpvlib.EventEndFile:
			if ev.Reason == mpvlib.EndError {
				log.Printf("reproductor: no se pudo reproducir (error %d)", ev.Err)
			}
			e.mu.Lock()
			var done chan struct{}
			if len(e.pending) > 0 {
				done = e.pending[0]
				e.pending = e.pending[1:]
			}
			idle := len(e.pending) == 0
			e.mu.Unlock()
			if idle {
				e.sink.Show(false)
			}
			pos, dur = 0, 0
			if done != nil {
				close(done)
			}
		case mpvlib.EventPropertyChange:
			switch ev.Name {
			case "time-pos":
				pos = ev.Double
			case "duration":
				dur = ev.Double
			}
			if (ev.Name == "time-pos" || ev.Name == "duration") && e.OnProgress != nil {
				e.OnProgress(pos, dur)
			}
		}
	}
}

// renderLoop draws each new video frame at the size the screen needs:
// the video's own size (karaoke videos are small) or less, never more —
// the screen scales it up, which costs nothing on the CPU.
func (e *engine) renderLoop() {
	defer e.loops.Done()
	t := time.NewTicker(8 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-e.quit:
			return
		case <-t.C:
		}
		if !e.p.FrameReady() {
			continue
		}
		// Ask mpv for the size every frame: change notifications are not
		// sent when two videos in a row have the same size.
		dw, ok1 := e.p.GetInt("dwidth")
		dh, ok2 := e.p.GetInt("dheight")
		if !ok1 || !ok2 || dw <= 0 || dh <= 0 {
			continue
		}
		vw, vh := int(dw), int(dh)
		e.mu.Lock()
		full := e.full
		e.mu.Unlock()
		tw, th := e.sink.Target(full)
		w, h := fitSize(vw, vh, tw, th)
		if w <= 0 || h <= 0 {
			continue
		}
		f := e.buffer(w, h)
		if f == nil {
			continue
		}
		if err := e.p.Render(f); err != nil {
			continue
		}
		e.sink.Frame(e.copyFrame(f, w, h))
		e.mu.Lock()
		e.drawn++
		first := e.drawn == 1
		e.mu.Unlock()
		if first {
			log.Printf("reproductor: primer cuadro del video %dx%d (se dibuja a %dx%d)", vw, vh, w, h)
		}
	}
}

// watchdog warns in the log when a video plays without pictures.
func (e *engine) watchdog() {
	defer e.loops.Done()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	warned := time.Time{}
	for {
		select {
		case <-e.quit:
			return
		case <-t.C:
		}
		e.mu.Lock()
		bad := e.isVideo && len(e.pending) > 0 && e.drawn == 0 && time.Since(e.started) > 4*time.Second && warned != e.started
		if bad {
			warned = e.started
		}
		e.mu.Unlock()
		if bad {
			dw, _ := e.p.GetInt("dwidth")
			dh, _ := e.p.GetInt("dheight")
			log.Printf("reproductor: AVISO el video suena pero no llegan cuadros (tamaño %dx%d)", dw, dh)
		}
	}
}

// buffer returns the C render buffer, reallocating it on size change.
// Only the render loop uses it, so freeing the old one is safe.
func (e *engine) buffer(w, h int) *mpvlib.Frame {
	if e.buf != nil && (e.buf.W != w || e.buf.H != h) {
		e.buf.Free()
		e.buf = nil
	}
	if e.buf == nil {
		e.buf = mpvlib.NewFrame(w, h)
	}
	return e.buf
}

// copyFrame copies the rendered frame into the next Go-owned picture,
// making every pixel opaque ("rgb0": the 4th byte is garbage).
func (e *engine) copyFrame(f *mpvlib.Frame, w, h int) *image.RGBA {
	pic := e.pics[e.next]
	if pic == nil || pic.Rect.Dx() != w || pic.Rect.Dy() != h {
		pic = image.NewRGBA(image.Rect(0, 0, w, h))
		e.pics[e.next] = pic
	}
	e.next = (e.next + 1) % len(e.pics)
	row := w * 4
	for y := 0; y < h; y++ {
		dst := pic.Pix[y*pic.Stride : y*pic.Stride+row]
		copy(dst, f.Pix[y*f.Stride:y*f.Stride+row])
		for i := 3; i < row; i += 4 {
			dst[i] = 255
		}
	}
	return pic
}

// fitSize scales the video (vw×vh) down to fit tw×th keeping its shape.
// The width is rounded to 16 pixels so rows stay 64-byte aligned.
func fitSize(vw, vh, tw, th int) (int, int) {
	if tw <= 0 || th <= 0 {
		tw, th = vw, vh
	}
	s := min(float64(tw)/float64(vw), float64(th)/float64(vh), 1)
	w := int(float64(vw)*s) &^ 15
	h := int(float64(vh) * float64(w) / float64(vw))
	return max(w, 16), max(h, 2)
}

// close shuts mpv down in order: stop drawing, ask mpv to quit, wait for
// both loops to leave libmpv, then free it.
func (e *engine) close() {
	close(e.quit)
	_ = e.p.Command("quit")
	stopped := make(chan struct{})
	go func() { e.loops.Wait(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		log.Printf("reproductor: mpv no terminó a tiempo")
		return // leak rather than free memory a loop may still use
	}
	e.p.Destroy()
	e.buf.Free()
	e.buf = nil
}
