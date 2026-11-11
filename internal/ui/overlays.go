package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/josegamboa/karaokenino/internal/catalog"
)

// overlay is a panel drawn over the main screen that takes the keys.
type overlay struct {
	name    string // "admin", "pin"… (tests and logs)
	root    fyne.CanvasObject
	key     func(name fyne.KeyName, action string)
	rune    func(r rune)
	onQueue func() // the queue changed
	onClose func()
}

func (a *App) showOverlay(o *overlay) {
	if a.ov != nil {
		a.root.Remove(a.ov.root)
	}
	a.ov = o
	a.root.Add(o.root)
	a.updateCountdown()
}

func (a *App) closeOverlay() {
	if a.ov == nil {
		return
	}
	a.root.Remove(a.ov.root)
	a.ov = nil
	a.resetIdle()
	a.rearmVideoFullscreen()
}

// isClose reports the keys that close any overlay: Esc or "/".
func isClose(name fyne.KeyName, action string) bool {
	return action == "back" || action == "playlist" || name == fyne.KeyEscape
}

// dialog is a centered panel with a title, built on its own scaled layout.
type dialog struct {
	a     *App
	lay   *scaledLayout
	objs  []fyne.CanvasObject
	box   Rect // panel rect
	inner Rect // content rect below the title
	title *canvas.Text
	hint  *keyHint
}

func newDialog(a *App, title string, w, h int) *dialog {
	dw, dh := designW, designH
	w, h = min(w, dw-40), min(h, dh-40)
	d := &dialog{a: a, lay: newScaledLayout(dw, dh)}
	d.box = Rect{Left: (dw - w) / 2, Top: (dh - h) / 2, Width: w, Height: h}
	pal := &a.pal
	d.add(d.lay.placeFull(canvas.NewRectangle(color.NRGBA{0, 0, 0, 170})))
	d.add(d.lay.place(newPanel(blend(pal.Bg1, pal.Bg2), pal.Accent, 18), d.box))
	accent := canvas.NewLinearGradient(pal.Accent, pal.Accent2, 90)
	d.add(d.lay.place(accent, Rect{Left: d.box.Left + 18, Top: d.box.Top + 58, Width: d.box.Width - 36, Height: 3}))
	d.title = newText(title, pal.Text, true)
	d.add(d.lay.placeText(d.title, Rect{Left: d.box.Left + 24, Top: d.box.Top + 14, Width: d.box.Width - 48, Height: 36}, 28))
	d.hint = newKeyHint(pal, Rect{Left: d.box.Left + 24, Top: d.box.Y2() - 36, Width: d.box.Width - 48, Height: 26}, 16, pal.TextDim, fyne.TextAlignLeading)
	d.add(d.hint.place(d.lay))
	d.inner = Rect{Left: d.box.Left + 24, Top: d.box.Top + 74, Width: d.box.Width - 48, Height: d.box.Height - 74 - 44}
	return d
}

func (d *dialog) add(o ...fyne.CanvasObject) { d.objs = append(d.objs, o...) }

// setHint shows the keys of this window at its bottom (mark keys with key()).
func (d *dialog) setHint(s string) { d.hint.set(s) }

// keys is a line of text with key caps (see key()).
func (d *dialog) keys(s string, r Rect, size float32, c color.Color) *keyHint {
	h := newKeyHint(&d.a.pal, r, size, c, fyne.TextAlignLeading)
	h.set(s)
	d.add(h.place(d.lay))
	return h
}

func (d *dialog) text(s string, r Rect, size float32, c color.Color, bold bool) *canvas.Text {
	t := newText(s, c, bold)
	d.add(d.lay.placeText(t, r, size))
	return t
}

func (d *dialog) list(area Rect, rowH int, font float32, numW int) *rowList {
	l := newRowList(d.lay, &d.a.pal, area, rowH, font, numW)
	d.add(l.objects()...)
	return l
}

func (d *dialog) container() fyne.CanvasObject {
	return container.New(d.lay, d.objs...)
}

func blend(a, b color.NRGBA) color.NRGBA {
	return color.NRGBA{uint8((int(a.R) + int(b.R)) / 2), uint8((int(a.G) + int(b.G)) / 2), uint8((int(a.B) + int(b.B)) / 2), 250}
}

// ---------- song number (key 0) ----------

// openNumber asks for a song number from the printed catalog.
func (a *App) openNumber() {
	d := newDialog(a, "Pedir canción por número", 900, 330)
	pal := &a.pal
	d.text("Marca el número del catálogo:", Rect{Left: d.inner.Left, Top: d.inner.Top + 6, Width: d.inner.Width, Height: 30}, 22, pal.TextDim, false)
	big := d.text("_", Rect{Left: d.inner.Left, Top: d.inner.Top + 44, Width: d.inner.Width, Height: 70}, 60, pal.Accent2, true)
	song := d.text("", Rect{Left: d.inner.Left, Top: d.inner.Top + 128, Width: d.inner.Width, Height: 34}, 26, pal.Text, true)
	d.setHint("Números: marcar  ·  " + key(".") + " borrar  ·  " + key("Intro") + " agregar  ·  " + key("/") + " cerrar")

	digits := ""
	var found *catalog.Track
	render := func() {
		big.Text = digits + "_"
		switch {
		case digits == "":
			song.Text = ""
		case found != nil:
			song.Text = fit(found.Title+" — "+found.Artist, d.inner.Width, 26)
		default:
			song.Text = "No hay canción con ese número"
		}
		big.Refresh()
		song.Refresh()
	}
	lookup := func() {
		found = nil
		if n, err := strconv.ParseInt(digits, 10, 64); err == nil && n > 0 {
			if t, err := a.Store.TrackByID(a.ctx, n); err == nil && a.genreVisible(t.Genre) {
				found = &t
			}
		}
	}
	o := &overlay{name: "numero", root: d.container()}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action):
			a.closeOverlay()
			return
		case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
			if len(digits) < 7 {
				digits += string(name)
				lookup()
			}
		case name == fyne.KeyPeriod || action == "backspace" || action == "next":
			if digits != "" {
				digits = digits[:len(digits)-1]
				lookup()
			}
		case action == "ok":
			if found != nil {
				a.closeOverlay()
				a.enqueue(*found)
				return
			}
		}
		render()
	}
	render()
	a.showOverlay(o)
}

// ---------- queue (key /) ----------

func (a *App) openQueue() {
	d := newDialog(a, "Cola de canciones", 900, 560)
	l := d.list(d.inner, 44, 24, 70)
	l.empty = "No hay canciones en la cola. Elige una y presiona Intro."
	render := func() {
		cur, q := a.box.Snapshot()
		var items []rowItem
		if cur != nil {
			tag := "AHORA"
			if cur.Filler {
				tag = "FONDO"
			}
			items = append(items, rowItem{Num: "▶", Text: cur.Title, Sub: cur.Artist, Badge: tag, Color: a.pal.Accent2})
		}
		for i, it := range q {
			items = append(items, rowItem{Num: fmt.Sprint(i + 1), Text: it.Title, Sub: it.Artist})
		}
		l.set(items, min(l.sel, max(len(items)-1, 0)))
		d.setHint(fmt.Sprintf("%d en cola  ·  ", len(q)) + key("8") + key("2") + " recorrer  ·  " + key("/") + " cerrar")
	}
	o := &overlay{name: "cola", root: d.container(), onQueue: render}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action) || action == "ok":
			a.closeOverlay()
		case action == "up":
			l.selectIdx(l.sel - 1)
		case action == "down":
			l.selectIdx(l.sel + 1)
		}
	}
	render()
	a.showOverlay(o)
}

// ---------- most requested (key 7) ----------

func (a *App) openTop() {
	d := newDialog(a, "Lo más pedido", 900, 560)
	l := d.list(d.inner, 44, 24, 70)
	l.empty = "Todavía no se ha cantado nada en este evento."
	ranked := a.topSongs(30)
	items := make([]rowItem, len(ranked))
	for i, r := range ranked {
		badge, col := badgeFor(&a.pal, r.Kind)
		items[i] = rowItem{Num: fmt.Sprintf("%d.", i+1), Text: r.Title, Sub: fmt.Sprintf("%s · %d veces", r.Artist, r.Plays), Badge: badge, Color: col}
	}
	l.set(items, 0)
	d.setHint(key("8") + key("2") + " elegir  ·  " + key("Intro") + " agregar a la cola  ·  " + key("/") + " cerrar")
	o := &overlay{name: "top", root: d.container()}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action) || action == "top":
			a.closeOverlay()
		case action == "up":
			l.selectIdx(l.sel - 1)
		case action == "down":
			l.selectIdx(l.sel + 1)
		case action == "ok" && l.sel < len(ranked):
			a.closeOverlay()
			a.enqueue(ranked[l.sel].Track)
		}
	}
	a.showOverlay(o)
}

// topSongs: this event's ranking, or all-time if the event has none yet.
func (a *App) topSongs(n int) []catalog.Ranked {
	a.stMu.Lock()
	ev := a.st.EventID
	a.stMu.Unlock()
	var out []catalog.Ranked
	for _, id := range []int64{ev, 0} {
		ranked, err := a.Store.Top(a.ctx, id, n)
		if err == nil && len(ranked) > 0 {
			out = ranked
			break
		}
		if ev == 0 {
			break
		}
	}
	vis := out[:0]
	for _, r := range out {
		if a.genreVisible(r.Genre) {
			vis = append(vis, r)
		}
	}
	return vis
}

// ---------- text search (full keyboard) ----------

func (a *App) openSearch(initial string) {
	d := newDialog(a, "Buscar canción", 1000, 600)
	pal := &a.pal
	field := Rect{Left: d.inner.Left, Top: d.inner.Top, Width: d.inner.Width, Height: 52}
	d.add(d.lay.place(newPanel(withAlpha(pal.Text, 20), pal.Accent2, 10), field))
	q := d.text("", Rect{Left: field.Left + 16, Top: field.Top + 10, Width: field.Width - 32, Height: 34}, 28, pal.Text, true)
	l := d.list(Rect{Left: d.inner.Left, Top: field.Y2() + 12, Width: d.inner.Width, Height: d.inner.Height - 64}, 42, 22, 90)
	l.empty = "Escribe parte del título o del cantante"
	d.setHint("Escribe  ·  " + key("↑") + key("↓") + " elegir  ·  " + key("Intro") + " agregar  ·  " + key("→") + " ir al artista  ·  " + key("Esc") + " cerrar")

	query := initial
	var results []catalog.Track
	var timer *time.Timer
	run := func() {
		results = nil
		if strings.TrimSpace(query) != "" {
			results, _ = a.Store.Search(a.ctx, query, a.genreIDs(), 60)
		}
		items := make([]rowItem, len(results))
		for i, t := range results {
			badge, col := badgeFor(pal, t.Kind)
			items[i] = rowItem{Num: fmt.Sprint(t.ID), Text: t.Title, Sub: t.Artist, Badge: badge, Color: col}
		}
		if len(items) == 0 && strings.TrimSpace(query) != "" {
			l.empty = "Sin resultados para «" + query + "»"
		}
		l.set(items, 0)
	}
	update := func() {
		q.Text = query + "_"
		q.Refresh()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(150*time.Millisecond, func() { fyne.Do(run) })
	}
	o := &overlay{name: "busqueda", root: d.container()}
	o.rune = func(r rune) {
		if len([]rune(query)) < 40 {
			query += string(r)
			update()
		}
	}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case name == fyne.KeyEscape:
			a.closeOverlay()
		case name == fyne.KeyBackspace:
			if r := []rune(query); len(r) > 0 {
				query = string(r[:len(r)-1])
				update()
			}
		case name == fyne.KeyUp:
			l.selectIdx(l.sel - 1)
		case name == fyne.KeyDown:
			l.selectIdx(l.sel + 1)
		case (name == fyne.KeyReturn || name == fyne.KeyEnter) && l.sel < len(results):
			a.closeOverlay()
			a.enqueue(results[l.sel])
		case name == fyne.KeyRight && l.sel < len(results):
			a.closeOverlay()
			a.showTrack(results[l.sel])
		}
	}
	update()
	a.showOverlay(o)
}
