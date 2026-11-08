package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// The screen font draws these symbols. A text with any other symbol is
// drawn broken (boxes, even for its spaces), so none may appear in the UI.
const okSymbols = "↑↓←→▶⏸♪🔇⚠…—«»●·−×"

func TestOnlyDrawableSymbols(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, r := range line {
				if r < 128 || unicode.Is(unicode.Latin, r) || strings.ContainsRune(okSymbols, r) ||
					strings.ContainsRune("¿¡°ºª", r) {
					continue
				}
				t.Errorf("%s:%d usa %q, que la fuente no dibuja: %s", f, i+1, r, strings.TrimSpace(line))
			}
		}
	}
}
