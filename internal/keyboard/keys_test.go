package keyboard

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestFyneKey(t *testing.T) {
	cases := map[string]fyne.KeyName{
		"KEY_KP8":        fyne.Key8,
		"KEY_KP0":        fyne.Key0,
		"KEY_KPENTER":    fyne.KeyEnter,
		"KEY_KPDOT":      fyne.KeyPeriod,
		"KEY_KPPLUS":     fyne.KeyPlus,
		"KEY_KPASTERISK": fyne.KeyAsterisk,
		"KEY_M":          fyne.KeyM,
		"key_m":          fyne.KeyM,
		"KEY_NOPE":       "",
	}
	for in, want := range cases {
		if got := FyneKey(in); got != want {
			t.Errorf("FyneKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestActions(t *testing.T) {
	got := Actions(map[string]string{"up": "KEY_KP8", "ok": "KEY_KPENTER", "bad": "???"})
	if got[fyne.Key8] != "up" || got[fyne.KeyEnter] != "ok" || len(got) != 2 {
		t.Fatalf("Actions = %v", got)
	}
}
