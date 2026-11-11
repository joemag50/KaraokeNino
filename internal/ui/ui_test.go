package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/library"
	"github.com/josegamboa/karaokenino/internal/player"
	"github.com/josegamboa/karaokenino/internal/state"
)

// testCatalog imports a small Musica.data with several artists.
func testCatalog(t testing.TB, cfg config.Config) *catalog.Store {
	t.Helper()
	store, err := catalog.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	var b strings.Builder
	b.WriteString("KARAOKES\r\n")
	artists := []string{"Juan Gabriel", "Shakira", "Maná", "Luis Miguel", "Jenni Rivera", "Los Ángeles Azules", "Zoé", "Banda MS", "Café Tacvba", "Thalía", "Vicente Fernández", "Alejandro Fernández", "Rocío Dúrcal"}
	for _, ar := range artists {
		b.WriteString("\t" + ar + "|D:\\master.1000\\karaokes\\" + ar + "\\|\r\n")
		for _, s := range []string{"Amor Eterno", "Querida", "Hasta Que Te Conocí", "Así Fue", "No Tengo Dinero", "Se Me Olvidó Otra Vez", "Abrázame Muy Fuerte"} {
			b.WriteString("\t\tD:\\master.1000\\karaokes\\" + ar + "\\|" + s + "|AVI\r\n")
		}
	}
	b.WriteString("z INFANTILES\r\n\tCri-Cri|D:\\master.1000\\infantiles\\Cri-Cri\\|\r\n\t\tD:\\master.1000\\infantiles\\Cri-Cri\\|La Patita|MP3\r\n")
	b.WriteString("MUSICA\r\n\tCelia Cruz|D:\\master.1000\\musica\\Celia Cruz\\|\r\n\t\tD:\\master.1000\\musica\\Celia Cruz\\|La Vida Es Un Carnaval|MP3\r\n")
	data := []byte(b.String())
	// Musica.data is Windows-1252: re-encode the accents.
	musica := filepath.Join(t.TempDir(), "Musica.data")
	enc, _ := latin1(data)
	if err := os.WriteFile(musica, enc, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ImportMusicaData(context.Background(), cfg, musica, store, catalog.ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	return store
}

func latin1(utf8 []byte) ([]byte, error) {
	var out []byte
	for _, r := range string(utf8) {
		out = append(out, byte(r))
	}
	return out, nil
}

func newTestApp(t testing.TB) *App {
	t.Helper()
	test.NewTempApp(t)
	cfg := config.Default()
	cfg.CatalogDB = filepath.Join(t.TempDir(), "c.db") // thumbs go next to it
	cfg.DemoMode = true
	cfg.Idle.AfterSeconds = 0
	cfg.Playback.BackgroundMusic.Enabled = false
	cfg.Business.Name = "Rockolas Niño"
	cfg.Event.Welcome = "¡Felices 15, Ana!"
	cfg.LogFile = ""
	store := testCatalog(t, cfg)
	mpv := player.New(player.MPVOptions{Demo: true, DemoFor: time.Hour, MaxVolume: 100, Volume: 80, Fullscreen: true})
	a, err := newApp(Deps{
		Cfg: cfg, Store: store, MPV: mpv,
		Library: library.New(cfg.Library),
		State:   state.NewStore(filepath.Join(t.TempDir(), "state.json")),
		Exit:    func(int) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.splashMin = 0
	t.Cleanup(func() { a.box.Clear(); a.cancel() })
	return a
}

// startToBrowse opens the app and picks Karaoke on the mode screen.
func startToBrowse(t *testing.T, a *App) {
	t.Helper()
	a.start()
	if a.layer == nil {
		t.Fatal("esperaba la pantalla de modos")
	}
	press(a, fyne.KeyEnter)
	if a.layer != nil {
		t.Fatal("Intro debe entrar al modo")
	}
}

func press(a *App, keys ...fyne.KeyName) {
	for _, k := range keys {
		a.onKey(k, false)
	}
}

// snapshot renders the window to PNG when KN_SNAPSHOTS is set (visual check).
func snapshot(t *testing.T, w fyne.Window, name string) {
	dir := os.Getenv("KN_SNAPSHOTS")
	if dir == "" {
		return
	}
	img := w.Canvas().Capture()
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseQueueAndOverlays(t *testing.T) {
	a := newTestApp(t)
	w := test.NewTempWindow(t, a.build())
	a.window = w
	w.Resize(fyne.NewSize(1280, 720))
	a.start()
	snapshot(t, w, "00-modos")
	press(a, fyne.KeyEnter)

	// Letters column: A (Alejandro Fernández), B, C, J, L, M, R, S, T, V, Z.
	var letters []string
	for _, l := range a.letters {
		letters = append(letters, l.Letter)
	}
	if strings.Join(letters, "") != "abcjlmrstvz" || a.focus != colLetters {
		t.Fatalf("letras %v, foco %d", letters, a.focus)
	}
	if lbl := a.scr.keypad.keys["8"].label.Text; lbl != "Letra ↑" {
		t.Fatalf("tecla 8 dice %q", lbl)
	}
	snapshot(t, w, "01-inicio-1280x720")

	// 2 ×3 → letter J; 6 → artists (Jenni Rivera, Juan Gabriel); 2 → Juan Gabriel.
	press(a, "2", "2", "2")
	if a.currentLetter() != "j" || len(a.artists) != 2 {
		t.Fatalf("letra %q, %d artistas", a.currentLetter(), len(a.artists))
	}
	press(a, "6", "2")
	if a.focus != colArtists || a.artists[a.artistIdx].Name != "Juan Gabriel" {
		t.Fatalf("foco %d artista %+v", a.focus, a.artists[a.artistIdx])
	}
	if lbl := a.scr.keypad.keys["6"].label.Text; lbl != "Canciones" {
		t.Fatalf("tecla 6 dice %q", lbl)
	}
	// Intro → songs; 2 → second song; Intro → queue.
	press(a, fyne.KeyEnter, "2")
	if a.focus != colSongs {
		t.Fatal("Intro en artistas debe ir a canciones")
	}
	want := a.tracks[1]
	press(a, fyne.KeyEnter)
	cur, _ := a.box.Snapshot()
	if cur == nil || cur.TrackID != want.ID || cur.Artist != "Juan Gabriel" {
		t.Fatalf("no se agregó la canción: %+v", cur)
	}
	if lbl := a.scr.keypad.keys["enter"].label.Text; lbl != "Agregar" {
		t.Fatalf("Intro dice %q", lbl)
	}
	// The video is fullscreen: the first key only brings the menu back.
	press(a, "8")
	if a.MPV.VideoFull() {
		t.Fatal("una tecla debe sacar del video")
	}
	press(a, "8", fyne.KeyEnter)
	if _, q := a.box.Snapshot(); len(q) != 1 {
		t.Fatalf("cola: %+v", q)
	}
	snapshot(t, w, "02-sonando")

	// 4 4 → back to letters; 3 = page down jumps to the end without wrapping.
	press(a, "4", "4", "3", "3")
	if a.focus != colLetters || a.currentLetter() != "z" {
		t.Fatalf("página abajo: foco %d letra %q", a.focus, a.currentLetter())
	}

	// 0 → song number.
	target := a.tracks[0]
	press(a, "0")
	for _, d := range []byte(fmtInt(target.ID)) {
		press(a, fyne.KeyName(string(d)))
	}
	snapshot(t, w, "04-numero")
	press(a, fyne.KeyEnter)
	if _, q := a.box.Snapshot(); len(q) != 2 || q[1].TrackID != target.ID {
		t.Fatalf("por número: %+v", q)
	}

	// / → queue.
	press(a, "/")
	if a.ov == nil {
		t.Fatal("no se abrió la cola")
	}
	snapshot(t, w, "05-cola")
	press(a, fyne.KeyEscape)

	// Typing opens the search; → shows the song in the browser.
	for _, r := range "celia" {
		a.onRune(r)
	}
	time.Sleep(250 * time.Millisecond) // debounce
	snapshot(t, w, "06-busqueda")
	press(a, fyne.KeyEscape)

	// Admin: Esc then PIN; kids mode is item 6.
	press(a, fyne.KeyEscape)
	press(a, "1", "2", "3", "4", fyne.KeyEnter)
	snapshot(t, w, "07-admin")
	for i := 0; i < 5; i++ {
		press(a, "2")
	}
	press(a, fyne.KeyEnter)
	if !a.kidsMode() || len(a.genres) != 1 || a.genres[0].Name != "z INFANTILES" {
		t.Fatalf("modo infantil: %v %+v", a.kidsMode(), a.genres)
	}
	press(a, fyne.KeyEscape)

	a.idle.show()
	snapshot(t, w, "08-espera")
	press(a, "5")
	if a.idle.visible {
		t.Fatal("una tecla debe quitar la pantalla de espera")
	}
}

func TestSearchJumpsToSong(t *testing.T) {
	a := newTestApp(t)
	w := test.NewTempWindow(t, a.build())
	a.window = w
	startToBrowse(t, a)
	ts, _ := a.Store.Search(a.ctx, "querida shakira", nil, 1)
	if len(ts) != 1 {
		t.Fatal("falta la canción de prueba")
	}
	a.showTrack(ts[0])
	if a.focus != colSongs || a.currentLetter() != "s" || a.tracks[a.trackIdx].ID != ts[0].ID {
		t.Fatalf("letra %q foco %d", a.currentLetter(), a.focus)
	}
}

func TestWrongPINClosesAfterThreeTries(t *testing.T) {
	a := newTestApp(t)
	w := test.NewTempWindow(t, a.build())
	a.window = w
	startToBrowse(t, a)
	// Esc inside a window closes it; Esc on the main screen asks for the PIN.
	press(a, "/")
	press(a, fyne.KeyEscape)
	if a.ov != nil {
		t.Fatal("Esc debe cerrar la cola")
	}
	press(a, fyne.KeyEscape)
	if a.ov == nil {
		t.Fatal("Esc debe abrir el PIN")
	}
	for i := 0; i < 3; i++ {
		press(a, "9", "9", fyne.KeyEnter)
	}
	if a.ov != nil {
		t.Fatal("el PIN incorrecto debe cerrar el diálogo")
	}
}

func TestScalesToAnyScreen(t *testing.T) {
	for _, sz := range []fyne.Size{{Width: 1920, Height: 1080}, {Width: 1024, Height: 768}, {Width: 1366, Height: 768}} {
		a := newTestApp(t)
		w := test.NewTempWindow(t, a.build())
		a.window = w
		w.Resize(sz)
		startToBrowse(t, a)
		lay := a.scr.lay
		if lay.dw*lay.scale > sz.Width+0.5 || lay.dh*lay.scale > sz.Height+0.5 || lay.offX < -0.5 || lay.offY < -0.5 {
			t.Fatalf("%vx%v: escala %v desborda", sz.Width, sz.Height, lay.scale)
		}
		snapshot(t, w, "pantalla-"+fmtInt(int64(sz.Width))+"x"+fmtInt(int64(sz.Height)))
	}
}

func fmtInt(n int64) string {
	return strconv.FormatInt(n, 10)
}

// After a restart (or a power cut) the rockola opens silent with an empty
// queue: the last song of the previous session must not come back. The
// volume is kept.
func TestRestartStartsWithEmptyQueue(t *testing.T) {
	test.NewTempApp(t)
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	// state.json as written by older versions, with the song that was playing.
	old := `{"volume": 60, "current": {"track_id": 1, "title": "Hombre libre"}, "queue": [{"track_id": 2}]}`
	if err := os.WriteFile(statePath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.CatalogDB = filepath.Join(dir, "c.db")
	cfg.DemoMode = true
	cfg.Idle.AfterSeconds = 0
	cfg.Playback.BackgroundMusic.Enabled = false
	cfg.LogFile = ""
	store := testCatalog(t, cfg)
	mpv := player.New(player.MPVOptions{Demo: true, DemoFor: time.Hour, MaxVolume: 100, Volume: 80, Fullscreen: true})
	a, err := newApp(Deps{Cfg: cfg, Store: store, MPV: mpv, Library: library.New(cfg.Library),
		State: state.NewStore(statePath), Exit: func(int) {}})
	if err != nil {
		t.Fatal(err)
	}
	a.splashMin = 0
	t.Cleanup(func() { a.box.Clear(); a.cancel() })
	w := test.NewTempWindow(t, a.build())
	a.window = w
	a.start()

	if cur, q := a.box.Snapshot(); cur != nil || len(q) != 0 {
		t.Fatalf("al reiniciar no debe volver nada de la sesión anterior: %+v %+v", cur, q)
	}
	if !strings.HasPrefix(a.scr.queueInfo.Text, "Cola vacía") {
		t.Fatalf("la cola está vacía y dice %q", a.scr.queueInfo.Text)
	}
	if a.MPV.Volume() != 60 {
		t.Fatalf("volumen %d", a.MPV.Volume())
	}
	a.skipKey() // "." has nothing to resume
	if cur, _ := a.box.Snapshot(); cur != nil {
		t.Fatalf("«.» no debe reanudar nada: %+v", cur)
	}
	a.saveState()
	if b, _ := os.ReadFile(statePath); strings.Contains(string(b), "Hombre libre") {
		t.Fatalf("la canción vieja sigue en state.json: %s", b)
	}
}

func TestWelcomeAndModes(t *testing.T) {
	a := newTestApp(t)
	a.splashMin = time.Hour // keep the welcome screen
	w := test.NewTempWindow(t, a.build())
	a.window = w
	w.Resize(fyne.NewSize(1280, 720))
	a.start()
	if a.layer == nil || a.layer != a.splash.overlay {
		t.Fatal("esperaba la pantalla de bienvenida")
	}
	press(a, fyne.KeyEnter) // ignored while loading
	if a.layer != a.splash.overlay {
		t.Fatal("la bienvenida no debe cerrarse con teclas")
	}
	snapshot(t, w, "00-bienvenida")

	a.splashMin = 0
	a.checkReady()
	if a.layer == nil || a.layer == a.splash.overlay {
		t.Fatal("esperaba la pantalla de modos")
	}
	press(a, "6", fyne.KeyEnter) // second card: MÚSICA (no videos in the sample)
	if a.mode != catalog.KindAudio || a.layer != nil {
		t.Fatalf("modo %q", a.mode)
	}
	for _, g := range a.genres {
		if g.Name == "KARAOKES" {
			t.Fatalf("KARAOKES no tiene música: %+v", a.genres)
		}
	}
	if len(a.tracks) == 0 || a.tracks[0].Kind != catalog.KindAudio {
		t.Fatalf("pistas: %+v", a.tracks)
	}
	press(a, "1") // back to the mode screen
	if a.layer == nil {
		t.Fatal("1 debe volver a elegir modo")
	}
	press(a, fyne.KeyEscape) // admin opens over the mode screen
	if a.ov == nil {
		t.Fatal("Esc debe abrir el PIN sobre la pantalla de modos")
	}
}

func TestMusicKeepsTheMenuVisible(t *testing.T) {
	a := newTestApp(t)
	w := test.NewTempWindow(t, a.build())
	a.window = w
	startToBrowse(t, a)
	// What the player does when a song starts: ask for fullscreen, but the
	// video view is not shown (music has no video).
	a.vv.setFull(true)
	if !a.scr.root.Visible() {
		t.Fatal("con música el menú debe seguir visible")
	}
	a.vv.Show(true)
	a.vv.setFull(true)
	if a.scr.root.Visible() {
		t.Fatal("con video en pantalla completa el menú se oculta")
	}
}
