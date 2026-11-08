package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/josegamboa/karaokenino/internal/config"
)

func TestDetectFindsDiskUnderAnyMount(t *testing.T) {
	media := t.TempDir()
	root := filepath.Join(media, "jose", "DISCO KARAOKE", "Master.1000")
	if err := os.MkdirAll(filepath.Join(root, "karaokes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An empty stale mount point listed first must be skipped.
	stale := filepath.Join(t.TempDir(), "master.1000")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}

	l := New(config.LibraryConfig{Roots: []string{stale}, DirName: "master.1000", SearchIn: []string{media}})
	got, err := l.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root=%q, quería %q", got, root)
	}
	if abs := l.Abs("karaokes/x.avi"); abs != filepath.Join(root, "karaokes", "x.avi") {
		t.Fatalf("Abs=%q", abs)
	}
	if rel, ok := l.Rel(filepath.Join(root, "karaokes", "x.avi")); !ok || rel != "karaokes/x.avi" {
		t.Fatalf("Rel=%q %v", rel, ok)
	}
}

func TestDetectNotFound(t *testing.T) {
	l := New(config.LibraryConfig{DirName: "master.1000", SearchIn: []string{t.TempDir()}})
	if _, err := l.Detect(); err != ErrNotFound {
		t.Fatalf("err=%v", err)
	}
}
