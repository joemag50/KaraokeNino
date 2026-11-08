package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "state.json")
	s := NewStore(p)
	max := 90
	in := State{Volume: 70, MaxVolume: &max, KidsMode: true}
	if err := s.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if out.Volume != 70 || *out.MaxVolume != 90 || !out.KidsMode {
		t.Fatalf("%+v", out)
	}
}

func TestLoadBrokenFileStartsClean(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := NewStore(p).Load()
	if err == nil || st.Volume != 0 {
		t.Fatalf("%v %+v", err, st)
	}
	if _, err := os.Stat(p + ".roto"); err != nil {
		t.Fatal("no se guardó la copia del archivo roto")
	}
}

func TestMissingFile(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "nada.json")).Load()
	if err != nil || st.Volume != 0 {
		t.Fatalf("%v %+v", err, st)
	}
}

// A state.json from an older version still has the queue: it loads fine,
// the queue is ignored and the next save drops it.
func TestOldQueueIsIgnored(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	old := `{"volume": 65, "current": {"track_id": 589, "title": "Hombre libre"}, "queue": [{"track_id": 1}]}`
	if err := os.WriteFile(p, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewStore(p)
	st, err := s.Load()
	if err != nil || st.Volume != 65 {
		t.Fatalf("%v %+v", err, st)
	}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "current") || strings.Contains(string(b), "queue") {
		t.Fatalf("la cola vieja sigue guardada: %s", b)
	}
}
