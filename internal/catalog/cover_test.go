package catalog

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
}

// Musica.data says "karaokes/zoé/images.jpg"; the Linux disk has
// "Karaokes/ZOÉ/IMAGES.JPG". The cover must still be found.
func TestCoverFoundIgnoringCase(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Karaokes", "ZOÉ")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "IMAGES.JPG")
	writePNG(t, want)
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	c := NewCovers(abs, "")
	for _, tc := range []struct{ folder, cover string }{
		{"karaokes/zoé", "karaokes/zoé/images.jpg"}, // cover listed in Musica.data
		{"karaokes/zoé", ""},                        // only the folder: look for images.jpg & co.
	} {
		c.Reset()
		got := c.For(tc.folder, tc.cover)
		st1, err1 := os.Stat(got)
		st2, _ := os.Stat(want)
		if err1 != nil || !os.SameFile(st1, st2) {
			t.Errorf("For(%q,%q) = %q, quería %q", tc.folder, tc.cover, got, want)
		}
	}
	if got := c.For("karaokes/no-existe", ""); got != "" {
		t.Errorf("sin carpeta: %q", got)
	}
}

// The cover listed in Musica.data may be gone or be a web page saved as
// ".jpg": then another image of the same folder is used.
func TestCoverFallsBackToAnotherImageInTheFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "karaokes 2026", "$ Adriel Favela")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "unnamed.jpg"), []byte("<html>no es imagen</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "cover.jpg"), nil, 0o644) // empty: skipped too
	want := filepath.Join(dir, "Adriel 2026.PNG")
	writePNG(t, want)
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	c := NewCovers(abs, "")
	for _, cover := range []string{"karaokes 2026/$ Adriel Favela/unnamed.jpg", "karaokes 2026/$ Adriel Favela/borrada.jpg"} {
		c.Reset()
		if got := c.For("karaokes 2026/$ Adriel Favela", cover); got != want {
			t.Errorf("For(%q) = %q, quería %q", cover, got, want)
		}
	}
	// No folder in the catalog: the folder of the listed cover is used.
	c.Reset()
	if got := c.For("", "karaokes 2026/$ Adriel Favela/unnamed.jpg"); got != want {
		t.Errorf("sin carpeta: %q", got)
	}
}
