package ui

import (
	"image/color"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// keypad draws the rockola's numeric keypad on screen. Each key shows what
// it does right now (it changes with the column that has the focus) and
// lights up when pressed, so guests learn the keys by watching.
type keypad struct {
	lay  *scaledLayout
	pal  *Palette
	keys map[string]*padKey
	// ripple is a ring that grows out of the last pressed key and fades.
	ripple   *canvas.Rectangle
	rippleOf *padKey
	rippleAt time.Time
}

type padKey struct {
	bg           *canvas.Rectangle
	glyph, label *canvas.Text
	r            Rect
	on           bool // has a function now
	flashUntil   time.Time
	popAt        time.Time // pressed: the key "pops" (grows and settles)
}

const keyPop = 260 * time.Millisecond

// keypad cells: id, glyph, column, row, width (cols), height (rows).
var padLayout = []struct {
	id, glyph  string
	c, r, w, h int
}{
	{"/", "/", 1, 0, 1, 1}, {"*", "*", 2, 0, 1, 1}, {"-", "−", 3, 0, 1, 1},
	{"7", "7", 0, 1, 1, 1}, {"8", "8", 1, 1, 1, 1}, {"9", "9", 2, 1, 1, 1}, {"+", "+", 3, 1, 1, 2},
	{"4", "4", 0, 2, 1, 1}, {"5", "5", 1, 2, 1, 1}, {"6", "6", 2, 2, 1, 1},
	{"1", "1", 0, 3, 1, 1}, {"2", "2", 1, 3, 1, 1}, {"3", "3", 2, 3, 1, 1}, {"enter", "Intro", 3, 3, 1, 2},
	{"0", "0", 0, 4, 2, 1}, {".", ".", 2, 4, 1, 1},
}

func newKeypad(lay *scaledLayout, pal *Palette, area Rect) (*keypad, []fyne.CanvasObject) {
	k := &keypad{lay: lay, pal: pal, keys: map[string]*padKey{}}
	const gap = 7
	cw := (area.Width - 3*gap) / 4
	rh := (area.Height - 4*gap) / 5
	var objs []fyne.CanvasObject
	for _, p := range padLayout {
		r := Rect{
			Left: area.Left + p.c*(cw+gap), Top: area.Top + p.r*(rh+gap),
			Width: p.w*cw + (p.w-1)*gap, Height: p.h*rh + (p.h-1)*gap,
		}
		key := &padKey{r: r, bg: newPanel(pal.Panel, pal.PanelBorder, 12)}
		gs := float32(min(rh, 60)) * 0.42
		if len(p.glyph) > 1 {
			gs *= 0.62
		}
		key.glyph = newText(p.glyph, pal.Text, true)
		key.glyph.Alignment = fyne.TextAlignCenter
		key.label = newText("", pal.TextDim, false)
		key.label.Alignment = fyne.TextAlignCenter
		mid := r.Top + r.Height/2
		objs = append(objs,
			lay.place(key.bg, r),
			lay.placeText(key.glyph, Rect{Left: r.Left, Top: mid - int(gs) - 1, Width: r.Width, Height: int(gs * 1.2)}, gs),
			lay.placeText(key.label, Rect{Left: r.Left + 2, Top: mid + 3, Width: r.Width - 4, Height: 14}, 11),
		)
		k.keys[p.id] = key
	}
	k.ripple = newPanel(color.NRGBA{}, color.NRGBA{}, 14)
	k.ripple.StrokeWidth = 3
	k.ripple.Hidden = true
	objs = append(objs, lay.place(k.ripple, area))
	return k, objs
}

// setLabels shows what each key does; keys without a label are dimmed.
func (k *keypad) setLabels(labels map[string]string) {
	for id, key := range k.keys {
		lbl := labels[id]
		key.on = lbl != ""
		key.label.Text = fit(lbl, key.r.Width, 11)
		k.paint(key)
	}
}

func (k *keypad) paint(key *padKey) {
	lit := time.Now().Before(key.flashUntil)
	switch {
	case lit:
		key.bg.FillColor = k.pal.Accent
		key.bg.StrokeColor = k.pal.Accent2
		key.glyph.Color = k.pal.SelectedText
		key.label.Color = k.pal.SelectedText
	case key.on:
		// Glass key: the party background shows through.
		key.bg.FillColor = k.pal.Panel
		key.bg.StrokeColor = k.pal.PanelBorder
		key.glyph.Color = k.pal.Text
		key.label.Color = k.pal.Accent2
	default:
		// No function now: a faint outline, dimmed glyph.
		key.bg.FillColor = withAlpha(k.pal.Panel, k.pal.Panel.A/2)
		key.bg.StrokeColor = color.NRGBA{}
		key.glyph.Color = withAlpha(k.pal.TextDim, 110)
		key.label.Color = k.pal.TextDim
	}
	key.bg.Refresh()
	key.glyph.Refresh()
	key.label.Refresh()
}

// flash lights a key for a moment.
func (k *keypad) flash(id string) {
	key := k.keys[id]
	if key == nil {
		return
	}
	key.flashUntil = time.Now().Add(220 * time.Millisecond)
	key.popAt = time.Now()
	k.rippleOf, k.rippleAt = key, key.popAt
	k.paint(key)
	time.AfterFunc(240*time.Millisecond, func() { fyne.Do(func() { k.paint(key) }) })
}

// animate makes the pressed key pop (it grows a few pixels and settles)
// and sends a ring out of it. Only rectangles move: no redrawing.
func (k *keypad) animate(now time.Time) {
	for _, key := range k.keys {
		if key.popAt.IsZero() {
			continue
		}
		p := float64(now.Sub(key.popAt)) / float64(keyPop)
		d := 0
		if p < 1 {
			d = int(math.Round(5 * math.Sin(math.Pi*p)))
		} else {
			key.popAt = time.Time{}
		}
		k.lay.move(key.bg, grow(key.r, d))
	}
	key := k.rippleOf
	if key == nil {
		return
	}
	p := float64(now.Sub(k.rippleAt)) / float64(2*keyPop)
	if p >= 1 {
		k.ripple.Hidden = true
		k.rippleOf = nil
		return
	}
	k.lay.move(k.ripple, grow(key.r, 3+int(16*p)))
	k.ripple.StrokeColor = withAlpha(k.pal.Accent2, uint8(220*(1-p)))
	k.ripple.Hidden = false
}

// padCell maps an evdev key name from the config to a keypad cell.
func padCell(evdev string) string {
	e := strings.ToUpper(strings.TrimSpace(evdev))
	switch e {
	case "KEY_KPENTER":
		return "enter"
	case "KEY_KPDOT":
		return "."
	case "KEY_KPPLUS":
		return "+"
	case "KEY_KPMINUS":
		return "-"
	case "KEY_KPASTERISK":
		return "*"
	case "KEY_KPSLASH":
		return "/"
	}
	if d, ok := strings.CutPrefix(e, "KEY_KP"); ok && len(d) == 1 && d[0] >= '0' && d[0] <= '9' {
		return d
	}
	return ""
}

// keyCell maps a pressed key to the keypad cell to light.
func (a *App) keyCell(name fyne.KeyName) string {
	switch name {
	case fyne.KeyEnter, fyne.KeyReturn:
		return "enter"
	case fyne.KeyPeriod, "+", "-", "*", "/":
		return string(name)
	}
	if len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		return string(name)
	}
	// Arrows light the keypad key with the same action (8 for ↑...).
	if act := a.actionFor(name); act != "" {
		return a.actionCells[act]
	}
	return ""
}
