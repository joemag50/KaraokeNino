package player

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathIgnoresCase(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "Juan Gabriel", "Amor Eterno.AVI")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolvePath(filepath.Join(dir, "JUAN GABRIEL", "amor eterno.avi"))
	if err != nil {
		t.Fatal(err)
	}
	// On case-insensitive filesystems (macOS) the original spelling already exists.
	if !sameFile(t, got, real) {
		t.Fatalf("got %q, want %q", got, real)
	}

	if _, err := ResolvePath(filepath.Join(dir, "Juan Gabriel", "No Existe.avi")); !os.IsNotExist(err) {
		t.Fatalf("esperaba ErrNotExist, got %v", err)
	}
}

func sameFile(t *testing.T, a, b string) bool {
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
