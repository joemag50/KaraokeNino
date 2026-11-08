package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/font/sfnt"
)

// ---------- contrast (WCAG 2.x) ----------

func lin(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func luminance(c color.NRGBA) float64 {
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// over draws c with opacity f over the opaque color bg (as the GPU blends).
func over(bg, c color.NRGBA, f float64) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a)*(1-f) + float64(b)*f)) }
	return color.NRGBA{mix(bg.R, c.R), mix(bg.G, c.G), mix(bg.B, c.B), 255}
}

func brightest(cs ...color.NRGBA) color.NRGBA {
	best := cs[0]
	for _, c := range cs[1:] {
		if luminance(c) > luminance(best) {
			best = c
		}
	}
	return best
}

func darkest(cs ...color.NRGBA) color.NRGBA {
	best := cs[0]
	for _, c := range cs[1:] {
		if luminance(c) < luminance(best) {
			best = c
		}
	}
	return best
}

// worstBackground is the brightest the animated background can get behind a
// text: the brighter end of the gradient, two far lights overlapping at
// their centers, a beam and a bubble on top, all at full strength, with the
// brightest colors they can have. (Bubble rings are 1.5 px lines and
// sparkles 3-6 px dots: too small to hide a letter, so they are left out.)
//
// On a light theme (dark text) the worst case is the darkest instead.
func worstBackground(p Palette) color.NRGBA {
	brightest := brightest
	if p.light() {
		brightest = darkest
	}
	bg := brightest(p.Bg1, p.Bg2)
	g := []color.NRGBA{p.Glow[0], p.Glow[1], p.Glow[2]}
	// the two brightest far lights
	first := brightest(g...)
	var rest []color.NRGBA
	for _, c := range g {
		if c != first {
			rest = append(rest, c)
		}
	}
	bg = over(bg, brightest(rest...), p.GlowAlpha)
	bg = over(bg, first, p.GlowAlpha)
	bg = over(bg, brightest(p.Glow[0], p.Glow[1]), beamPeak)
	bg = over(bg, brightest(p.Glow[0], p.Glow[1], p.Glow[2]), bubbleFill)
	return bg
}

type pair struct {
	what   string
	fg, bg color.NRGBA
	min    float64
}

func themePairs(p Palette) []pair {
	worst := worstBackground(p)
	panel := over(worst, p.Panel, float64(p.Panel.A)/255)
	dialog := blend(p.Bg1, p.Bg2)
	dialog.A = 255
	card := over(panel, p.Karaoke, 70.0/255) // selected mode card (brightest tint)
	ps := []pair{
		{"texto sobre el fondo", p.Text, worst, 4.5},
		{"texto tenue sobre el fondo", p.TextDim, worst, 4.5},
		{"acento 2 sobre el fondo", p.Accent2, worst, 4.5},
		{"texto sobre panel", p.Text, panel, 4.5},
		{"texto tenue sobre panel", p.TextDim, panel, 4.5},
		{"acento 2 sobre panel", p.Accent2, panel, 4.5},
		{"fila elegida", p.SelectedText, p.Accent, 4.5},
		{"fila elegida (destello al agregar)", p.SelectedText, p.Accent2, 4.5},
		{"texto sobre ventana", p.Text, dialog, 4.5},
		{"texto tenue sobre ventana", p.TextDim, dialog, 4.5},
		{"acento 2 sobre ventana", p.Accent2, dialog, 4.5},
		{"acento sobre ventana (aviso del PIN)", p.Accent, dialog, 4.5},
		{"texto sobre tarjeta elegida", p.Text, card, 4.5},
		{"texto tenue sobre tarjeta elegida", p.TextDim, card, 4.5},
		{"conteo sobre su tarjeta elegida (texto grande)", p.Karaoke, card, 3},
	}
	for _, b := range []struct {
		n string
		c color.NRGBA
	}{{"karaoke", p.Karaoke}, {"video", p.Video}, {"audio", p.Audio}} {
		ps = append(ps,
			pair{"etiqueta " + b.n, p.BadgeText, b.c, 4.5},
			pair{"etiqueta " + b.n + " en fila elegida", b.c, p.SelectedText, 4.5},
			pair{"conteo " + b.n + " sobre panel (texto grande)", b.c, panel, 3},
		)
	}
	return ps
}

// TestPaletteContrast keeps every theme readable from across the room, even
// at the brightest moment of the animated background.
func TestPaletteContrast(t *testing.T) {
	for _, name := range ThemeNames() {
		p := Themes[name]
		for _, c := range themePairs(p) {
			r := contrast(c.fg, c.bg)
			t.Logf("%-9s %-48s %5.2f:1", name, c.what, r)
			if r < c.min {
				t.Errorf("%s: %s: contraste %.2f:1, mínimo %.1f:1", name, c.what, r, c.min)
			}
		}
	}
}

// ---------- fonts ----------

// The embedded fonts draw Spanish themselves; symbols they lack fall back
// (Fyne tries Noto Sans, the emoji font, then the system fonts), as before.
func TestFontGlyphs(t *testing.T) {
	const spanish = "áéíóúñüÁÉÍÓÚÑÜ¿¡°ºª«»…—·×−0123456789"
	for _, res := range []fyne.Resource{fontRegular, fontBold} {
		f, err := sfnt.Parse(res.Content())
		if err != nil {
			t.Fatalf("%s: %v", res.Name(), err)
		}
		var b sfnt.Buffer
		has := func(r rune) bool {
			i, err := f.GlyphIndex(&b, r)
			return err == nil && i != 0
		}
		for _, r := range spanish + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz" {
			if !has(r) {
				t.Errorf("%s no tiene %q", res.Name(), r)
			}
		}
		var fallback []string
		for _, r := range okSymbols {
			if !has(r) {
				fallback = append(fallback, string(r))
			}
		}
		t.Logf("%s: %d KB; símbolos de la fuente de respaldo: %s", res.Name(), len(res.Content())/1024, strings.Join(fallback, " "))
	}
}

func TestPartyThemeFonts(t *testing.T) {
	th := newPartyTheme()
	if th.Font(fyne.TextStyle{}) != fontRegular || th.Font(fyne.TextStyle{Bold: true}) != fontBold {
		t.Fatal("el tema no usa las fuentes de fiesta")
	}
	// Symbol and monospace text keep Fyne's fonts.
	if th.Font(fyne.TextStyle{Symbol: true}) != theme.DefaultTheme().Font(fyne.TextStyle{Symbol: true}) ||
		th.Font(fyne.TextStyle{Monospace: true}) != theme.DefaultTheme().Font(fyne.TextStyle{Monospace: true}) {
		t.Fatal("símbolos o monoespaciada cambiaron")
	}
	if total := len(fontRegular.Content()) + len(fontBold.Content()); total > 1<<20 {
		t.Fatalf("fuentes de %d KB: demasiado para el binario", total/1024)
	}
}
