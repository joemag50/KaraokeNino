package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingKeepsBoundedFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logs", "k.log")
	r, err := NewRotating(p, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 39) + "\n"
	for i := 0; i < 20; i++ {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	r.Close()
	for _, f := range []string{p, p + ".1", p + ".2"} {
		st, err := os.Stat(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if st.Size() > 100 {
			t.Fatalf("%s mide %d", f, st.Size())
		}
	}
	if _, err := os.Stat(p + ".3"); !os.IsNotExist(err) {
		t.Fatal("se guardaron demasiadas copias")
	}
	if got := Tail(p, 1); len(got) != 1 || got[0] != strings.TrimSpace(line) {
		t.Fatalf("tail: %q", got)
	}
}
