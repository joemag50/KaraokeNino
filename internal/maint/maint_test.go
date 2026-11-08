package maint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/thumbs"
)

func testStore(t *testing.T) *catalog.Store {
	t.Helper()
	ctx := context.Background()
	s, err := catalog.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	err = s.WithTx(ctx, func(tx *catalog.Store) error {
		g, err := tx.UpsertGenre(ctx, "KARAOKES")
		if err != nil {
			return err
		}
		a, err := tx.UpsertArtist(ctx, g, "Juan Gabriel", "karaokes/Juan Gabriel", "karaokes/Juan Gabriel/images.jpg")
		if err != nil {
			return err
		}
		if _, err := tx.UpsertTrack(ctx, catalog.NewTrack{ArtistID: a, Name: "Amor Eterno", File: "karaokes/Juan Gabriel/Amor Eterno.avi", MediaType: "AVI"}); err != nil {
			return err
		}
		if err := tx.RefreshCounts(ctx); err != nil {
			return err
		}
		return tx.RebuildSearch(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The names CleanThumbs keeps are the ones internal/thumbs looks up.
func TestThumbNameIsWhatThumbsUses(t *testing.T) {
	dir := t.TempDir()
	folder, cover := "karaokes/Juan Gabriel", "karaokes/Juan Gabriel/images.jpg"
	p := filepath.Join(dir, catalog.ThumbName(folder, cover))
	if err := os.WriteFile(p, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := thumbs.New(dir, 240).Get(folder+"|"+cover, nil); got != p {
		t.Fatalf("thumbs busca %q, mantenimiento conserva %q", got, p)
	}
}

func TestCleanThumbsRemovesOnlyOrphans(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	dir := t.TempDir()
	keep := filepath.Join(dir, catalog.ThumbName("karaokes/Juan Gabriel", "karaokes/Juan Gabriel/images.jpg"))
	orphan := filepath.Join(dir, catalog.ThumbName("karaokes/Ya No Está", ""))
	tmpOld := filepath.Join(dir, ".thumb-111.jpg")
	tmpNew := filepath.Join(dir, ".thumb-222.jpg")
	other := filepath.Join(dir, "LEEME.txt")
	for _, p := range []string{keep, orphan, tmpOld, tmpNew, other} {
		if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(tmpOld, old, old)

	res, err := CleanThumbs(ctx, s, dir, true)
	if err != nil || res.Removed != 2 || res.Files != 2 {
		t.Fatalf("simulado: %v %+v", err, res)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("el simulacro borró")
	}
	res, err = CleanThumbs(ctx, s, dir, false)
	if err != nil || res.Removed != 2 || res.Freed != 10 {
		t.Fatalf("%v %+v", err, res)
	}
	for p, want := range map[string]bool{keep: true, orphan: false, tmpOld: false, tmpNew: true, other: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Fatalf("%s existe=%v", filepath.Base(p), err == nil)
		}
	}
}

func TestCleanThumbsRefusesWhenNothingMatches(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	dir := t.TempDir()
	for i := 0; i < 60; i++ {
		p := filepath.Join(dir, catalog.ThumbName("otro esquema", string(rune('a'+i))))
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CleanThumbs(ctx, s, dir, false); err == nil {
		t.Fatal("borró todas las miniaturas")
	}
	if n, _ := os.ReadDir(dir); len(n) != 60 {
		t.Fatalf("quedan %d", len(n))
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"20260101-1000", "20260301-1000", "20260201-1000", "20260401-1000"} {
		if err := os.WriteFile(filepath.Join(dir, "catalog-"+d+".db"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, _, err := PruneBackups(dir, 2, false)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.db"))
	if len(left) != 2 || !strings.Contains(left[0], "0301") || !strings.Contains(left[1], "0401") {
		t.Fatalf("quedaron %v", left)
	}
}

func TestRunRoutine(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	logs := t.TempDir()
	if err := os.WriteFile(filepath.Join(logs, "kiosk.log"), []byte(strings.Repeat("línea\n", 2<<20)), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(ctx, s, Options{ThumbsDir: t.TempDir(), LogDir: logs, MusicRoot: t.TempDir(), PrunePlaysMonths: 12, Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(rep.Lines, "\n")
	for _, want := range []string{"Catálogo sano", "Historial", "Faltantes", "Índice de búsqueda", "ANALYZE", "VACUUM", "Miniaturas", "Registros: 1", "Disco de música"} {
		if !strings.Contains(text, want) {
			t.Fatalf("falta %q en:\n%s", want, text)
		}
	}
	if st, _ := os.Stat(filepath.Join(logs, "kiosk.log")); st.Size() > 8<<20 {
		t.Fatalf("kiosk.log mide %d", st.Size())
	}
	if _, err := Run(ctx, s, Options{Full: true, DryRun: true, PurgeMissing: true}); err != nil {
		t.Fatal(err)
	}
}
