package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// idleScreen is the attract screen shown when nobody touches the keypad:
// welcome message, business name/contact/logo and rotating slides
// ("Lo más pedido", how to request a song, what is playing). Any key hides it.
type idleScreen struct {
	a       *App
	lay     *scaledLayout
	root    *fyne.Container
	visible bool

	bg                      *partyBackground
	welcome, brand, contact *canvas.Text
	slideTitle              *canvas.Text
	lines                   []*keyHint
	logo                    *canvas.Image
	footer                  *canvas.Text
	slide                   int
	ticker                  *time.Ticker
	stop                    chan struct{}
	fx                      *fxLoop
}

func newIdleScreen(a *App) *idleScreen {
	dw, dh := designW, designH
	pal := &a.pal
	s := &idleScreen{a: a, lay: newScaledLayout(dw, dh)}
	var objs []fyne.CanvasObject
	add := func(o ...fyne.CanvasObject) { objs = append(objs, o...) }
	cx := func(t *canvas.Text) *canvas.Text { t.Alignment = fyne.TextAlignCenter; return t }

	s.bg = newPartyBackground(s.lay, pal)
	add(s.bg.objects...)

	s.logo = canvas.NewImageFromResource(nil)
	s.logo.FillMode = canvas.ImageFillContain
	add(s.lay.place(s.logo, Rect{Left: dw/2 - 90, Top: dh * 6 / 100, Width: 180, Height: 110}))
	s.brand = cx(newText("", pal.Text, true))
	add(s.lay.placeText(s.brand, Rect{Left: 40, Top: dh * 22 / 100, Width: dw - 80, Height: 60}, 54))
	s.welcome = cx(newText("", pal.Accent2, true))
	add(s.lay.placeText(s.welcome, Rect{Left: 40, Top: dh * 33 / 100, Width: dw - 80, Height: 50}, 40))

	line := canvas.NewLinearGradient(pal.Accent, pal.Accent2, 90)
	add(s.lay.place(line, Rect{Left: dw/2 - 240, Top: dh*43/100 - 6, Width: 480, Height: 3}))
	s.slideTitle = cx(newText("", pal.Accent2, true))
	add(s.lay.placeText(s.slideTitle, Rect{Left: 40, Top: dh * 45 / 100, Width: dw - 80, Height: 40}, 32))
	for i := 0; i < 6; i++ {
		t := newKeyHint(pal, Rect{Left: 60, Top: dh*53/100 + i*38, Width: dw - 120, Height: 36}, 26, pal.Text, fyne.TextAlignCenter)
		add(t.place(s.lay))
		s.lines = append(s.lines, t)
	}
	s.contact = cx(newText("", pal.TextDim, true))
	add(s.lay.placeText(s.contact, Rect{Left: 40, Top: dh - 92, Width: dw - 80, Height: 34}, 26))
	s.footer = cx(newText("Presiona cualquier tecla para elegir tu canción", pal.TextDim, false))
	add(s.lay.placeText(s.footer, Rect{Left: 40, Top: dh - 50, Width: dw - 80, Height: 28}, 20))

	s.root = container.New(s.lay, objs...)
	return s
}

func (s *idleScreen) show() {
	if s.visible {
		return
	}
	s.visible = true
	a := s.a
	brand := a.Cfg.Business.Name
	if brand == "" {
		brand = "KaraokeNino"
	}
	s.brand.Text = fit(brand, designW-80, 54)
	s.welcome.Text = fit(a.welcomeText(), designW-80, 40)
	s.contact.Text = a.Cfg.Business.Contact
	if a.Cfg.Business.Logo != "" && fileExists(a.Cfg.Business.Logo) {
		s.logo.File = a.Cfg.Business.Logo
		s.logo.Hidden = false
	} else {
		s.logo.Hidden = true
	}
	for _, t := range []*canvas.Text{s.brand, s.welcome, s.contact} {
		t.Refresh()
	}
	s.logo.Refresh()
	s.slide = 0
	s.render()
	a.root.Add(s.root)

	// The party background moves while the idle screen is on (the main
	// screen's animation pauses under it).
	if a.fxOn {
		s.fx = startFXLoop(func(now time.Time) bool {
			if !s.visible || a.MPV.VideoPlaying() {
				return false
			}
			s.bg.frame(now)
			s.bg.repaint()
			return true
		})
	}

	every := time.Duration(max(a.Cfg.Idle.RotateSeconds, 3)) * time.Second
	s.ticker = time.NewTicker(every)
	s.stop = make(chan struct{})
	go func(t *time.Ticker, stop chan struct{}) {
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				fyne.Do(func() {
					if s.visible {
						s.slide++
						s.render()
					}
				})
			}
		}
	}(s.ticker, s.stop)
}

func (s *idleScreen) hide() {
	if !s.visible {
		return
	}
	s.visible = false
	s.fx.Stop()
	s.fx = nil
	if s.ticker != nil {
		s.ticker.Stop()
		close(s.stop)
	}
	s.a.root.Remove(s.root)
	s.a.resetIdle()
}

// render shows the current slide.
func (s *idleScreen) render() {
	a := s.a
	var title string
	var lines []string
	slides := 3
	switch s.slide % slides {
	case 0:
		title = "Lo más pedido"
		for i, r := range a.topSongs(6) {
			lines = append(lines, fmt.Sprintf("%d. %s — %s", i+1, r.Title, r.Artist))
		}
		if len(lines) == 0 {
			title = "¡Sé el primero en cantar!"
			lines = []string{"Elige la letra, el artista y la canción",
				"con " + key("8") + key("2") + " para subir y bajar, y " + key("4") + key("6") + " para cambiar de columna",
				"Presiona " + key("Intro") + " para agregarla a la cola"}
		}
	case 1:
		title = "¿Cómo pedir una canción?"
		lines = []string{
			key("8") + key("2") + " subir y bajar   ·   " + key("4") + key("6") + " cambiar de columna",
			"Letra  →  Artista  →  Canción   ·   " + key("Intro") + " agregar a la cola",
			key("0") + " pedir por número del catálogo",
			key("/") + " ver la cola   ·   " + key("7") + " lo más pedido   ·   " + key("1") + " Karaoke · Videos · Música",
			key("+") + key("−") + " volumen   ·   " + key("*") + " pausa",
		}
	case 2:
		cur, q := a.box.Snapshot()
		title = "Sonando ahora"
		switch {
		case cur == nil && len(q) > 0:
			title = "Canciones en espera"
			for i, it := range q {
				if i >= 4 {
					lines = append(lines, fmt.Sprintf("… y %d más", len(q)-4))
					break
				}
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, it.Label()))
			}
		case cur == nil:
			title = "La cola está vacía"
			lines = []string{"¡Anímate! Elige tu canción favorita"}
		default:
			lines = append(lines, "▶ "+cur.Label())
			for i, it := range q {
				if i >= 4 {
					lines = append(lines, fmt.Sprintf("… y %d más", len(q)-4))
					break
				}
				lines = append(lines, fmt.Sprintf("%d. %s", i+1, it.Label()))
			}
		}
	}
	s.slideTitle.Text = title
	s.slideTitle.Refresh()
	for i, t := range s.lines {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		t.set(line)
	}
}
