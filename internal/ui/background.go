package ui

import (
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// The party background: a static gradient with three layers of light on top
// that drift at different speeds (parallax) —
//
//	far:  big soft colored lights (blobs) that drift and breathe
//	mid:  two tall stage-light beams that sweep left and right
//	near: bokeh bubbles that rise, and (per theme) tiny twinkling sparkles
//
// It is built to be cheap on an old PC with software OpenGL:
//
//   - Nothing is rasterized per frame. The soft lights are one 64×64 image
//     per color made once (softDisc), scaled up by the GPU; moving them keeps
//     their texture (same size, no Refresh). Fading uses the image alpha,
//     which is a shader uniform. Bubbles and sparkles are canvas.Circle,
//     drawn by a shader from their color: no texture at all.
//   - A frame only moves ~30 objects and refreshes one invisible rectangle
//     (poke) so Fyne repaints; no gradient is ever regenerated.
//   - It runs at fxFPS (10) and stops while a video plays, while another
//     screen covers it, and when config ui.animations is false.
//
// See BenchmarkPartyFrame for the CPU cost of a frame.

// Animation constants (the far lights use Palette.GlowAlpha).
// TestPaletteContrast uses them to find the brightest background behind
// text, so text stays readable at any moment.
const (
	fxFPS       = 10
	beamPeak    = 0.13 // alpha at the center of a beam
	bubbleFill  = 0.16 // alpha of a bubble's fill
	bubbleRing  = 0.34 // alpha of a bubble's edge
	sparklePeak = 0.9  // alpha of a sparkle at its brightest (3-6 px dots)
	nBubbles    = 14
)

// softDisc is a round light that fades to transparent at its edge, made
// once per color (128×128: 64 KB; the GPU scales it to any size). The
// falloff is a gaussian that reaches zero at the edge, so scaled up it has
// no visible rim or steps.
func softDisc(c color.NRGBA, peak float64) image.Image {
	key := discKey{c, peak}
	discMu.Lock()
	defer discMu.Unlock()
	if img, ok := discCache[key]; ok {
		return img
	}
	const n = 128
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := (float64(x) + 0.5 - n/2) / (n / 2)
			dy := (float64(y) + 0.5 - n/2) / (n / 2)
			d := dx*dx + dy*dy
			if d >= 1 {
				continue
			}
			f := math.Exp(-3.5*d) * (1 - d*d) // 1 at the center, 0 at the edge
			img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(255 * peak * f)})
		}
	}
	discCache[key] = img
	return img
}

type discKey struct {
	c    color.NRGBA
	peak float64
}

var (
	discMu    sync.Mutex
	discCache = map[discKey]image.Image{}
)

// bgSprite is a soft light drifting around a home position.
type bgSprite struct {
	img        *canvas.Image
	w, h       int     // design size (constant: the texture is kept)
	cx, cy     float64 // home center
	ax, ay     float64 // drift amplitude
	px, py     float64 // drift periods (s)
	phase      float64
	breathe    float64 // fade period (s)
	depth      float64 // parallax depth: how much it follows the focus shift
	minVisible float64 // lowest alpha while breathing
}

// bgBubble is a bokeh bubble rising from the bottom.
type bgBubble struct {
	c              *canvas.Circle
	col            color.NRGBA
	r              int
	x0, speed      float64
	sway, swayP, p float64
}

// bgSparkle is a tiny dot that twinkles somewhere new each time.
type bgSparkle struct {
	c      *canvas.Circle
	col    color.NRGBA
	period float64
	phase  float64
	seed   uint32
	r      int
}

type partyBackground struct {
	lay     *scaledLayout
	pal     *Palette
	objects []fyne.CanvasObject

	glows    []*bgSprite
	beams    []*bgSprite
	bubbles  []*bgBubble
	sparkles []*bgSparkle
	poke     *canvas.Rectangle

	shift, shiftTarget float64 // parallax shift (design px), eased
	t0                 time.Time
}

// newPartyBackground builds the background objects (bottom of the screen's
// object list). The static gradient is always drawn; the lights stay still
// until frame is called.
func newPartyBackground(lay *scaledLayout, pal *Palette) *partyBackground {
	b := &partyBackground{lay: lay, pal: pal, t0: time.Now()}
	add := func(o fyne.CanvasObject) { b.objects = append(b.objects, o) }
	add(lay.placeFull(canvas.NewRectangle(color.Black)))
	add(lay.placeFull(canvas.NewLinearGradient(pal.Bg1, pal.Bg2, 0)))

	sprite := func(c color.NRGBA, peak float64, s bgSprite) *bgSprite {
		sp := s
		sp.img = canvas.NewImageFromImage(softDisc(c, peak))
		sp.img.FillMode = canvas.ImageFillStretch
		sp.img.ScaleMode = canvas.ImageScaleSmooth
		add(lay.place(sp.img, Rect{int(sp.cx) - sp.w/2, int(sp.cy) - sp.h/2, sp.w, sp.h}))
		return &sp
	}
	// Far: three big lights, slow.
	b.glows = []*bgSprite{
		sprite(pal.Glow[0], pal.GlowAlpha, bgSprite{w: 820, h: 680, cx: 230, cy: 170, ax: 170, ay: 90, px: 47, py: 61, phase: 0, breathe: 9, depth: 0.25, minVisible: 0.55}),
		sprite(pal.Glow[1], pal.GlowAlpha, bgSprite{w: 900, h: 720, cx: 1060, cy: 560, ax: 190, ay: 110, px: 53, py: 43, phase: 2, breathe: 11, depth: 0.3, minVisible: 0.55}),
		sprite(pal.Glow[2], pal.GlowAlpha, bgSprite{w: 700, h: 600, cx: 700, cy: 260, ax: 260, ay: 140, px: 67, py: 39, phase: 4, breathe: 13, depth: 0.2, minVisible: 0.4}),
	}
	// Mid: two stage beams sweeping across.
	b.beams = []*bgSprite{
		sprite(pal.Glow[1], beamPeak, bgSprite{w: 360, h: 1100, cx: 640, cy: 300, ax: 620, ay: 0, px: 29, py: 1, phase: 0, breathe: 7, depth: 0.6, minVisible: 0.5}),
		sprite(pal.Glow[0], beamPeak, bgSprite{w: 300, h: 1000, cx: 640, cy: 360, ax: 600, ay: 0, px: 37, py: 1, phase: 3, breathe: 5, depth: 0.6, minVisible: 0.5}),
	}
	// Near: bokeh bubbles in the theme's light colors.
	cols := pal.Glow[:]
	for i := 0; i < nBubbles; i++ {
		h := hash32(uint32(i) * 2654435761)
		bb := &bgBubble{
			col:   cols[i%len(cols)],
			r:     10 + int(h%30),
			x0:    float64(40 + (i*1237)%1200),
			speed: 9 + float64(h>>8%14),
			sway:  10 + float64(h>>16%30),
			swayP: 6 + float64(h>>20%7),
			p:     float64(h>>4%1000) / 1000,
		}
		bb.c = canvas.NewCircle(withAlpha(bb.col, alpha(bubbleFill)))
		bb.c.StrokeColor = withAlpha(bb.col, alpha(bubbleRing))
		bb.c.StrokeWidth = 1.5
		add(lay.place(bb.c, Rect{int(bb.x0), int(bb.p * designH), 2 * bb.r, 2 * bb.r}))
		b.bubbles = append(b.bubbles, bb)
	}
	// Glitter (boda, xv) or confetti (infantil): tiny dots that twinkle.
	for i := 0; i < pal.Sparkles; i++ {
		sp := &bgSparkle{col: pal.Accent2, period: 2.2 + float64(i%5)*0.6, phase: float64(i) * 0.37, seed: uint32(i)*7919 + 1, r: 2 + i%3}
		switch {
		case pal.light(): // confetti: every color of the theme
			sp.col = []color.NRGBA{pal.Accent, pal.Accent2, pal.Karaoke, pal.Video}[i%4]
		case i%3 == 0:
			sp.col = rgb(255, 255, 255)
		}
		sp.c = canvas.NewCircle(color.NRGBA{})
		sp.c.Hidden = true
		add(lay.place(sp.c, Rect{0, 0, 2 * sp.r, 2 * sp.r}))
		b.sparkles = append(b.sparkles, sp)
	}
	b.poke = canvas.NewRectangle(color.Transparent)
	add(lay.place(b.poke, Rect{0, 0, 0, 0}))
	b.frame(b.t0) // a pleasant still picture even without animations
	return b
}

// parallax shifts the layers sideways (focus moved to another column):
// near layers move more than far ones.
func (b *partyBackground) parallax(px float64) { b.shiftTarget = px }

// frame places every light for time now. It only moves objects and changes
// their colors; the caller asks Fyne to repaint with b.repaint.
func (b *partyBackground) frame(now time.Time) {
	t := now.Sub(b.t0).Seconds()
	b.shift += (b.shiftTarget - b.shift) * 0.18
	tau := 2 * math.Pi
	for _, layer := range [][]*bgSprite{b.glows, b.beams} {
		for _, s := range layer {
			x := s.cx + s.ax*math.Sin(tau*t/s.px+s.phase) - b.shift*s.depth
			y := s.cy + s.ay*math.Sin(tau*t/s.py+s.phase*1.3)
			b.lay.move(s.img, Rect{int(x) - s.w/2, int(y) - s.h/2, s.w, s.h})
			f := s.minVisible + (1-s.minVisible)*(0.5+0.5*math.Sin(tau*t/s.breathe+s.phase))
			s.img.Translucency = 1 - f
		}
	}
	span := float64(designH + 120)
	for _, bb := range b.bubbles {
		y := designH + 60 - math.Mod(t*bb.speed+bb.p*span, span)
		x := bb.x0 + bb.sway*math.Sin(tau*t/bb.swayP+bb.p*tau) - b.shift
		b.lay.move(bb.c, Rect{int(x) - bb.r, int(y) - bb.r, 2 * bb.r, 2 * bb.r})
		tw := 0.65 + 0.35*math.Sin(tau*t/(bb.swayP*0.7)+bb.p*9)
		bb.c.FillColor = withAlpha(bb.col, alpha(bubbleFill*tw))
		bb.c.StrokeColor = withAlpha(bb.col, alpha(bubbleRing*tw))
	}
	for _, sp := range b.sparkles {
		cycle := t/sp.period + sp.phase
		n := uint32(cycle)
		v := math.Sin(math.Pi * (cycle - math.Floor(cycle)))
		v = v * v * v * v * v * v // short bright twinkle
		h := hash32(sp.seed + n*2246822519)
		x, y := int(h%designW), int((h>>12)%designH)
		b.lay.move(sp.c, Rect{x - sp.r, y - sp.r, 2 * sp.r, 2 * sp.r})
		sp.c.FillColor = withAlpha(sp.col, alpha(sparklePeak*v))
		sp.c.Hidden = v < 0.02
	}
}

// repaint asks Fyne to draw the window again without freeing any texture:
// only the invisible poke rectangle is refreshed.
func (b *partyBackground) repaint() { canvas.Refresh(b.poke) }

// alpha converts an opacity 0..1 to a color alpha.
func alpha(f float64) uint8 { return uint8(255*min(max(f, 0), 1) + 0.5) }

func hash32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

// ---------- the animation loop ----------

// fxLoop calls frame about fxFPS times a second on the Fyne thread. frame
// reports whether it animated; while it does not (a video is playing,
// another screen covers it) the loop only checks 4 times a second.
type fxLoop struct {
	stop    chan struct{}
	once    sync.Once
	stopped atomic.Bool
}

func startFXLoop(frame func(now time.Time) bool) *fxLoop {
	l := &fxLoop{stop: make(chan struct{})}
	go func() {
		var busy, idle atomic.Bool
		every := time.Second / fxFPS
		t := time.NewTimer(every)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
			}
			// One frame at a time: if the screen is busy, skip this one.
			if busy.CompareAndSwap(false, true) {
				fyne.Do(func() {
					defer busy.Store(false)
					if !l.stopped.Load() {
						idle.Store(!frame(time.Now()))
					}
				})
			}
			if idle.Load() {
				t.Reset(250 * time.Millisecond)
			} else {
				t.Reset(every)
			}
		}
	}()
	return l
}

func (l *fxLoop) Stop() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		l.stopped.Store(true)
		close(l.stop)
	})
}

// ---------- the main screen ----------

// startFX starts the main screen's animation (Run calls it when
// ui.animations is on). It pauses by itself whenever the screen is not
// what the guests see or a video needs the CPU.
func (a *App) startFX() {
	if !a.fxOn || a.scr == nil || a.fxLoop != nil {
		return
	}
	a.fxLoop = startFXLoop(func(now time.Time) bool {
		if !a.browseFXVisible() {
			return false
		}
		a.scr.animate(now)
		return true
	})
}

// browseFXVisible: the main screen is on and no video is playing (in the
// preview box or fullscreen) and no full-screen layer covers it.
func (a *App) browseFXVisible() bool {
	if a.layer != nil || (a.idle != nil && a.idle.visible) {
		return false
	}
	return !a.MPV.VideoPlaying()
}

// animate draws one frame of the main screen: background, the glow of the
// selected rows, keypad pops and the queue sparkle.
func (s *browseScreen) animate(now time.Time) {
	s.bg.parallax(float64(s.a.focus-colArtists) * 70)
	s.bg.frame(now)
	t := now.Sub(s.bg.t0).Seconds()
	for _, l := range []*rowList{s.letters, s.artists, s.songs} {
		l.animate(now, t)
	}
	s.keypad.animate(now)
	s.bg.repaint()
}

// celebrate plays the "added to the queue" sparkle on the selected song.
func (a *App) celebrate() {
	if a.scr == nil || a.ov != nil || a.layer != nil {
		return
	}
	a.scr.songs.celebrate(time.Now(), a.fxOn && a.browseFXVisible())
}
