package ui

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestKeyHintCaps(t *testing.T) {
	a := startedApp(t)
	h := newKeyHint(&a.pal, Rect{0, 0, 900, 30}, 18, a.pal.Text, fyne.TextAlignLeading)
	h.set(key("8") + key("2") + " elegir  ·  " + key("Intro") + " aceptar")
	if h.Text != "82 elegir  ·  Intro aceptar" {
		t.Fatalf("texto %q", h.Text)
	}
	caps := 0
	for i, c := range h.caps {
		if c.Visible() {
			caps++
			if h.glyphs[i].Text == "" {
				t.Fatal("cada tecla lleva su nombre")
			}
		}
	}
	if caps != 3 {
		t.Fatalf("deben dibujarse 3 teclas, hay %d", caps)
	}
	// A title with brackets is plain text, not a key.
	h.set("Sigue: Amor Eterno [Karaoke]")
	for _, c := range h.caps {
		if c.Visible() {
			t.Fatal("los corchetes de un título no son teclas")
		}
	}
	if plainKeys(fullHint) == fullHint {
		t.Fatal("la pista del video debe marcar sus teclas")
	}
}
