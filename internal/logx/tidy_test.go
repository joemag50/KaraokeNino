package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCapFileKeepsTheEnd(t *testing.T) {
	p := filepath.Join(t.TempDir(), "crash.log")
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("línea de prueba número tal\n")
	}
	b.WriteString("la última\n")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	freed, err := CapFile(p, 4096)
	if err != nil || freed <= 0 {
		t.Fatalf("freed=%d err=%v", freed, err)
	}
	got, _ := os.ReadFile(p)
	if len(got) > 2048 || !strings.HasSuffix(string(got), "la última\n") || !strings.HasPrefix(string(got), "línea") {
		t.Fatalf("quedó %d bytes: %q", len(got), got)
	}
	if freed, _ := CapFile(p, 4096); freed != 0 {
		t.Fatal("recortó un archivo chico")
	}
}

func TestTidyRemovesOldRotatedLogs(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.Repeat("x\n", size/2)), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		_ = os.Chtimes(p, when, when)
	}
	write("karaokenino.log", 100, 0)
	write("karaokenino.log.1", 100, 24*time.Hour)
	write("karaokenino.log.3", 100, 200*24*time.Hour)
	write("kiosk.log", 10000, 0)
	write("notas.txt", 10, 365*24*time.Hour)

	if n, _, err := Tidy(dir, 90*24*time.Hour, 4096, true); err != nil || n != 2 {
		t.Fatalf("simulado: n=%d err=%v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "karaokenino.log.3")); err != nil {
		t.Fatal("el simulacro borró")
	}
	n, freed, err := Tidy(dir, 90*24*time.Hour, 4096, false)
	if err != nil || n != 2 || freed < 100 {
		t.Fatalf("n=%d freed=%d err=%v", n, freed, err)
	}
	for name, want := range map[string]bool{"karaokenino.log": true, "karaokenino.log.1": true, "karaokenino.log.3": false, "notas.txt": true} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Fatalf("%s existe=%v", name, err == nil)
		}
	}
	if st, _ := os.Stat(filepath.Join(dir, "kiosk.log")); st.Size() > 2048 {
		t.Fatalf("kiosk.log mide %d", st.Size())
	}
}
