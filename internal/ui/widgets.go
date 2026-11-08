package ui

import (
	"image/color"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"github.com/josegamboa/karaokenino/internal/catalog"
)

// Small drawing helpers. Everything is a canvas primitive placed on a
// scaledLayout, so it scales with the TV.

func newText(s string, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextStyle = fyne.TextStyle{Bold: bold}
	return t
}

func newPanel(fill, border color.NRGBA, radius float32) *canvas.Rectangle {
	r := canvas.NewRectangle(fill)
	r.StrokeColor = border
	if border.A > 0 {
		r.StrokeWidth = 1.5
	}
	r.CornerRadius = radius
	return r
}

// fit shortens s so it fits width design px at font size (approximate).
func fit(s string, width int, size float32) string {
	maxChars := int(float32(width) / (size * 0.56))
	if maxChars < 4 {
		maxChars = 4
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:maxChars-1])) + "…"
}

// rowItem is one line of a rowList.
type rowItem struct {
	Num      string
	Text     string
	Sub      string // dim text after the main text (singer)
	Badge    string
	Color    color.NRGBA // badge color
	Icon     string      // image file (lists with icons)
	Initials string      // shown when there is no icon
}

// rowList is a fixed number of visible rows over a scrolled item list.
type rowList struct {
	lay      *scaledLayout
	pal      *Palette
	area     Rect
	rowH     int
	font     float32
	numW     int
	iconW    int
	badgeW   int
	rows     []*listRow
	items    []rowItem
	sel, top int
	empty    string
	// active marks the list that has the keypad focus; the others show
	// their selection dimmed.
	active bool
	// iconFn, if set, gives the icon of item i; only visible rows ask.
	iconFn func(i int) string

	// halo is the glow behind the selected row (it pulses while the screen
	// animates); sparks fly out of the row when a song is queued.
	halo        *canvas.Rectangle
	sparks      []*canvas.Circle
	celebrateAt time.Time
}

const (
	nSparks       = 12
	sparkDuration = 700 * time.Millisecond
	rowFlash      = 300 * time.Millisecond
)

type listRow struct {
	bg, badgeBg      *canvas.Rectangle
	num, text, badge *canvas.Text
	iconBg           *canvas.Rectangle
	icon             *canvas.Image
	initials         *canvas.Text
}

func newRowList(lay *scaledLayout, pal *Palette, area Rect, rowH int, font float32, numW int) *rowList {
	return newList(lay, pal, area, rowH, font, numW, 0, 92)
}

// newList builds a list; iconW > 0 adds a square image per row, badgeW 0 = no badge.
func newList(lay *scaledLayout, pal *Palette, area Rect, rowH int, font float32, numW, iconW, badgeW int) *rowList {
	l := &rowList{lay: lay, pal: pal, area: area, rowH: rowH, font: font, numW: numW, iconW: iconW, badgeW: badgeW, active: true}
	n := area.Height / rowH
	l.halo = newPanel(withAlpha(pal.Accent, 90), color.NRGBA{}, float32(rowH)/3)
	l.halo.Hidden = true
	lay.place(l.halo, l.haloRect(0))
	for i := 0; i < nSparks; i++ {
		c := canvas.NewCircle(pal.Accent2)
		c.Hidden = true
		lay.place(c, Rect{area.Left, area.Top, 6, 6})
		l.sparks = append(l.sparks, c)
	}
	textH := int(font * 1.3)
	for i := 0; i < n; i++ {
		y := area.Top + i*rowH
		r := &listRow{
			bg:      newPanel(color.NRGBA{}, color.NRGBA{}, float32(rowH)/5),
			badgeBg: newPanel(color.NRGBA{}, color.NRGBA{}, 4),
			num:     newText("", pal.TextDim, false),
			text:    newText("", pal.Text, false),
			badge:   newText("", pal.BadgeText, true),
		}
		r.badge.Alignment = fyne.TextAlignCenter
		r.num.Alignment = fyne.TextAlignTrailing
		lay.place(r.bg, Rect{Left: area.Left, Top: y + 1, Width: area.Width, Height: rowH - 2})
		x := area.Left
		if iconW > 0 {
			ir := Rect{Left: x + 6, Top: y + (rowH-iconW)/2, Width: iconW, Height: iconW}
			r.iconBg = newPanel(withAlpha(pal.Accent2, 35), color.NRGBA{}, 6)
			r.initials = newText("", withAlpha(pal.Text, 210), true)
			r.initials.Alignment = fyne.TextAlignCenter
			r.icon = canvas.NewImageFromResource(nil)
			r.icon.FillMode = canvas.ImageFillContain
			lay.place(r.iconBg, ir)
			lay.placeText(r.initials, Rect{Left: ir.Left, Top: ir.Top + (iconW-int(font*1.2))/2, Width: iconW, Height: int(font * 1.2)}, font*0.8)
			lay.place(r.icon, ir)
			x += iconW + 14
		}
		textTop := y + (rowH-textH)/2
		if numW > 0 {
			lay.placeText(r.num, Rect{Left: x + 4, Top: textTop, Width: numW - 12, Height: textH}, font*0.8)
			x += numW
		}
		textW := area.X2() - x - 10
		if badgeW > 0 {
			textW -= badgeW + 14
			bh := int(font * 1.05)
			br := Rect{Left: area.X2() - badgeW - 10, Top: y + (rowH-bh)/2, Width: badgeW, Height: bh}
			lay.place(r.badgeBg, br)
			lay.placeText(r.badge, br, font*0.6)
		}
		lay.placeText(r.text, Rect{Left: x, Top: textTop, Width: textW, Height: textH}, font)
		l.rows = append(l.rows, r)
	}
	return l
}

func (l *rowList) textWidth() int {
	w := l.area.Width - l.numW - 10
	if l.iconW > 0 {
		w -= l.iconW + 14
	}
	if l.badgeW > 0 {
		w -= l.badgeW + 14
	}
	return w
}

func (l *rowList) objects() []fyne.CanvasObject {
	out := []fyne.CanvasObject{l.halo}
	for _, r := range l.rows {
		out = append(out, r.bg, r.badgeBg, r.num, r.text, r.badge)
		if r.icon != nil {
			out = append(out, r.iconBg, r.initials, r.icon)
		}
	}
	for _, c := range l.sparks {
		out = append(out, c)
	}
	return out
}

// haloRect is the glow around visible row i.
func (l *rowList) haloRect(i int) Rect {
	return Rect{Left: l.area.Left - 4, Top: l.area.Top + i*l.rowH - 3, Width: l.area.Width + 8, Height: l.rowH + 6}
}

// selRow is the visible row of the selection (-1: none).
func (l *rowList) selRow() int {
	i := l.sel - l.top
	if len(l.items) == 0 || i < 0 || i >= len(l.rows) {
		return -1
	}
	return i
}

// animate pulses the glow of the selected row and moves the queue sparkle.
// Only colors and positions change: nothing is redrawn on the CPU.
func (l *rowList) animate(now time.Time, t float64) {
	if !l.halo.Hidden {
		pulse := 0.5 + 0.5*math.Sin(2*math.Pi*t/1.6)
		l.halo.FillColor = withAlpha(l.pal.Accent, uint8(45+85*pulse))
	}
	if l.celebrateAt.IsZero() {
		return
	}
	el := now.Sub(l.celebrateAt)
	if i := l.selRow(); i >= 0 && l.active {
		bg := l.pal.Accent
		if el < rowFlash {
			bg = l.pal.Accent2
		}
		l.rows[i].bg.FillColor = bg
	}
	p := float64(el) / float64(sparkDuration)
	if p >= 1 {
		l.celebrateAt = time.Time{}
		for _, c := range l.sparks {
			c.Hidden = true
		}
		return
	}
	i := max(l.selRow(), 0)
	cx := float64(l.area.Left + l.area.Width*2/5)
	cy := float64(l.area.Top + i*l.rowH + l.rowH/2)
	ease := 1 - (1-p)*(1-p)
	cols := []color.NRGBA{l.pal.Accent2, l.pal.Accent, rgb(255, 255, 255)}
	for k, c := range l.sparks {
		ang := 2 * math.Pi * (float64(k) + 0.5) / float64(len(l.sparks))
		dist := (70 + float64(k%3)*28) * ease
		r := int(6 - 4*p)
		x := cx + math.Cos(ang)*dist*1.6 // wider than tall, like the row
		y := cy + math.Sin(ang)*dist*0.7
		l.lay.move(c, Rect{Left: int(x) - r, Top: int(y) - r, Width: 2 * r, Height: 2 * r})
		c.FillColor = withAlpha(cols[k%len(cols)], uint8(255*(1-p)))
		c.Hidden = false
	}
}

// celebrate marks a song just added to the queue: the selected row flashes
// and, when the screen animates, sparks fly out of it.
func (l *rowList) celebrate(now time.Time, animated bool) {
	i := l.selRow()
	if i < 0 {
		return
	}
	if animated {
		l.celebrateAt = now
		return
	}
	l.rows[i].bg.FillColor = l.pal.Accent2
	canvas.Refresh(l.rows[i].bg)
	time.AfterFunc(rowFlash, func() { fyne.Do(l.refresh) })
}

func (l *rowList) set(items []rowItem, sel int) {
	l.items = items
	l.sel = min(max(sel, 0), max(len(items)-1, 0))
	l.top = 0
	l.refresh()
}

func (l *rowList) selectIdx(i int) {
	if len(l.items) == 0 {
		return
	}
	l.sel = (i%len(l.items) + len(l.items)) % len(l.items)
	l.refresh()
}

func (l *rowList) setActive(a bool) {
	if l.active != a {
		l.active = a
		l.refresh()
	}
}

func (l *rowList) refresh() {
	n := len(l.rows)
	// Keep the selection away from the edges so guests see what comes next.
	margin := min(2, (n-1)/2)
	if l.sel < l.top+margin {
		l.top = l.sel - margin
	}
	if l.sel > l.top+n-1-margin {
		l.top = l.sel - n + 1 + margin
	}
	l.top = max(0, min(l.top, len(l.items)-n))
	for i, r := range l.rows {
		idx := l.top + i
		if idx >= len(l.items) {
			r.bg.FillColor = color.NRGBA{}
			r.badgeBg.FillColor = color.NRGBA{}
			r.num.Text, r.badge.Text = "", ""
			r.text.Text = ""
			if i == 0 && len(l.items) == 0 {
				r.text.Text = l.empty
				r.text.Color = l.pal.TextDim
			}
			if r.icon != nil {
				r.icon.Hidden, r.iconBg.Hidden, r.initials.Text = true, true, ""
				canvas.Refresh(r.icon)
				canvas.Refresh(r.iconBg)
				canvas.Refresh(r.initials)
			}
			canvas.Refresh(r.bg)
			canvas.Refresh(r.badgeBg)
			canvas.Refresh(r.num)
			canvas.Refresh(r.text)
			canvas.Refresh(r.badge)
			continue
		}
		it := l.items[idx]
		selected := idx == l.sel
		r.num.Text = it.Num
		txt := it.Text
		if it.Sub != "" {
			txt += "  ·  " + it.Sub
		}
		r.text.Text = fit(txt, l.textWidth(), l.font)
		r.badge.Text = it.Badge
		r.badge.Color = l.pal.BadgeText
		if it.Badge != "" {
			r.badgeBg.FillColor = it.Color
		} else {
			r.badgeBg.FillColor = color.NRGBA{}
		}
		if r.icon != nil {
			if l.iconFn != nil {
				it.Icon = l.iconFn(idx)
			}
			r.iconBg.Hidden = false
			r.icon.Hidden = it.Icon == ""
			if r.icon.File != it.Icon {
				// Rows are reused while scrolling: the image method Refresh
				// reloads the new file; canvas.Refresh would only repaint the
				// picture already decoded (the previous artist's cover).
				r.icon.File = it.Icon
				r.icon.Resource = nil
				r.icon.Refresh()
			} else {
				canvas.Refresh(r.icon)
			}
			r.initials.Text = ""
			if it.Icon == "" {
				r.initials.Text = it.Initials
			}
			canvas.Refresh(r.iconBg)
			canvas.Refresh(r.initials)
		}
		switch {
		case selected && l.active:
			if it.Badge != "" {
				// Dark chip with the badge color as text, on the bright bar.
				r.badgeBg.FillColor = l.pal.SelectedText
				r.badge.Color = it.Color
			}
			r.bg.FillColor = l.pal.Accent
			r.text.Color = l.pal.SelectedText
			r.num.Color = l.pal.SelectedText
			r.text.TextStyle.Bold = true
		case selected:
			r.bg.FillColor = withAlpha(l.pal.Accent, 70)
			r.text.Color = l.pal.Text
			r.num.Color = l.pal.TextDim
			r.text.TextStyle.Bold = true
		default:
			r.bg.FillColor = color.NRGBA{}
			if i%2 == 1 {
				r.bg.FillColor = withAlpha(l.pal.Text, 7)
			}
			r.text.Color = l.pal.Text
			r.num.Color = l.pal.TextDim
			r.text.TextStyle.Bold = false
		}
		canvas.Refresh(r.bg)
		canvas.Refresh(r.badgeBg)
		canvas.Refresh(r.num)
		canvas.Refresh(r.text)
		canvas.Refresh(r.badge)
	}
	// Glow around the selected row of the list that has the focus.
	if i := l.selRow(); i >= 0 && l.active {
		l.lay.move(l.halo, l.haloRect(i))
		l.halo.Hidden = false
	} else {
		l.halo.Hidden = true
	}
	canvas.Refresh(l.halo)
}

// progressBar is a thin bar with a fill proportional to pos/dur.
type progressBar struct {
	lay      *scaledLayout
	area     Rect
	bg, fill *canvas.Rectangle
	w        int  // fill width in design px
	hidden   bool // show(false)
}

func newProgressBar(lay *scaledLayout, pal *Palette, area Rect) *progressBar {
	p := &progressBar{lay: lay, area: area,
		bg:   newPanel(withAlpha(pal.Text, 30), color.NRGBA{}, float32(area.Height)/2),
		fill: newPanel(pal.Accent2, color.NRGBA{}, float32(area.Height)/2),
	}
	lay.place(p.bg, area)
	lay.place(p.fill, Rect{Left: area.Left, Top: area.Top, Width: 0, Height: area.Height})
	p.fill.Hidden = true // nothing played yet
	return p
}

func (p *progressBar) set(frac float64) {
	frac = min(max(frac, 0), 1)
	w := int(float64(p.area.Width) * frac)
	if w == p.w {
		return // same pixel: nothing to redraw
	}
	p.w = w
	p.lay.move(p.fill, Rect{Left: p.area.Left, Top: p.area.Top, Width: w, Height: p.area.Height})
	p.fill.Hidden = w == 0 || p.hidden
	canvas.Refresh(p.fill)
}

func (p *progressBar) show(v bool) {
	if p.hidden == !v {
		return
	}
	p.hidden = !v
	p.bg.Hidden, p.fill.Hidden = !v, !v || p.w == 0
	canvas.Refresh(p.bg)
	canvas.Refresh(p.fill)
}

// badgeFor labels a song by kind.
func badgeFor(pal *Palette, k catalog.Kind) (string, color.NRGBA) {
	switch k {
	case catalog.KindKaraoke:
		return "KARAOKE", pal.Karaoke
	case catalog.KindAudio:
		return "AUDIO", pal.Audio
	default:
		return "VIDEO", pal.Video
	}
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		r := []rune(w)[0]
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 {
			out = append(out, r)
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "♪"
	}
	return strings.ToUpper(string(out))
}
