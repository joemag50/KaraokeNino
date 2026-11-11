package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/josegamboa/karaokenino/internal/catalog"
)

// Version is shown on the welcome screen (set by main).
var Version = "dev"

// ---------- layers: full-screen screens under the dialogs ----------

// showLayer puts a full-screen screen (welcome, mode choice) over the main
// screen. Dialogs still open on top of it, and Esc still reaches the admin.
func (a *App) showLayer(o *overlay) {
	if a.layer != nil {
		a.root.Remove(a.layer.root)
	}
	a.layer = o
	a.root.Add(o.root)
	if a.ov != nil { // keep the dialog on top
		a.root.Remove(a.ov.root)
		a.root.Add(a.ov.root)
	}
	a.updateCountdown()
}

func (a *App) closeLayer() {
	if a.layer == nil {
		return
	}
	if a.layer.onClose != nil {
		a.layer.onClose()
	}
	a.root.Remove(a.layer.root)
	a.layer = nil
	a.resetIdle()
	a.rearmVideoFullscreen()
}

// ---------- welcome + loading ----------

type splashScreen struct {
	*overlay
	a                 *App
	lay               *scaledLayout
	status, detail    *canvas.Text
	bars              []*canvas.Rectangle
	barArea           []Rect
	bg                *partyBackground
	fx                *fxLoop
	shownAt           time.Time
	diskDone, scanned bool
}

func (a *App) newSplash() *splashScreen {
	dw, dh := designW, designH
	pal := &a.pal
	s := &splashScreen{a: a, lay: newScaledLayout(dw, dh)}
	var objs []fyne.CanvasObject
	add := func(o ...fyne.CanvasObject) { objs = append(objs, o...) }
	cx := func(t *canvas.Text) *canvas.Text { t.Alignment = fyne.TextAlignCenter; return t }

	s.bg = newPartyBackground(s.lay, pal)
	add(s.bg.objects...)

	// Equalizer bars above the name: cheap animation that says "working".
	const nBars = 9
	bw, gap, maxH := 22, 12, 110
	x0 := dw/2 - (nBars*bw+(nBars-1)*gap)/2
	baseY := dh*36/100 + maxH
	for i := 0; i < nBars; i++ {
		r := newPanel(pal.Accent, color.NRGBA{}, 6)
		if i%2 == 1 {
			r.FillColor = pal.Accent2
		}
		area := Rect{Left: x0 + i*(bw+gap), Top: baseY - maxH, Width: bw, Height: maxH}
		add(s.lay.place(r, area))
		s.bars = append(s.bars, r)
		s.barArea = append(s.barArea, area)
	}
	s.moveBars(0.4) // a still equalizer when animations are off

	name := cx(newText("KaraokeNino", pal.Text, true))
	add(s.lay.placeText(name, Rect{Left: 0, Top: dh * 52 / 100, Width: dw, Height: 96}, 88))
	line := canvas.NewLinearGradient(pal.Accent, pal.Accent2, 90)
	add(s.lay.place(line, Rect{Left: dw/2 - 260, Top: dh*52/100 + 108, Width: 520, Height: 4}))
	tag := "La rockola de tu fiesta"
	if b := strings.TrimSpace(a.Cfg.Business.Name); b != "" && !strings.EqualFold(b, "KaraokeNino") {
		tag = b
	}
	sub := cx(newText(tag, pal.TextDim, false))
	add(s.lay.placeText(sub, Rect{Left: 0, Top: dh*52/100 + 122, Width: dw, Height: 36}, 28))
	if w := a.welcomeText(); w != "" {
		welcome := cx(newText(fit(w, dw-80, 30), pal.Accent2, true))
		add(s.lay.placeText(welcome, Rect{Left: 0, Top: dh*52/100 + 166, Width: dw, Height: 40}, 30))
	}

	s.status = cx(newText("Abriendo el catálogo…", pal.Text, true))
	add(s.lay.placeText(s.status, Rect{Left: 0, Top: dh - 96, Width: dw, Height: 30}, 22))
	s.detail = cx(newText("", pal.TextDim, false))
	add(s.lay.placeText(s.detail, Rect{Left: 0, Top: dh - 62, Width: dw, Height: 24}, 17))
	ver := newText("v "+Version, withAlpha(pal.TextDim, 150), false)
	ver.Alignment = fyne.TextAlignTrailing
	add(s.lay.placeText(ver, Rect{Left: dw - 300, Top: dh - 30, Width: 280, Height: 20}, 13))

	s.overlay = &overlay{root: container.New(s.lay, objs...)}
	// Keys are ignored while loading (Esc still opens the admin menu).
	s.key = func(fyne.KeyName, string) {}
	s.onClose = func() { s.fx.Stop() }
	return s
}

// moveBars sets the equalizer for time t (s).
func (s *splashScreen) moveBars(t float64) {
	for i, b := range s.bars {
		area := s.barArea[i]
		h := 0.25 + 0.75*math.Abs(math.Sin(t*2.2+float64(i)*0.7))
		hh := int(float64(area.Height) * h)
		s.lay.move(b, Rect{Left: area.Left, Top: area.Y2() - hh, Width: area.Width, Height: hh})
	}
}

func (s *splashScreen) show() {
	s.shownAt = time.Now()
	s.a.showLayer(s.overlay)
	if !s.a.fxOn {
		return
	}
	// Equalizer and party background at fxFPS: says "working" without
	// taking the CPU the catalog needs.
	start := time.Now()
	s.fx = startFXLoop(func(now time.Time) bool {
		if !s.visible() {
			s.fx.Stop()
			return false
		}
		s.moveBars(now.Sub(start).Seconds())
		s.bg.frame(now)
		s.bg.repaint()
		return true
	})
}

func (s *splashScreen) visible() bool { return s.a.layer == s.overlay }

func (s *splashScreen) setStatus(msg, detail string) {
	if !s.visible() {
		return
	}
	s.status.Text = msg
	s.detail.Text = detail
	s.status.Refresh()
	s.detail.Refresh()
}

// checkReady leaves the welcome screen once the catalog can be used: the
// disk was found (or we gave up looking), an empty catalog finished its
// first scan, and the logo was on screen for a moment.
func (a *App) checkReady() {
	s := a.splash
	if s == nil || !s.visible() {
		return
	}
	wait := a.splashMin - time.Since(s.shownAt)
	if wait > 0 {
		time.AfterFunc(wait, func() { fyne.Do(a.checkReady) })
		return
	}
	if !s.diskDone {
		return
	}
	if len(a.genres) == 0 && a.diskOK && !s.scanned {
		return // empty catalog: the first scan is building it
	}
	a.closeLayer()
	if len(a.genres) == 0 {
		a.flash("El catálogo está vacío: conecta el disco de música o importa Musica.data")
		return
	}
	a.openModes()
}

// ---------- mode choice: karaoke / videos / music ----------

type modeCard struct {
	kind  catalog.Kind
	title string
	desc  string
	color color.NRGBA
	count int
}

// openModes asks what the guest wants to do. With only one kind of songs on
// the disk it goes straight to it.
func (a *App) openModes() {
	counts, _ := a.Store.KindCounts(a.ctx)
	all := []modeCard{
		{catalog.KindKaraoke, "KARAOKE", "Canta con la letra en pantalla", a.pal.Karaoke, counts[catalog.KindKaraoke]},
		{catalog.KindVideo, "VIDEOS", "Videos musicales", a.pal.Video, counts[catalog.KindVideo]},
		{catalog.KindAudio, "MÚSICA", "Canciones para escuchar y bailar", a.pal.Audio, counts[catalog.KindAudio]},
	}
	var cards []modeCard
	for _, c := range all {
		if c.count > 0 {
			cards = append(cards, c)
		}
	}
	if len(cards) <= 1 {
		if len(cards) == 1 {
			a.setMode(cards[0].kind)
		}
		return
	}

	dw, dh := designW, designH
	pal := &a.pal
	lay := newScaledLayout(dw, dh)
	var objs []fyne.CanvasObject
	add := func(o ...fyne.CanvasObject) { objs = append(objs, o...) }
	cx := func(t *canvas.Text) *canvas.Text { t.Alignment = fyne.TextAlignCenter; return t }

	bg := newPartyBackground(lay, pal)
	add(bg.objects...)
	brand := cx(newText("KaraokeNino", pal.Accent2, true))
	add(lay.placeText(brand, Rect{Top: dh * 7 / 100, Width: dw, Height: 34}, 26))
	q := cx(newText("¿Qué quieres hacer?", pal.Text, true))
	add(lay.placeText(q, Rect{Top: dh * 15 / 100, Width: dw, Height: 64}, 54))

	cw, ch, gap := 340, 330, 40
	x0 := dw/2 - (len(cards)*cw+(len(cards)-1)*gap)/2
	top := dh * 32 / 100
	type cardObjs struct {
		bg, band *canvas.Rectangle
		title    *canvas.Text
		area     Rect
	}
	var co []cardObjs
	for i, c := range cards {
		r := Rect{Left: x0 + i*(cw+gap), Top: top, Width: cw, Height: ch}
		bg := newPanel(pal.Panel, pal.PanelBorder, 22)
		bg.StrokeWidth = 4
		band := newPanel(c.color, color.NRGBA{}, 4)
		add(lay.place(bg, r))
		add(lay.place(band, Rect{Left: r.Left + 18, Top: r.Top + 18, Width: r.Width - 36, Height: 8}))
		t := cx(newText(c.title, pal.Text, true))
		add(lay.placeText(t, Rect{Left: r.Left, Top: r.Top + 80, Width: r.Width, Height: 64}, 54))
		d := cx(newText(c.desc, pal.TextDim, false))
		add(lay.placeText(d, Rect{Left: r.Left, Top: r.Top + 170, Width: r.Width, Height: 28}, 20))
		n := cx(newText(songCount(c.count), c.color, true))
		add(lay.placeText(n, Rect{Left: r.Left, Top: r.Top + 240, Width: r.Width, Height: 30}, 24))
		co = append(co, cardObjs{bg, band, t, r})
	}
	hint := newKeyHint(pal, Rect{Top: dh - 72, Width: dw, Height: 32}, 20, pal.TextDim, fyne.TextAlignCenter)
	hint.set(key("4") + key("6") + " elegir   ·   " + key("Intro") + " entrar   ·   " + key("1") + " volver a esta pantalla")
	add(hint.place(lay))

	sel := 0
	for i, c := range cards {
		if c.kind == a.mode {
			sel = i
		}
	}
	render := func() {
		for i, c := range co {
			r := c.area
			if i == sel {
				r = grow(c.area, 10)
				c.bg.FillColor = withAlpha(cards[i].color, 70)
				c.bg.StrokeColor = cards[i].color
			} else {
				c.bg.FillColor = pal.Panel
				c.bg.StrokeColor = pal.PanelBorder
			}
			lay.move(c.bg, r)
			lay.move(c.band, Rect{Left: r.Left + 18, Top: r.Top + 18, Width: r.Width - 36, Height: 8})
			c.bg.Refresh()
		}
	}
	o := &overlay{root: container.New(lay, objs...)}
	if a.fxOn {
		// Party background, and the chosen card's border breathes.
		var fx *fxLoop
		fx = startFXLoop(func(now time.Time) bool {
			if a.layer != o {
				fx.Stop()
				return false
			}
			bg.frame(now)
			pulse := 0.5 + 0.5*math.Sin(2*math.Pi*now.Sub(bg.t0).Seconds()/1.6)
			co[sel].bg.StrokeColor = withAlpha(cards[sel].color, uint8(150+105*pulse))
			bg.repaint()
			return true
		})
		o.onClose = fx.Stop
	}
	o.key = func(name fyne.KeyName, action string) {
		switch action {
		case "left", "up":
			sel = (sel - 1 + len(cards)) % len(cards)
		case "right", "down":
			sel = (sel + 1) % len(cards)
		case "ok":
			a.closeLayer()
			a.setMode(cards[sel].kind)
			return
		}
		render()
	}
	render()
	a.showLayer(o)
}

// setMode filters the browser to karaoke, videos or music.
func (a *App) setMode(k catalog.Kind) {
	a.mode = k
	if err := a.loadGenres(); err != nil {
		a.flash(err.Error())
		return
	}
	a.flash(modeName(k) + " · " + key("1") + " para cambiar")
}

func modeName(k catalog.Kind) string {
	switch k {
	case catalog.KindKaraoke:
		return "Karaoke"
	case catalog.KindVideo:
		return "Videos"
	case catalog.KindAudio:
		return "Música"
	}
	return "Todo"
}

func songCount(n int) string {
	if n == 1 {
		return "1 canción"
	}
	return thousands(n) + " canciones"
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
