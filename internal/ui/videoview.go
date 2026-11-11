package ui

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// videoView is the built-in player's picture: part of the KaraokeNino
// screen, not a separate window. Fullscreen it covers everything and shows
// our own messages on top; small it sits in the preview box while guests
// pick the next song. The keypad always reaches us.
type videoView struct {
	a    *App
	lay  *scaledLayout
	root *fyne.Container

	bg            *canvas.Rectangle
	img           *canvas.Image
	topBg, botBg  *canvas.LinearGradient
	title, singer *canvas.Text
	osd           *keyHint
	hint          *keyHint
	progress      *progressBar // hairline at the bottom edge

	mu       sync.Mutex
	full     bool
	shown    bool
	target   [2][2]int // [small, full] pixel sizes
	latest   *image.RGBA
	pending  atomic.Bool
	hideOSD  *time.Timer
	hideHint *time.Timer
}

func newVideoView(a *App) *videoView {
	v := &videoView{a: a, lay: newScaledLayout(designW, designH)}
	pal := &a.pal
	v.bg = canvas.NewRectangle(color.Black)
	v.img = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 16, 9)))
	v.img.FillMode = canvas.ImageFillContain
	v.img.ScaleMode = canvas.ImageScaleFastest
	v.topBg = canvas.NewLinearGradient(color.NRGBA{0, 0, 0, 200}, color.NRGBA{}, 0)
	v.botBg = canvas.NewLinearGradient(color.NRGBA{}, color.NRGBA{0, 0, 0, 220}, 0)
	v.title = newText("", pal.Text, true)
	v.singer = newText("", pal.Accent2, true)
	v.osd = newKeyHint(pal, Rect{40, designH - 118, designW - 80, 40}, 34, pal.Text, fyne.TextAlignCenter)
	v.hint = newKeyHint(pal, Rect{40, designH - 54, designW - 80, 30}, 20, pal.TextDim, fyne.TextAlignCenter)

	v.lay.placeFull(v.bg)
	v.lay.placeFull(v.img)
	v.lay.place(v.topBg, Rect{0, 0, designW, 110})
	v.lay.placeText(v.title, Rect{40, 22, designW - 80, 40}, 34)
	v.lay.placeText(v.singer, Rect{40, 62, designW - 80, 30}, 24)
	v.lay.place(v.botBg, Rect{0, designH - 150, designW, 150})
	v.osd.place(v.lay)
	v.hint.place(v.lay)
	// Over the video the progress is a faint hairline on the very bottom
	// edge: always there (the singer sees how much is left) but too thin and
	// translucent to hide the lyrics. It moves at most once per design pixel.
	v.progress = newProgressBar(v.lay, pal, Rect{0, designH - 4, designW, 4})
	v.progress.bg.FillColor = color.NRGBA{255, 255, 255, 22}
	v.progress.fill.FillColor = withAlpha(pal.Accent2, 150)

	v.root = container.New(v.lay, v.bg, v.img, v.topBg, v.title, v.singer, v.botBg, v.osd.root, v.hint.root,
		v.progress.bg, v.progress.fill)
	v.root.Hide()
	return v
}

// ---------- player.FrameSink (called from the player goroutines) ----------

func (v *videoView) Frame(img *image.RGBA) {
	v.mu.Lock()
	v.latest = img
	v.mu.Unlock()
	// One screen update at a time: if the screen is busy, the next frame
	// simply replaces this one (no backlog, no lag).
	if !v.pending.CompareAndSwap(false, true) {
		return
	}
	fyne.Do(func() {
		v.mu.Lock()
		img := v.latest
		v.mu.Unlock()
		v.pending.Store(false)
		if img != nil && v.shown {
			v.img.Image = img
			v.img.Refresh()
		}
	})
}

func (v *videoView) Target(full bool) (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	t := v.target[0]
	if full {
		t = v.target[1]
	}
	return t[0], t[1]
}

func (v *videoView) SetFull(full bool) {
	fyne.Do(func() { v.setFull(full) })
}

func (v *videoView) Show(show bool) {
	fyne.Do(func() {
		if show == v.shown {
			return
		}
		v.shown = show
		if show {
			v.root.Show()
			v.setFull(v.full)
		} else {
			v.root.Hide()
			v.a.scr.root.Show()
		}
		v.root.Refresh()
	})
}

// ---------- screen side ----------

// setFull places the video over the whole screen or in the preview box.
func (v *videoView) setFull(full bool) {
	v.mu.Lock()
	v.full = full
	v.mu.Unlock()
	for _, o := range []fyne.CanvasObject{v.topBg, v.title, v.singer, v.osd.root, v.hint.root} {
		if full {
			o.Show()
		} else {
			o.Hide()
		}
	}
	v.progress.show(full)
	v.updateShade()
	v.bg.Hidden = !full
	if full {
		v.lay.setFull(v.img)
		// Nothing of the menu shows behind a fullscreen video: don't draw it.
		// (Only while a video is on screen: music keeps the menu visible.)
		if v.shown {
			v.a.scr.root.Hide()
			v.showHint()
		}
	} else {
		v.lay.move(v.img, previewRect)
		v.a.scr.root.Show()
	}
	v.root.Refresh()
}

// setTargets stores the pixel sizes of the preview box and the screen.
func (v *videoView) setTargets(small, full [2]int) {
	v.mu.Lock()
	v.target = [2][2]int{small, full}
	v.mu.Unlock()
}

// startSong shows the song title over the video.
func (v *videoView) startSong(title, singer string) {
	v.title.Text = fit(title, designW-80, 34)
	v.singer.Text = singer
	v.title.Refresh()
	v.singer.Refresh()
	v.showHint()
}

// message shows a banner ("Siguiente: …", volume) for d.
func (v *videoView) message(text string, d time.Duration) {
	v.osd.set(firstLine(text))
	v.updateShade()
	if v.hideOSD != nil {
		v.hideOSD.Stop()
	}
	v.hideOSD = time.AfterFunc(d, func() {
		fyne.Do(func() {
			v.osd.set("")
			v.updateShade()
		})
	})
}

// fullHint tells the singer how to reach the list without stopping the song.
var fullHint = key("7") + " ver la lista y pedir otra canción   ·   " + key("+") + key("−") + " volumen   ·   " + key("*") + " pausa   ·   " + key(".") + " saltar"

// updateShade darkens the bottom of the video only while a message or the
// hint is there, so it never sits over the lyrics for the whole song.
func (v *videoView) updateShade() {
	v.mu.Lock()
	full := v.full
	v.mu.Unlock()
	hide := !full || (v.osd.Text == "" && v.hint.Text == "")
	if v.botBg.Hidden != hide {
		v.botBg.Hidden = hide
		v.botBg.Refresh()
	}
}

// setProgress moves the bottom hairline (frac 0..1).
func (v *videoView) setProgress(frac float64) {
	if v == nil {
		return
	}
	v.progress.set(frac)
}

// hintFor is how long the fullscreen hint stays.
var hintFor = 6 * time.Second

// showHint shows how to reach the list for a few seconds.
func (v *videoView) showHint() {
	v.hint.set(fullHint)
	v.updateShade()
	if v.hideHint != nil {
		v.hideHint.Stop()
	}
	v.hideHint = time.AfterFunc(hintFor, func() {
		fyne.Do(func() {
			v.hint.set("")
			v.updateShade()
		})
	})
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			// Our messages are "▶ title\nSiguiente: …": the title is already
			// shown at the top, keep the second part.
			return s[i+1:]
		}
	}
	return s
}
