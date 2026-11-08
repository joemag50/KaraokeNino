package keyboard

import (
	"strings"

	"fyne.io/fyne/v2"
)

// Fyne reports keypad digits as plain digits and keypad Enter as KeyEnter
// (main Enter is KeyReturn), so evdev names map onto those.
var evdevToFyne = map[string]fyne.KeyName{
	"KEY_KPENTER":    fyne.KeyEnter,
	"KEY_ENTER":      fyne.KeyReturn,
	"KEY_KPDOT":      fyne.KeyPeriod,
	"KEY_KPPLUS":     fyne.KeyPlus,
	"KEY_KPMINUS":    fyne.KeyMinus,
	"KEY_KPASTERISK": fyne.KeyAsterisk,
	"KEY_KPSLASH":    fyne.KeySlash,
	"KEY_UP":         fyne.KeyUp,
	"KEY_DOWN":       fyne.KeyDown,
	"KEY_LEFT":       fyne.KeyLeft,
	"KEY_RIGHT":      fyne.KeyRight,
	"KEY_PAGEUP":     fyne.KeyPageUp,
	"KEY_PAGEDOWN":   fyne.KeyPageDown,
	"KEY_SPACE":      fyne.KeySpace,
	"KEY_ESC":        fyne.KeyEscape,
	"KEY_BACKSPACE":  fyne.KeyBackspace,
}

// FyneKey converts an evdev name (KEY_KP8, KEY_M, KEY_KPENTER...) to the
// key name Fyne delivers. Returns "" if unknown.
func FyneKey(evdev string) fyne.KeyName {
	evdev = strings.ToUpper(strings.TrimSpace(evdev))
	if k, ok := evdevToFyne[evdev]; ok {
		return k
	}
	rest, ok := strings.CutPrefix(evdev, "KEY_KP")
	if !ok {
		rest, ok = strings.CutPrefix(evdev, "KEY_")
	}
	if ok && len(rest) == 1 && (rest[0] >= '0' && rest[0] <= '9' || rest[0] >= 'A' && rest[0] <= 'Z') {
		return fyne.KeyName(rest)
	}
	return ""
}

// Actions builds key -> logical action ("up", "ok", ...) from config bindings.
func Actions(bindings map[string]string) map[fyne.KeyName]string {
	out := make(map[fyne.KeyName]string, len(bindings))
	for action, evdev := range bindings {
		if k := FyneKey(evdev); k != "" {
			out[k] = action
		}
	}
	return out
}
