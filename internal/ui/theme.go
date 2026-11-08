package ui

import (
	_ "embed"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Palette colors the drawn panels, text and highlights.
//
// Every text color is checked against the background it is drawn on, in the
// worst case (the brightest moment of the animated background), by
// TestPaletteContrast: at least 4.5:1 (WCAG AA for body text).
type Palette struct {
	Name                  string
	Bg1, Bg2              color.NRGBA // background gradient (top → bottom)
	Panel, PanelBorder    color.NRGBA // translucent glass cards over the background
	Accent, Accent2       color.NRGBA // selection bar, highlighted text
	Text, TextDim         color.NRGBA
	SelectedText          color.NRGBA // text on Accent
	Karaoke, Video, Audio color.NRGBA // badges
	BadgeText             color.NRGBA // text on the badge colors
	Shadow                color.NRGBA
	// Glow are the colors of the soft lights drifting in the background;
	// GlowAlpha their strength at the center (bright colors get less).
	Glow      [3]color.NRGBA
	GlowAlpha float64
	// Sparkles is how many tiny twinkling dots the background has (glitter).
	Sparkles int
	// Band is the strip behind the status bar (darker on dark themes,
	// lighter on light ones).
	Band color.NRGBA
}

// light reports a theme with a light background and dark text.
func (p Palette) light() bool { return p.Text.R+p.Text.G < p.Bg1.R+p.Bg1.G }

func rgb(r, g, b uint8) color.NRGBA                { return color.NRGBA{r, g, b, 255} }
func rgba(r, g, b, a uint8) color.NRGBA            { return color.NRGBA{r, g, b, a} }
func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// Themes are chosen with config "theme" or from the admin menu (per event).
var Themes = map[string]Palette{
	// Neon night club: magenta and electric-blue lights on deep indigo,
	// a cyan neon selection bar.
	"moderno": {
		Name: "moderno",
		Bg1:  rgb(26, 6, 70), Bg2: rgb(5, 2, 26),
		Panel: rgba(10, 4, 34, 145), PanelBorder: rgba(90, 230, 255, 90),
		Accent: rgb(40, 230, 255), Accent2: rgb(255, 170, 240),
		Text: rgb(252, 250, 255), TextDim: rgb(214, 208, 250), SelectedText: rgb(10, 6, 40),
		Karaoke: rgb(255, 150, 234), Video: rgb(40, 230, 255), Audio: rgb(196, 172, 255),
		BadgeText: rgb(14, 6, 40),
		Shadow:    rgba(0, 0, 0, 160),
		Glow:      [3]color.NRGBA{rgb(255, 0, 170), rgb(0, 110, 255), rgb(140, 30, 255)},
		GlowAlpha: 0.34,
		Sparkles:  6,
		Band:      rgba(0, 0, 0, 110),
	},
	// Wedding: deep wine with champagne gold and blush rose; warm rose and
	// gold lights, a gold selection bar and gold glitter.
	"boda": {
		Name: "boda",
		Bg1:  rgb(80, 12, 42), Bg2: rgb(28, 4, 16),
		Panel: rgba(36, 6, 20, 150), PanelBorder: rgba(250, 210, 140, 120),
		Accent: rgb(250, 212, 128), Accent2: rgb(255, 218, 226),
		Text: rgb(255, 250, 242), TextDim: rgb(252, 234, 228), SelectedText: rgb(52, 10, 26),
		Karaoke: rgb(250, 212, 128), Video: rgb(255, 176, 196), Audio: rgb(240, 226, 206),
		BadgeText: rgb(52, 10, 26),
		Shadow:    rgba(0, 0, 0, 160),
		Glow:      [3]color.NRGBA{rgb(230, 60, 110), rgb(220, 150, 40), rgb(170, 40, 90)},
		GlowAlpha: 0.22,
		Sparkles:  16,
		Band:      rgba(0, 0, 0, 110),
	},
	// Kids: a light, sunny candy theme (cotton-candy pink to sky blue) with
	// dark purple text, a raspberry selection bar and rainbow confetti.
	"infantil": {
		Name: "infantil",
		Bg1:  rgb(255, 228, 244), Bg2: rgb(206, 236, 255),
		Panel: rgba(255, 255, 255, 175), PanelBorder: rgba(236, 72, 153, 150),
		Accent: rgb(205, 0, 110), Accent2: rgb(88, 22, 172),
		Text: rgb(38, 16, 86), TextDim: rgb(84, 52, 132), SelectedText: rgb(255, 255, 255),
		Karaoke: rgb(176, 50, 0), Video: rgb(0, 118, 108), Audio: rgb(146, 36, 196),
		BadgeText: rgb(255, 255, 255),
		Shadow:    rgba(80, 20, 120, 60),
		Glow:      [3]color.NRGBA{rgb(255, 120, 190), rgb(255, 214, 60), rgb(90, 220, 200)},
		GlowAlpha: 0.34,
		Sparkles:  22,
		Band:      rgba(255, 255, 255, 150),
	},
	// XV años: fuchsia and violet lights, a fuchsia selection bar and lots
	// of gold sparkle.
	"xv": {
		Name: "xv",
		Bg1:  rgb(64, 6, 80), Bg2: rgb(20, 3, 40),
		Panel: rgba(30, 4, 42, 145), PanelBorder: rgba(255, 140, 225, 100),
		Accent: rgb(255, 112, 210), Accent2: rgb(255, 220, 120),
		Text: rgb(255, 247, 253), TextDim: rgb(246, 214, 240), SelectedText: rgb(40, 4, 44),
		Karaoke: rgb(255, 112, 210), Video: rgb(204, 170, 255), Audio: rgb(255, 220, 120),
		BadgeText: rgb(40, 4, 44),
		Shadow:    rgba(0, 0, 0, 160),
		Glow:      [3]color.NRGBA{rgb(255, 20, 160), rgb(140, 50, 255), rgb(200, 120, 10)},
		GlowAlpha: 0.33,
		Sparkles:  18,
		Band:      rgba(0, 0, 0, 110),
	},
}

// ThemeNames lists themes in a stable order (admin menu).
func ThemeNames() []string {
	names := make([]string, 0, len(Themes))
	for n := range Themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// PaletteFor returns the named theme, falling back to "moderno".
func PaletteFor(name string) Palette {
	if p, ok := Themes[strings.ToLower(strings.TrimSpace(name))]; ok {
		return p
	}
	return Themes["moderno"]
}

// ---------- fonts ----------

// Fonts are embedded: the rockola never has internet. Both are SIL OFL 1.1
// (licenses next to them in fonts/). They are static instances of the
// Google Fonts variable fonts (github.com/google/fonts, ofl/nunito wght=600
// and ofl/fredoka wght=700 wdth=100), cut to Latin + punctuation + arrows
// with fontTools (varLib.instancer, subset): ~75 KB together.
//
//   - Nunito SemiBold: regular text and long lists. Rounded, open and very
//     readable from across the room.
//   - Fredoka Bold: every bold text (titles, headings, the selected row,
//     badges, keypad). Round and festive.
//
// Glyphs these fonts lack (arrows, ▶ ⏸ ♪ ● ⚠, emoji) still come from Fyne's
// fallback chain: Noto Sans, the emoji font and the system fonts, exactly as
// before (TestFontGlyphs).
var (
	//go:embed fonts/Nunito-SemiBold.ttf
	nunitoSemiBold []byte
	//go:embed fonts/Fredoka-Bold.ttf
	fredokaBold []byte

	fontRegular = fyne.NewStaticResource("Nunito-SemiBold.ttf", nunitoSemiBold)
	fontBold    = fyne.NewStaticResource("Fredoka-Bold.ttf", fredokaBold)
)

// partyTheme is Fyne's default theme with the embedded fonts.
type partyTheme struct{ fyne.Theme }

// newPartyTheme wraps Fyne's default theme.
func newPartyTheme() fyne.Theme { return partyTheme{theme.DefaultTheme()} }

func (t partyTheme) Font(s fyne.TextStyle) fyne.Resource {
	switch {
	case s.Monospace, s.Symbol, s.Italic:
		return t.Theme.Font(s)
	case s.Bold:
		return fontBold
	}
	return fontRegular
}
