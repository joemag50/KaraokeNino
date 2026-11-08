package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAddsNewSongsAndHidesMissing(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})

	root := t.TempDir()
	// Imported songs that are on disk (one with different case, as Windows leaves them).
	touch(t, root, "karaokes/Juan Gabriel/Amor Eterno.AVI")
	touch(t, root, "karaokes/Juan Gabriel/Querida .mpg")
	touch(t, root, "karaokes/Juan Gabriel/Adiós amor.mpg")
	touch(t, root, "karaokes/zoé/Labios Rotos.mpg")
	touch(t, root, "musica/CELIA CRUZ/La Vida Es Un Carnaval.mp3")
	// New: artist folder next to existing karaoke artists -> KARAOKES.
	touch(t, root, "karaokes/Natalia Lafourcade/Hasta la Raíz.mp4")
	touch(t, root, "karaokes/Natalia Lafourcade/images.jpg") // not media
	// New top-level folder -> new genre.
	touch(t, root, "cumbias/Los Ángeles Azules/Nunca Es Suficiente.mp3")
	// CDG karaoke: the mp3 twin is not a separate song.
	touch(t, root, "karaokes/Mix/Track 01.cdg")
	touch(t, root, "karaokes/Mix/Track 01.mp3")
	// Ignored.
	touch(t, root, "System Volume Information/x.mp3")
	// Missing from disk: "Corazón Espinado".

	res, err := Scan(ctx, store, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Missing != 1 || res.Renamed != 1 {
		t.Fatalf("resultado: %+v", res)
	}

	got, _ := store.Search(ctx, "hasta la raiz", nil, 5)
	if len(got) != 1 || got[0].Genre != "KARAOKES" || !got[0].Karaoke || got[0].Artist != "Natalia Lafourcade" {
		t.Fatalf("nueva canción: %+v", got)
	}
	got, _ = store.Search(ctx, "nunca suficiente", nil, 5)
	if len(got) != 1 || got[0].Genre != "CUMBIAS" {
		t.Fatalf("nuevo género: %+v", got)
	}
	if got, _ := store.Search(ctx, "corazon espinado", nil, 5); len(got) != 0 {
		t.Fatalf("faltante visible: %+v", got)
	}
	if got, _ := store.Search(ctx, "amor eterno", nil, 5); len(got) != 1 || got[0].File != "karaokes/Juan Gabriel/Amor Eterno.AVI" {
		t.Fatalf("mayúsculas: %+v", got)
	}

	// Second scan: nothing new. Then the song comes back.
	touch(t, root, "2026/karaokes 2026/# Seguro cantas una/Corazón Espinado - Maná.avi")
	res, err = Scan(ctx, store, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || res.Restored != 1 || res.Missing != 0 {
		t.Fatalf("segundo escaneo: %+v", res)
	}
}

func TestScanDoesNotHideCatalogOnWrongDisk(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	var data = "KARAOKES\r\n\tA|D:\\master.1000\\karaokes\\A\\|\r\n"
	for i := 0; i < 300; i++ {
		data += "\t\tD:\\master.1000\\karaokes\\A\\|Song " + itoa(int64(i)) + "|MP3\r\n"
	}
	importInto(t, store, data, ImportOptions{})
	root := t.TempDir()
	touch(t, root, "otro/Otro/x.mp3")
	res, err := Scan(ctx, store, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.MissingSkipped || res.Missing != 0 {
		t.Fatalf("resultado: %+v", res)
	}
}

func TestDuplicates(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	data := "KARAOKES\r\n" +
		"\tReyli|D:\\master.1000\\karaokes\\Reyli\\|\r\n" +
		"\t\tD:\\master.1000\\karaokes\\Reyli\\|Amor Del Bueno|MPG\r\n" +
		"\t# Mix|D:\\master.1000\\karaokes\\# Mix\\|\r\n" +
		"\t\tD:\\master.1000\\karaokes\\# Mix\\|AMOR DEL BUENO - Reyli|AVI\r\n"
	importInto(t, store, data, ImportOptions{})
	d, err := store.Duplicates(ctx, 10)
	if err != nil || len(d) != 1 || len(d[0]) != 2 {
		t.Fatalf("duplicados: %v %+v", err, d)
	}
	st, _ := store.Stats(ctx)
	if st.DupGroups != 1 {
		t.Fatalf("stats: %+v", st)
	}
}
