package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// key marks a key inside a hint: keyHint draws it as a little key cap, so
// guests see it is a key to press and not a word ("7 ver la lista" →
// key("7")+"  ver la lista"). The marks are control characters, never in a
// song title, so a "[Karaoke]" in a title stays plain text.
func key(k string) string { return keyOpen + k + keyClose }

const keyOpen, keyClose = "\x01", "\x02"

var keyMarks = strings.NewReplacer(keyOpen, "", keyClose, "")

// plainKeys is the hint as plain text (logs, tests, mpv's own OSD).
func plainKeys(s string) string { return keyMarks.Replace(s) }

type hintSeg struct {
	text string
	key  bool
}

func parseHint(s string) []hintSeg {
	var segs []hintSeg
	for s != "" {
		i := strings.Index(s, keyOpen)
		if i < 0 {
			segs = append(segs, hintSeg{text: s})
			break
		}
		if i > 0 {
			segs = append(segs, hintSeg{text: s[:i]})
		}
		s = s[i+len(keyOpen):]
		j := strings.Index(s, keyClose)
		if j < 0 {
			j = len(s)
		}
		segs = append(segs, hintSeg{text: s[:j], key: true})
		s = s[min(j+len(keyClose), len(s)):]
	}
	return segs
}

// keyHint is a line of text with key caps in it. It is its own full-screen
// container on the same design grid, placed with placeFull, so it can be
// shown and hidden as one piece.
type keyHint struct {
	lay   *scaledLayout
	root  *fyne.Container
	pal   *Palette
	area  Rect
	size  float32
	align fyne.TextAlign
	color color.Color
	style fyne.TextStyle // of the text (the keys are always bold)

	texts  []*canvas.Text
	caps   []*canvas.Rectangle // the key: glassy, with a light border
	under  []*canvas.Rectangle // a darker edge below it (a key you press)
	glyphs []*canvas.Text

	spec string
	Text string // plainKeys(spec)
}

const hintMaxSegs = 12

func newKeyHint(pal *Palette, area Rect, size float32, c color.Color, align fyne.TextAlign) *keyHint {
	h := &keyHint{lay: newScaledLayout(designW, designH), pal: pal, area: area, size: size, align: align, color: c}
	var objs []fyne.CanvasObject
	for i := 0; i < hintMaxSegs; i++ {
		u := newPanel(withAlpha(pal.Text, 70), color.NRGBA{}, 5)
		k := newPanel(withAlpha(pal.Text, 38), withAlpha(pal.Text, 170), 5)
		g := newText("", pal.Text, true)
		g.Alignment = fyne.TextAlignCenter
		t := newText("", c, false)
		for _, o := range []fyne.CanvasObject{u, k, g, t} {
			o.Hide()
		}
		h.under, h.caps, h.glyphs, h.texts = append(h.under, u), append(h.caps, k), append(h.glyphs, g), append(h.texts, t)
		objs = append(objs, u, k, g, t)
	}
	h.root = container.New(h.lay, objs...)
	return h
}

// place puts the hint in a parent's scaled layout (same design size).
func (h *keyHint) place(lay *scaledLayout) fyne.CanvasObject { return lay.placeFull(h.root) }

func (h *keyHint) set(spec string) {
	if spec == h.spec {
		return
	}
	h.spec, h.Text = spec, plainKeys(spec)
	h.layout()
}

func (h *keyHint) setColor(c color.Color) {
	h.color = c
	for _, t := range h.texts {
		t.Color = c
		t.Refresh()
	}
}

func (h *keyHint) show(on bool) {
	if on {
		h.root.Show()
	} else {
		h.root.Hide()
	}
}

// metrics of a key cap for text size s.
func capMetrics(s float32) (glyph, height, pad, gap float32) {
	return s * 0.82, s * 1.3, s * 0.4, s * 0.25
}

func (h *keyHint) widths(segs []hintSeg, s float32) ([]float32, float32) {
	gs, ch, pad, gap := capMetrics(s)
	ws := make([]float32, len(segs))
	var total float32
	for i, sg := range segs {
		if sg.key {
			ws[i] = max(fyne.MeasureText(sg.text, gs, fyne.TextStyle{Bold: true}).Width+2*pad, ch)
			total += 2 * gap // a little air on both sides of every key
		} else {
			ws[i] = fyne.MeasureText(sg.text, s, h.style).Width
		}
		total += ws[i]
	}
	return ws, total
}

func (h *keyHint) layout() {
	segs := parseHint(h.spec)
	if len(segs) > hintMaxSegs {
		segs = segs[:hintMaxSegs]
	}
	s := h.size
	ws, total := h.widths(segs, s)
	if aw := float32(h.area.Width); total > aw && total > 0 {
		s *= max(aw/total, 0.7) // shrink a bit; what still does not fit is cut
		ws, total = h.widths(segs, s)
	}
	gs, ch, _, gap := capMetrics(s)
	x := float32(h.area.Left)
	switch free := float32(h.area.Width) - total; {
	case free <= 0:
	case h.align == fyne.TextAlignCenter:
		x += free / 2
	case h.align == fyne.TextAlignTrailing:
		x += free
	}
	right := float32(h.area.X2())
	mid := float32(h.area.Top) + float32(h.area.Height)/2
	capR := Rect{Top: int(mid - ch/2 + 0.5), Height: int(ch + 0.5)}
	nt, nk := 0, 0
	for i, sg := range segs {
		if x >= right {
			break
		}
		if sg.key {
			x += gap
			if x+ws[i] > right {
				break
			}
			r := capR
			r.Left, r.Width = int(x+0.5), int(ws[i]+0.5)
			h.put(h.under[nk], Rect{r.Left, r.Top + 2, r.Width, r.Height}, 0)
			h.put(h.caps[nk], r, 0)
			h.glyphs[nk].Text = sg.text
			h.put(h.glyphs[nk], r, gs)
			nk++
			x += gap
		} else {
			t := h.texts[nt]
			w := ws[i]
			t.Text = sg.text
			if x+w > right {
				t.Text = fit(strings.TrimSpace(sg.text), int(right-x), s)
				w = right - x
			}
			t.Color, t.TextStyle = h.color, h.style
			h.put(t, Rect{int(x + 0.5), h.area.Top, int(w + 1.5), h.area.Height}, s)
			nt++
		}
		x += ws[i]
	}
	for i := range h.texts {
		vis := func(o fyne.CanvasObject, on bool) {
			if on {
				o.Show()
			} else {
				o.Hide()
			}
		}
		vis(h.texts[i], i < nt)
		vis(h.under[i], i < nk)
		vis(h.caps[i], i < nk)
		vis(h.glyphs[i], i < nk)
	}
	h.root.Refresh()
}

func (h *keyHint) put(o fyne.CanvasObject, r Rect, textSize float32) {
	p := placement{r: r, textSize: textSize}
	h.lay.put(o, p)
	h.lay.apply(o, p)
}
