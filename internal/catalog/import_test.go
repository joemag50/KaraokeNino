package catalog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josegamboa/karaokenino/internal/config"
)

// sample mirrors the real Musica.data: Latin-1 bytes (0xF3 = ó, 0xE9 = é), CRLF,
// several tracks sharing the same folder, a trailing space in a file name,
// a compilation folder and malformed lines.
var sample = "KARAOKES\r\n" +
	"\tJuan Gabriel|D:\\master.1000\\karaokes\\Juan Gabriel\\|D:\\master.1000\\karaokes\\Juan Gabriel\\images.jpg\r\n" +
	"\t\tD:\\master.1000\\karaokes\\Juan Gabriel\\|Amor Eterno|AVI\r\n" +
	"\t\tD:\\master.1000\\karaokes\\Juan Gabriel\\|Querida |MPG\r\n" +
	"\t\tD:\\master.1000\\karaokes\\Juan Gabriel\\|Adi\xf3s amor|MPG\r\n" +
	"\t\tD:\\master.1000\\karaokes\\Juan Gabriel\\|Sin tipo|\r\n" +
	"\tZo\xe9|D:\\master.1000\\karaokes\\zo\xe9\\|\r\n" +
	"\t\tD:\\master.1000\\karaokes\\zo\xe9\\|Labios Rotos|MPG\r\n" +
	"\t# Seguro cantas una|D:\\master.1000\\2026\\karaokes 2026\\# Seguro cantas una\\|\r\n" +
	"\t\tD:\\master.1000\\2026\\karaokes 2026\\# Seguro cantas una\\|Coraz\xf3n Espinado - Man\xe1|AVI\r\n" +
	"MUSICA\r\n" +
	"\tCelia Cruz|D:\\master.1000\\musica\\CELIA CRUZ\\|\r\n" +
	"\t\tD:\\master.1000\\musica\\CELIA CRUZ\\|La Vida Es Un Carnaval|MP3\r\n" +
	"\t\tD:\\master.1000\\musica\\CELIA CRUZ\\|DAT|\r\n"

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Library.WindowsRoot = `D:\master.1000`
	return cfg
}

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func importInto(t *testing.T, store *Store, data string, opts ImportOptions) ImportResult {
	t.Helper()
	musica := filepath.Join(t.TempDir(), "Musica.data")
	if err := os.WriteFile(musica, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ImportMusicaData(context.Background(), testConfig(), musica, store, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func genreID(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	gs, err := s.Genres(context.Background(), KindAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gs {
		if g.Name == name {
			return g.ID
		}
	}
	t.Fatalf("no hay género %q en %+v", name, gs)
	return 0
}

func TestImportKeepsEveryTrackWithRelativePath(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	res := importInto(t, store, sample, ImportOptions{})
	if res.Tracks != 6 || res.Artists != 4 || res.Genres != 2 || res.Skipped != 2 {
		t.Fatalf("resultado inesperado: %s", res)
	}
	artists, err := store.Artists(ctx, genreID(t, store, "KARAOKES"), KindAll, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Sorted ignoring the leading "#" and accents.
	var names []string
	for _, a := range artists {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "Juan Gabriel,# Seguro cantas una,Zoé" {
		t.Fatalf("artistas: %v", names)
	}
	if artists[0].Cover != "karaokes/Juan Gabriel/images.jpg" || artists[0].Tracks != 3 {
		t.Fatalf("artista: %+v", artists[0])
	}
	tracks, err := store.Tracks(ctx, artists[0].ID, KindAll)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Adiós amor":  "karaokes/Juan Gabriel/Adiós amor.mpg",
		"Amor Eterno": "karaokes/Juan Gabriel/Amor Eterno.avi",
		"Querida":     "karaokes/Juan Gabriel/Querida .mpg",
	}
	if len(tracks) != len(want) {
		t.Fatalf("pistas: %+v", tracks)
	}
	for _, tr := range tracks {
		if want[tr.Title] != tr.File {
			t.Errorf("%q: file=%q, quería %q", tr.Title, tr.File, want[tr.Title])
		}
	}

	// Compilation folder: "Title - Artist" is split.
	comp, _ := store.Tracks(ctx, artists[1].ID, KindAll)
	if len(comp) != 1 || comp[0].Title != "Corazón Espinado" || comp[0].Artist != "Maná" {
		t.Fatalf("compilación: %+v", comp)
	}
}

func TestReimportKeepsSongNumbers(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	before, err := store.Search(ctx, "amor eterno", nil, 5)
	if err != nil || len(before) != 1 {
		t.Fatalf("search: %v %+v", err, before)
	}
	if err := store.RecordPlay(ctx, before[0].ID, 0); err != nil {
		t.Fatal(err)
	}

	// Second import without Celia Cruz.
	smaller := sample[:strings.Index(sample, "MUSICA\r\n")]
	res := importInto(t, store, smaller, ImportOptions{})
	if res.Removed != 1 {
		t.Fatalf("quitadas=%d", res.Removed)
	}
	after, err := store.TrackByID(ctx, before[0].ID)
	if err != nil || after.Title != "Amor Eterno" {
		t.Fatalf("el número cambió: %v %+v", err, after)
	}
	top, err := store.Top(ctx, 0, 5)
	if err != nil || len(top) != 1 || top[0].Plays != 1 {
		t.Fatalf("ranking: %v %+v", err, top)
	}
	if gs, _ := store.Genres(ctx, KindAll); len(gs) != 1 {
		t.Fatalf("géneros: %+v", gs)
	}
}

func TestImportKaraokeOnly(t *testing.T) {
	res := importInto(t, newStore(t), sample, ImportOptions{KaraokeOnly: true})
	if res.Genres != 1 || res.Artists != 3 || res.Tracks != 5 {
		t.Fatalf("resultado inesperado: %s", res)
	}
}

func TestSearchIgnoresAccentsAndFiltersGenres(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	got, err := store.Search(ctx, "corazon mana", nil, 10)
	if err != nil || len(got) != 1 || got[0].Title != "Corazón Espinado" {
		t.Fatalf("search: %v %+v", err, got)
	}
	got, _ = store.Search(ctx, "adio", nil, 10) // prefix
	if len(got) != 1 {
		t.Fatalf("prefijo: %+v", got)
	}
	got, _ = store.Search(ctx, `carnaval"`, []int64{genreID(t, store, "KARAOKES")}, 10)
	if len(got) != 0 {
		t.Fatalf("filtro de género: %+v", got)
	}
	idx, err := store.ArtistIndexForLetter(ctx, genreID(t, store, "KARAOKES"), KindAll, "s")
	if err != nil || idx != 1 {
		t.Fatalf("ABC: %d %v", idx, err)
	}
}

func TestExportHTML(t *testing.T) {
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	var buf bytes.Buffer
	if err := ExportHTML(context.Background(), store, &buf, "Catálogo", []string{"karaokes"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Amor Eterno") || strings.Contains(out, "Carnaval") || !strings.Contains(out, "— Maná") {
		t.Fatalf("export: %s", out)
	}
}

func TestKindsSplitKaraokeVideoAudio(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	data := "KARAOKES\r\n\tA|D:\\master.1000\\karaokes\\A\\|\r\n\t\tD:\\master.1000\\karaokes\\A\\|K1|AVI\r\n" +
		"VIDEOS\r\n\tB|D:\\master.1000\\videos\\B\\|\r\n\t\tD:\\master.1000\\videos\\B\\|V1|AVI\r\n\t\tD:\\master.1000\\videos\\B\\|A1|MP3\r\n" +
		"MIXES\r\n\tC|D:\\master.1000\\MIXES\\C\\|\r\n\t\tD:\\master.1000\\MIXES\\C\\|Track 01|CDG\r\n"
	importInto(t, store, data, ImportOptions{})
	counts, err := store.KindCounts(ctx)
	if err != nil || counts[KindKaraoke] != 2 || counts[KindVideo] != 1 || counts[KindAudio] != 1 {
		t.Fatalf("conteos %v %v", counts, err)
	}
	gs, _ := store.Genres(ctx, KindKaraoke)
	if len(gs) != 2 { // KARAOKES + MIXES (CDG)
		t.Fatalf("géneros karaoke: %+v", gs)
	}
	vid := genreID(t, store, "VIDEOS")
	if n, _ := store.ArtistCount(ctx, vid, KindKaraoke); n != 0 {
		t.Fatalf("VIDEOS no tiene karaokes, dio %d", n)
	}
	as, _ := store.Artists(ctx, vid, KindAudio, 10, 0)
	if len(as) != 1 || as[0].Tracks != 1 {
		t.Fatalf("artistas de audio: %+v", as)
	}
	ts, _ := store.Tracks(ctx, as[0].ID, KindVideo)
	if len(ts) != 1 || ts[0].Title != "V1" || ts[0].Kind != KindVideo {
		t.Fatalf("pistas de video: %+v", ts)
	}
}

func TestLetters(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample, ImportOptions{})
	g := genreID(t, store, "KARAOKES")
	ls, err := store.Letters(ctx, []int64{g}, KindAll)
	if err != nil {
		t.Fatal(err)
	}
	// Juan Gabriel, # Seguro cantas una (sorted as "seguro"), Zoé.
	if len(ls) != 3 || ls[0].Letter != "j" || ls[1].Letter != "s" || ls[2].Letter != "z" {
		t.Fatalf("letras: %+v", ls)
	}
	as, err := store.ArtistsByLetter(ctx, []int64{g}, KindAll, "Z")
	if err != nil || len(as) != 1 || as[0].Name != "Zoé" {
		t.Fatalf("artistas con Z: %v %+v", err, as)
	}
}

// The guest list has no genres: the same artist in two genres is one entry
// with the songs of both, and nil genres means every genre.
func TestArtistsMergedAcrossGenres(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	importInto(t, store, sample+
		"\tJuan Gabriel|D:\\master.1000\\musica\\JUAN GABRIEL\\|\r\n"+
		"\t\tD:\\master.1000\\musica\\JUAN GABRIEL\\|Hasta Que Te Conoc\xed|MP3\r\n", ImportOptions{})
	ls, err := store.Letters(ctx, nil, KindAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		if l.Letter == "j" && l.Count != 1 {
			t.Fatalf("Juan Gabriel debe contar una vez: %+v", ls)
		}
	}
	as, err := store.ArtistsByLetter(ctx, nil, KindAll, "j")
	if err != nil || len(as) != 1 || len(as[0].IDs) != 2 {
		t.Fatalf("artistas con J: %v %+v", err, as)
	}
	if as[0].Cover == "" {
		t.Fatalf("debe usar la carpeta con portada: %+v", as[0])
	}
	ts, err := store.TracksOf(ctx, as[0].IDs, KindAll)
	if err != nil || len(ts) != 4 {
		t.Fatalf("canciones de los dos géneros: %v %d", err, len(ts))
	}
	only, _ := store.ArtistsByLetter(ctx, []int64{genreID(t, store, "MUSICA")}, KindAll, "j")
	if len(only) != 1 || len(only[0].IDs) != 1 {
		t.Fatalf("con filtro de género (modo infantil) solo esa carpeta: %+v", only)
	}
}
