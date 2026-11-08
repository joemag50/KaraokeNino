package library

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Big folders are remembered; folders added or renamed later still resolve.
// (Only meaningful on case-sensitive file systems such as the rockola's.)
func TestResolveRemembersBigFoldersButSeesChanges(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for i := 0; i < minCachedEntries+5; i++ {
		mk(fmt.Sprintf("MUSICA/Artista %02d/Song.MP3", i))
	}
	check := func(want, ask string) {
		t.Helper()
		got, err := Resolve(filepath.Join(root, ask))
		if err != nil {
			t.Fatalf("%s: %v", ask, err)
		}
		if !same(t, got, want) {
			t.Fatalf("%s -> %s, quería %s", ask, got, want)
		}
	}
	check(filepath.Join(root, "MUSICA/Artista 03/Song.MP3"), "musica/artista 03/song.mp3")

	// New artist folder after the listing was remembered.
	check(mk("MUSICA/Nuevo/Otra.AVI"), "musica/nuevo/otra.avi")

	// Folder renamed (only the case changed).
	old := filepath.Join(root, "MUSICA/Artista 04")
	if err := os.Rename(old, filepath.Join(root, "MUSICA/ARTISTA 04 tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "MUSICA/ARTISTA 04 tmp"), filepath.Join(root, "MUSICA/ARTISTA 04")); err != nil {
		t.Fatal(err)
	}
	check(filepath.Join(root, "MUSICA/ARTISTA 04/Song.MP3"), "musica/artista 04/song.mp3")

	if _, err := Resolve(filepath.Join(root, "musica/no existe/x.mp3")); !os.IsNotExist(err) {
		t.Fatalf("esperaba ErrNotExist, got %v", err)
	}
}

func same(t *testing.T, a, b string) bool {
	t.Helper()
	sa, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(sa, sb)
}
