package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrunePlaysKeepsAllTimeTop(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	ev, _ := store.StartEvent(ctx, "boda")
	old := time.Now().AddDate(-2, 0, 0).Unix()
	// Track 1: 3 old plays + 1 new; track 2: 2 new.
	for i := 0; i < 3; i++ {
		if _, err := store.q.ExecContext(ctx, `INSERT INTO plays(track_id,event_id,played_at) VALUES(1,?,?)`, ev, old); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{1, 2, 2} {
		if err := store.RecordPlay(ctx, id, ev); err != nil {
			t.Fatal(err)
		}
	}
	n, err := store.PrunePlays(ctx, time.Now().AddDate(0, -12, 0))
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	top, err := store.Top(ctx, 0, 10)
	if err != nil || len(top) != 2 || top[0].ID != 1 || top[0].Plays != 4 || top[1].Plays != 2 {
		t.Fatalf("top total: %v %+v", err, top)
	}
	if top, _ := store.Top(ctx, ev, 10); len(top) != 2 || top[0].ID != 2 || top[0].Plays != 2 {
		t.Fatalf("top del evento: %+v", top)
	}
	// Pruning again folds nothing twice.
	if n, _ := store.PrunePlays(ctx, time.Now().AddDate(0, -12, 0)); n != 0 {
		t.Fatalf("segunda poda: %d", n)
	}
	if top, _ := store.Top(ctx, 0, 1); top[0].Plays != 4 {
		t.Fatalf("doble conteo: %+v", top)
	}
	if st, _ := store.Stats(ctx); st.Plays != 6 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestPurgeMissingOnlyOldOnes(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	all, _ := store.Search(ctx, "amor eterno", nil, 1)
	gone, keep := all[0].ID, int64(0)
	if err := store.SetMissing(ctx, gone, true); err != nil {
		t.Fatal(err)
	}
	lv, _ := store.Search(ctx, "carnaval", nil, 1)
	keep = lv[0].ID
	if err := store.SetMissing(ctx, keep, true); err != nil {
		t.Fatal(err)
	}
	// "Amor Eterno" has been missing for 3 months, "Carnaval" since today.
	if _, err := store.q.ExecContext(ctx, `UPDATE tracks SET missing_since=? WHERE id=?`, time.Now().AddDate(0, -3, 0).Unix(), gone); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPlay(ctx, gone, 0); err != nil {
		t.Fatal(err)
	}
	cut := time.Now().AddDate(0, 0, -30)
	ms, err := store.MissingTracks(ctx, cut, 10)
	if err != nil || len(ms) != 1 || ms[0].ID != gone || ms[0].Since.IsZero() {
		t.Fatalf("faltantes viejas: %v %+v", err, ms)
	}
	if ms, _ := store.MissingTracks(ctx, time.Time{}, 10); len(ms) != 2 {
		t.Fatalf("todas las faltantes: %+v", ms)
	}
	n, err := store.PurgeMissing(ctx, cut)
	if err != nil || n != 1 {
		t.Fatalf("purga: n=%d err=%v", n, err)
	}
	var left int
	_ = store.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracks WHERE id=?`, gone).Scan(&left)
	if left != 0 {
		t.Fatal("no se borró")
	}
	if top, _ := store.Top(ctx, 0, 10); len(top) != 0 {
		t.Fatalf("quedó en el ranking: %+v", top)
	}
	if problems, err := store.IntegrityCheck(ctx); err != nil || len(problems) != 0 {
		t.Fatalf("integridad: %v %v", err, problems)
	}
	// Restoring clears the date.
	if err := store.SetMissing(ctx, keep, false); err != nil {
		t.Fatal(err)
	}
	if ms, _ := store.MissingTracks(ctx, time.Time{}, 10); len(ms) != 0 {
		t.Fatalf("sigue faltante: %+v", ms)
	}
	if _, err := store.PurgeMissing(ctx, time.Time{}); err == nil {
		t.Fatal("purga sin fecha")
	}
}

func TestShortSearchesReturnMatches(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	for _, q := range []string{"a", "la", "la vi", "zo"} {
		got, err := store.Search(ctx, q, nil, 60)
		if err != nil || len(got) == 0 {
			t.Fatalf("%q: %v %+v", q, err, got)
		}
	}
	got, _ := store.Search(ctx, "la", []int64{999}, 60)
	if len(got) != 0 {
		t.Fatalf("filtro de género: %+v", got)
	}
}

func TestScanInBatches(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	root := t.TempDir()
	for _, f := range []string{"a/Uno/1.mp3", "a/Uno/2.mp3", "a/Dos/3.avi", "b/Tres/4.mpg", "b/Tres/5.mp3", "b/Tres/6.MP4", "b/Cuatro/7.mp3"} {
		touch(t, root, f)
	}
	res, err := ScanWith(ctx, store, root, ScanOptions{Fast: true, BatchSize: 3})
	if err != nil || res.Added != 7 || res.Files != 7 {
		t.Fatalf("%v %+v", err, res)
	}
	if got, _ := store.Search(ctx, "6", nil, 5); len(got) != 1 || got[0].Artist != "Tres" {
		t.Fatalf("búsqueda: %+v", got)
	}
	gs, _ := store.Genres(ctx, KindAll)
	if len(gs) != 2 {
		t.Fatalf("géneros: %+v", gs)
	}
	if n, _ := store.ArtistCount(ctx, gs[1].ID, KindAll); n != 2 {
		t.Fatalf("artistas de B: %d", n)
	}
	// Case change on disk (copied from Windows again): fixed, not duplicated.
	if err := os.Rename(filepath.Join(root, "b/Tres/5.mp3"), filepath.Join(root, "b/Tres/5.MP3")); err != nil {
		t.Fatal(err)
	}
	res, err = ScanWith(ctx, store, root, ScanOptions{Fast: true, BatchSize: 3})
	if err != nil || res.Added != 0 || res.Missing != 0 {
		t.Fatalf("re-escaneo: %v %+v", err, res)
	}
	if got, _ := store.Search(ctx, "5", nil, 5); len(got) != 1 || got[0].File != "b/Tres/5.MP3" {
		t.Fatalf("mayúsculas: %+v", got)
	}
}

// Unplugging the disk mid-scan must not hide the songs not read yet.
func TestScanDiskGoneMidwayHidesNothing(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	root := t.TempDir()
	touch(t, root, "karaokes/Juan Gabriel/Amor Eterno.avi")
	touch(t, root, "musica/CELIA CRUZ/La Vida Es Un Carnaval.mp3")
	calls := 0
	res, err := ScanWith(ctx, store, root, ScanOptions{Busy: func() bool {
		// Called before each folder is read: after the root was listed,
		// "musica" vanishes as if the USB cable came out.
		if calls++; calls == 2 {
			os.RemoveAll(filepath.Join(root, "musica"))
		}
		return false
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.MissingSkipped || res.ReadErrors == 0 || res.Missing != 0 {
		t.Fatalf("resultado: %+v", res)
	}
	if got, _ := store.Search(ctx, "carnaval", nil, 5); len(got) != 1 {
		t.Fatal("se ocultó una canción por el disco desconectado")
	}
}

func TestBackupIsAUsableCatalog(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	p := filepath.Join(t.TempDir(), "respaldo.db")
	if err := store.Backup(ctx, p); err != nil {
		t.Fatal(err)
	}
	b, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if got, _ := b.Search(ctx, "amor eterno", nil, 5); len(got) != 1 {
		t.Fatalf("respaldo: %+v", got)
	}
	if err := store.Backup(ctx, p); err == nil {
		t.Fatal("sobrescribió un respaldo")
	}
}

func TestIsThumbName(t *testing.T) {
	if IsThumbName("nota.jpg") || IsThumbName(".thumb-123.jpg") || !IsThumbName(ThumbName("a", "b")) {
		t.Fatal("IsThumbName")
	}
}
