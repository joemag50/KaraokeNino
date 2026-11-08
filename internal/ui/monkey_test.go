package ui

import (
	"fmt"
	"github.com/josegamboa/karaokenino/internal/catalog"
	"math/rand"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/library"
	"github.com/josegamboa/karaokenino/internal/player"
)

// A stubborn guest: thousands of random keys on every screen (keypad,
// arrows, Esc, letters). After every key the screen must still make sense.
func TestStubbornGuest(t *testing.T) {
	for seed := int64(1); seed <= 6; seed++ {
		t.Run(fmt.Sprint("semilla", seed), func(t *testing.T) { stubborn(t, seed, 4000) })
	}
}

func stubborn(t *testing.T, seed int64, n int) {
	a := newTestApp(t)
	a.Cfg.Admin.ShutdownCommand = []string{"true"}
	a.Cfg.Admin.RebootCommand = []string{"true"}
	exits := 0
	a.Exit = func(int) { exits++ }
	w := test.NewTempWindow(t, a.build())
	a.window = w
	w.Resize(fyne.NewSize(1280, 720))
	a.start()

	keys := []fyne.KeyName{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ".", "+", "-", "*", "/",
		fyne.KeyEnter, fyne.KeyReturn, fyne.KeyEscape, fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight,
		fyne.KeyBackspace, fyne.KeyPageUp, fyne.KeyPageDown}
	r := rand.New(rand.NewSource(seed))
	var last []string
	for i := 0; i < n; i++ {
		var desc string
		switch x := r.Intn(100); {
		case x < 4:
			c := rune('a' + r.Intn(26))
			desc = string(c)
			a.onRune(c)
		case x < 6:
			desc = "(espera)"
			time.Sleep(5 * time.Millisecond)
		default:
			k := keys[r.Intn(len(keys))]
			desc = string(k)
			a.onKey(k, false)
		}
		last = append(last, desc)
		if len(last) > 25 {
			last = last[1:]
		}
		if msg := checkScreen(a); msg != "" {
			t.Fatalf("tecla %d (%s): %s\núltimas: %s", i, desc, msg, strings.Join(last, " "))
		}
	}
	t.Logf("%d teclas, %d salidas pedidas, cola=%d", n, exits, len(func() []string { _, q := a.box.Snapshot(); s := make([]string, len(q)); return s }()))
}

// checkScreen returns what is wrong with the screen, or "".
func checkScreen(a *App) string {
	switch {
	case a.letterIdx < 0 || (len(a.letters) > 0 && a.letterIdx >= len(a.letters)):
		return fmt.Sprintf("letra %d de %d", a.letterIdx, len(a.letters))
	case a.artistIdx < 0 || (len(a.artists) > 0 && a.artistIdx >= len(a.artists)):
		return fmt.Sprintf("artista %d de %d", a.artistIdx, len(a.artists))
	case a.trackIdx < 0 || (len(a.tracks) > 0 && a.trackIdx >= len(a.tracks)):
		return fmt.Sprintf("canción %d de %d", a.trackIdx, len(a.tracks))
	case a.focus < colLetters || a.focus > colSongs:
		return fmt.Sprintf("foco %d", a.focus)
	}
	// The lists on screen match the data.
	if a.scr.letters.sel != a.letterIdx && len(a.letters) > 0 {
		return fmt.Sprintf("la lista de letras marca %d y la app %d", a.scr.letters.sel, a.letterIdx)
	}
	if a.scr.artists.sel != a.artistIdx && len(a.artists) > 0 {
		return fmt.Sprintf("la lista de artistas marca %d y la app %d", a.scr.artists.sel, a.artistIdx)
	}
	if a.scr.songs.sel != a.trackIdx && len(a.tracks) > 0 {
		return fmt.Sprintf("la lista de canciones marca %d y la app %d", a.scr.songs.sel, a.trackIdx)
	}
	// Only one dialog at a time, and it is really on screen.
	n := 0
	for _, o := range a.root.Objects {
		if a.ov != nil && o == a.ov.root {
			n++
		}
	}
	if a.ov != nil && n != 1 {
		return fmt.Sprintf("el diálogo está %d veces en pantalla", n)
	}
	// Messages must not lie.
	cur, q := a.box.Snapshot()
	qi := a.scr.queueInfo.Text
	if strings.HasPrefix(qi, "Cola vacía") && (cur != nil || len(q) > 0) {
		return fmt.Sprintf("dice %q pero hay %d en cola (sonando: %v)", qi, len(q), cur != nil)
	}
	st := a.scr.status.Text
	if a.Cfg.DemoMode && strings.Contains(st, "Buscando el disco") {
		return "en demo dice que busca el disco"
	}
	if a.splash.visible() && a.Cfg.DemoMode && strings.Contains(a.splash.status.Text, "Buscando el disco") {
		return "la bienvenida en demo dice que busca el disco"
	}
	return ""
}

func startedApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.Cfg.Admin.ShutdownCommand = []string{"true"}
	a.Cfg.Admin.RebootCommand = []string{"true"}
	w := test.NewTempWindow(t, a.build())
	a.window = w
	w.Resize(fyne.NewSize(1280, 720))
	startToBrowse(t, a)
	press(a, fyne.KeyEnter, fyne.KeyEnter) // letters → artists → songs
	return a
}

// Pressing Intro ten times on the same song must not queue it ten times.
func TestNecioEnterTenTimes(t *testing.T) {
	a := startedApp(t)
	for i := 0; i < 10; i++ {
		press(a, fyne.KeyEnter)
	}
	cur, q := a.box.Snapshot()
	if cur == nil || len(q) != 0 {
		t.Fatalf("la misma canción quedó %d veces en la cola", len(q)+1)
	}
	if !strings.Contains(a.scr.status.Text, "Ya está sonando") {
		t.Fatalf("debe avisar que ya suena: %q", a.scr.status.Text)
	}
	press(a, "2", fyne.KeyEnter, fyne.KeyEnter)
	if _, q := a.box.Snapshot(); len(q) != 1 || !strings.Contains(a.scr.status.Text, "Ya está en la cola") {
		t.Fatalf("cola %d, mensaje %q", len(q), a.scr.status.Text)
	}
}

// One "." must not skip someone else's song: it asks to press again.
func TestNecioSkipNeedsTwoPresses(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter, "2", fyne.KeyEnter, "2", fyne.KeyEnter) // 3 songs
	first, _ := a.box.Snapshot()
	press(a, ".")
	if cur, _ := a.box.Snapshot(); cur.TrackID != first.TrackID {
		t.Fatal("una sola . no debe saltar la canción")
	}
	if !strings.Contains(a.scr.status.Text, "otra vez") {
		t.Fatalf("debe pedir confirmación: %q", a.scr.status.Text)
	}
	press(a, ".")
	if cur, _ := a.box.Snapshot(); cur.TrackID == first.TrackID {
		t.Fatal("dos . seguidas deben saltar")
	}
	// Mashing "." does not empty the whole queue in one go: each skip needs its own pair.
	press(a, ".", ".", ".", ".")
	if cur, q := a.box.Snapshot(); cur == nil && len(q) == 0 {
		t.Fatal("presionar . muchas veces vació toda la cola")
	}
}

// A forgotten pause or volume at 0 must stay visible, not vanish after 5 s.
func TestNecioPauseAndMuteStayVisible(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter, "*")
	if !strings.Contains(a.scr.sticky, "pausa") {
		t.Fatalf("la pausa debe quedar indicada: %q", a.scr.sticky)
	}
	if a.scr.keypad.keys["*"].label.Text != "Continuar" {
		t.Fatalf("* dice %q", a.scr.keypad.keys["*"].label.Text)
	}
	press(a, "*")
	if strings.Contains(a.scr.sticky, "pausa") {
		t.Fatal("al continuar debe quitarse el aviso de pausa")
	}
	for i := 0; i < 30; i++ {
		press(a, "-")
	}
	if a.MPV.Volume() != 0 || !strings.Contains(a.scr.sticky, "Volumen en 0") {
		t.Fatalf("volumen %d, aviso %q", a.MPV.Volume(), a.scr.sticky)
	}
	press(a, "+")
	if strings.Contains(a.scr.sticky, "Volumen en 0") {
		t.Fatal("al subir el volumen debe quitarse el aviso")
	}
}

// Guessing the PIN: after 3 wrong tries, wait before trying again.
func TestNecioGuessesPIN(t *testing.T) {
	a := startedApp(t)
	for try := 0; try < 3; try++ {
		if try == 0 {
			press(a, fyne.KeyEscape)
		}
		press(a, "0", "0", "0", "0", fyne.KeyEnter)
	}
	if a.ov != nil {
		t.Fatal("tras 3 intentos debe cerrarse")
	}
	press(a, fyne.KeyEscape)
	press(a, "1", "2", "3", "4", fyne.KeyEnter) // even the right PIN waits
	if a.ov != nil && a.ov.name == "admin" {
		t.Fatal("no debe dejar entrar durante la espera")
	}
}

// Every instruction the guest reads must match the real keys.
func TestTextsMatchTheKeys(t *testing.T) {
	a := newTestApp(t)
	a.splashMin = time.Hour
	w := test.NewTempWindow(t, a.build())
	a.window = w
	a.start()
	if strings.Contains(a.splash.status.Text, "Buscando el disco") {
		t.Fatalf("en demo la bienvenida dice %q", a.splash.status.Text)
	}
	var all []string
	for s := 0; s < 3; s++ {
		a.idle.slide = s
		a.idle.render()
		all = append(all, a.idle.slideTitle.Text)
		for _, l := range a.idle.lines {
			all = append(all, l.Text)
		}
	}
	text := strings.Join(all, " | ")
	for _, old := range []string{"4 y 6", "4 / 6  cambiar de artista", "0  buscar por letra", "0 0"} {
		if strings.Contains(text, old) {
			t.Errorf("la pantalla de espera dice %q (navegación vieja): %s", old, text)
		}
	}
	if strings.Contains(a.scr.status.Text, "administrador") {
		t.Errorf("la barra le anuncia el admin a los invitados: %q", a.scr.status.Text)
	}
	a.Library = library.New(config.LibraryConfig{DirName: "no-existe", SearchIn: []string{t.TempDir()}})
	a.rescan()
	a.flash("x")
	a.Cfg.DemoMode = false
	if a.rescan(); a.scanning {
		t.Error("sin disco no debe decir que escanea")
	}
}

// Fullscreen video (karaoke or music video): every key except volume and
// pause brings the menu back, every time; 7 returns to fullscreen; "." leaves
// AND prepares the skip.
func TestVideoFullscreenKeys(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter) // a karaoke video
	if !a.MPV.VideoFull() {
		t.Fatal("el video debe empezar en pantalla completa")
	}
	cur, _ := a.box.Snapshot()
	for round := 0; round < 5; round++ {
		press(a, fyne.KeyEnter)
		if a.MPV.VideoFull() {
			t.Fatalf("vuelta %d: Intro no sacó del video", round)
		}
		if _, q := a.box.Snapshot(); len(q) != 0 {
			t.Fatalf("vuelta %d: Intro para salir agregó una canción", round)
		}
		if got := a.scr.keypad.keys["7"].label.Text; got != "Cantar" {
			t.Fatalf("la tecla 7 dice %q en vez de volver al video", got)
		}
		press(a, "7")
		if !a.MPV.VideoFull() {
			t.Fatalf("vuelta %d: 7 no regresó a pantalla completa", round)
		}
	}
	for _, k := range []fyne.KeyName{"8", "2", "4", "6", "5", "0", "/", "1", fyne.KeyUp} {
		press(a, k)
		if a.MPV.VideoFull() {
			t.Fatalf("la tecla %s no sacó del video", k)
		}
		if a.ov != nil {
			a.closeOverlay()
		}
		if a.layer != nil {
			a.closeLayer()
		}
		press(a, "7")
	}
	press(a, "+", "-", "*", "*")
	if !a.MPV.VideoFull() {
		t.Fatal("volumen y pausa no deben sacar del video")
	}
	// "." leaves the video and asks for the second "."
	press(a, ".")
	if a.MPV.VideoFull() || !strings.Contains(a.scr.status.Text, "otra vez") {
		t.Fatalf("la primera . debe salir y pedir otra: full=%v %q", a.MPV.VideoFull(), a.scr.status.Text)
	}
	press(a, ".")
	if now, _ := a.box.Snapshot(); now != nil && now.TrackID == cur.TrackID {
		t.Fatal("la segunda . debe saltar")
	}
}

// The idle screen must never hide a video that starts.
func TestIdleScreenDoesNotCoverVideo(t *testing.T) {
	a := startedApp(t)
	a.idle.show()
	a.onSongStart(player.Item{Title: "x", Artist: "y"}, nil)
	time.Sleep(1700 * time.Millisecond)
	if a.idle.visible {
		t.Fatal("la pantalla de espera quedó encima del video")
	}
}

// Covers asked for before the disk shows up must be tried again later.
func TestCoversBeforeDiskAreRetried(t *testing.T) {
	a := startedApp(t)
	a.diskOK = false
	if p := a.coverFor("karaokes/Juan Gabriel", ""); p != "" {
		t.Fatalf("sin disco no hay portada: %q", p)
	}
	a.thumbs.Get("x", nil)
	a.diskOK = true
	a.thumbs.ResetFailed()
	if p := a.coverFor("karaokes/Juan Gabriel", ""); p != "" {
		t.Fatalf("todavía no se hace la miniatura: %q", p)
	}
}

// When the last song ends and nothing is queued, the guest is back in the
// menu of the same mode, where they left it.
func TestBackToMenuWhenQueueEnds(t *testing.T) {
	a := startedApp(t)
	press(a, "6") // move in songs so we can check the position is kept
	letter, artist := a.currentLetter(), a.artistIdx
	press(a, fyne.KeyEnter) // a karaoke video
	// What the built-in player does for a video:
	a.vv.Show(true)
	a.vv.setFull(true)
	press(a, "/") // the guest opened the queue and left it there
	if a.scr.root.Visible() {
		t.Fatal("con el video en pantalla completa el menú está oculto")
	}
	done := make(chan struct{})
	a.box.OnFinished = func() { fyne.Do(a.backToMenu); close(done) }
	a.box.Next() // the song ends; nothing else in the queue
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no avisó que terminó la cola")
	}
	time.Sleep(50 * time.Millisecond)
	switch {
	case !a.scr.root.Visible() || a.vv.root.Visible():
		t.Fatal("no regresó al menú")
	case a.ov != nil:
		t.Fatalf("quedó abierta la ventana %q", a.ov.name)
	case a.mode != "karaoke" || a.currentLetter() != letter || a.artistIdx != artist:
		t.Fatalf("cambió de lugar: modo %q letra %q artista %d", a.mode, a.currentLetter(), a.artistIdx)
	case !strings.Contains(a.scr.status.Text, "Se terminó la cola"):
		t.Fatalf("mensaje: %q", a.scr.status.Text)
	}
}

// The admin menu is never closed by the queue ending.
func TestQueueEndKeepsAdminOpen(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter)
	press(a, fyne.KeyEscape) // leaves the (simulated) fullscreen video
	press(a, fyne.KeyEscape, "1", "2", "3", "4", fyne.KeyEnter)
	if a.ov == nil || a.ov.name != "admin" {
		t.Fatal("no se abrió el admin")
	}
	a.backToMenu()
	if a.ov == nil || a.ov.name != "admin" {
		t.Fatal("el fin de la cola cerró el menú de administrador")
	}
}

// While a guest browses, the preview box says when the video comes back.
func TestFullscreenCountdown(t *testing.T) {
	a := startedApp(t)
	a.videoBackDelay = 2 * time.Second
	press(a, fyne.KeyEnter) // video, fullscreen
	if a.scr.countdown.Text != "" {
		t.Fatalf("en pantalla completa no hay conteo: %q", a.scr.countdown.Text)
	}
	press(a, "8") // back to the menu
	if !strings.HasPrefix(a.scr.countdown.Text, "Pantalla completa en 2 s") {
		t.Fatalf("conteo %q", a.scr.countdown.Text)
	}
	time.Sleep(1100 * time.Millisecond)
	if !strings.HasPrefix(a.scr.countdown.Text, "Pantalla completa en 1 s") {
		t.Fatalf("el conteo no avanza: %q", a.scr.countdown.Text)
	}
	press(a, "2") // a key restarts it
	if !strings.HasPrefix(a.scr.countdown.Text, "Pantalla completa en 2 s") {
		t.Fatalf("una tecla debe reiniciar el conteo: %q", a.scr.countdown.Text)
	}
	press(a, "/") // a dialog: wait until it closes
	if a.scr.countdown.Text != "Pantalla completa al cerrar" {
		t.Fatalf("con diálogo: %q", a.scr.countdown.Text)
	}
	time.Sleep(2300 * time.Millisecond)
	if a.MPV.VideoFull() {
		t.Fatal("no debe volver a pantalla completa con un diálogo abierto")
	}
	press(a, fyne.KeyEscape) // close: the countdown starts again
	time.Sleep(2300 * time.Millisecond)
	if !a.MPV.VideoFull() {
		t.Fatal("después de cerrar el diálogo debe regresar a pantalla completa")
	}
	if a.scr.countdown.Text != "" {
		t.Fatalf("en pantalla completa se quita el conteo: %q", a.scr.countdown.Text)
	}
}

// "If you are singing, you can add a song and go back to singing": 7
// toggles Reproducción (fullscreen) and Lista, Intro adds in the list, and
// volume and pause work in both modes.
func TestSingAddAndBackToSinging(t *testing.T) {
	a := startedApp(t)
	if a.videoBackDelay != 10*time.Second {
		t.Fatalf("la vuelta sola a pantalla completa debe ser a los 10 s, no %v", a.videoBackDelay)
	}
	press(a, fyne.KeyEnter) // a karaoke video, fullscreen
	if !a.MPV.VideoFull() {
		t.Fatal("el video debe empezar en pantalla completa")
	}
	for round := 0; round < 3; round++ {
		press(a, "7") // Reproducción → Lista
		if a.MPV.VideoFull() {
			t.Fatalf("vuelta %d: 7 no abrió la lista", round)
		}
		if !strings.Contains(a.scr.status.Text, "7 para volver a cantar") {
			t.Fatalf("al salir debe decir cómo volver: %q", a.scr.status.Text)
		}
		if got := a.scr.keypad.keys["7"].label.Text; got != "Cantar" {
			t.Fatalf("en la lista la tecla 7 dice %q", got)
		}
		vol := a.MPV.Volume()
		press(a, "-")
		if a.MPV.Volume() >= vol || a.MPV.VideoFull() {
			t.Fatalf("en la lista el volumen no bajó (%d → %d)", vol, a.MPV.Volume())
		}
		_, before := a.box.Snapshot()
		press(a, "2", fyne.KeyEnter) // another song
		if _, q := a.box.Snapshot(); len(q) != len(before)+1 {
			t.Fatalf("vuelta %d: Intro en la lista no agregó (cola %d → %d)", round, len(before), len(q))
		}
		press(a, "7") // Lista → Reproducción
		if !a.MPV.VideoFull() {
			t.Fatalf("vuelta %d: 7 no regresó a cantar", round)
		}
		press(a, "+", "*")
		if !a.MPV.VideoFull() || !a.MPV.Paused() {
			t.Fatal("en pantalla completa volumen y pausa funcionan sin salir")
		}
		press(a, "*")
	}
}

// "Siguiente: …" shows only in the last 10 s of the song, not at its start.
func TestNextUpOnlyNearTheEnd(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter, "7", "2", fyne.KeyEnter) // one playing, one waiting
	if _, q := a.box.Snapshot(); len(q) != 1 {
		t.Fatalf("debe haber una canción esperando, hay %d", len(q))
	}
	cur, _ := a.box.Snapshot()
	told := func() bool { return a.nextShown == cur.TrackID }
	a.onProgress(3, 200)
	if told() {
		t.Fatal("al empezar no debe decir quién sigue")
	}
	a.onProgress(185, 200) // 15 s left
	if told() {
		t.Fatal("a 15 s del final todavía no")
	}
	a.onProgress(191, 200) // 9 s left
	if !told() {
		t.Fatal("a 10 s del final debe decir quién sigue")
	}
}

// From the songs column (the last one) 6 goes back to the player, like 7.
func TestSixFromSongsBackToPlayer(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter) // a karaoke video, fullscreen
	press(a, "7")           // to the list
	if a.MPV.VideoFull() || a.focus != colSongs {
		t.Fatalf("debe estar en la lista, columna de canciones (columna %d)", a.focus)
	}
	if got := a.scr.keypad.keys["6"].label.Text; got != "Cantar" {
		t.Fatalf("en Canciones con video la tecla 6 dice %q", got)
	}
	press(a, "6")
	if !a.MPV.VideoFull() {
		t.Fatal("6 en Canciones no regresó al reproductor")
	}
	press(a, "7", "4") // list, then to the artists column
	if a.focus != colArtists || a.MPV.VideoFull() {
		t.Fatal("4 debe ir a Artistas sin volver al video")
	}
	press(a, "6") // artists → songs, not the player
	if a.focus != colSongs || a.MPV.VideoFull() {
		t.Fatal("desde Artistas, 6 pasa a Canciones")
	}
}

// The panel under the songs lists what comes next, and "+N más" when it
// does not fit.
func TestQueuePanelShowsUpcoming(t *testing.T) {
	a := startedApp(t)
	if !strings.HasPrefix(a.scr.queueTxt[0].Text, "Vacía") {
		t.Fatalf("sin cola: %q", a.scr.queueTxt[0].Text)
	}
	press(a, fyne.KeyEnter) // plays now
	press(a, "7")           // to the list
	if !strings.HasPrefix(a.scr.queueTxt[0].Text, "Vacía") {
		t.Fatalf("lo que suena no va en «siguen»: %q", a.scr.queueTxt[0].Text)
	}
	press(a, "2", fyne.KeyEnter, "2", fyne.KeyEnter)
	_, q := a.box.Snapshot()
	if len(q) != 2 || !strings.HasPrefix(a.scr.queueTxt[0].Text, q[0].Title) ||
		!strings.HasPrefix(a.scr.queueTxt[1].Text, q[1].Title) || a.scr.queueTxt[2].Text != "" {
		t.Fatalf("panel %q / %q / %q, cola %+v", a.scr.queueTxt[0].Text, a.scr.queueTxt[1].Text, a.scr.queueTxt[2].Text, q)
	}
	if a.scr.queueNum[0].Text != "1" || !strings.Contains(a.scr.queueTitle.Text, "(2)") {
		t.Fatalf("número %q título %q", a.scr.queueNum[0].Text, a.scr.queueTitle.Text)
	}
	for i := 0; i < 4; i++ { // 6 waiting: 4 lines + "+2 más"
		press(a, "2", fyne.KeyEnter)
	}
	_, q = a.box.Snapshot()
	if len(q) != 6 {
		t.Fatalf("cola de %d", len(q))
	}
	a.onProgress(70, 200)
	snapshot(t, a.window, "09-lista-con-cola")
	if last := a.scr.queueTxt[queueRows-1].Text; !strings.HasPrefix(last, "+2 más") {
		t.Fatalf("última línea %q", last)
	}
	if !strings.HasPrefix(a.scr.queueTxt[3].Text, q[3].Title) {
		t.Fatalf("cuarta línea %q, esperaba %q", a.scr.queueTxt[3].Text, q[3].Title)
	}
	a.box.Next() // the next one starts: the list moves up
	time.Sleep(50 * time.Millisecond)
	_, q = a.box.Snapshot()
	if !strings.HasPrefix(a.scr.queueTxt[0].Text, q[0].Title) {
		t.Fatalf("no se actualizó: %q, sigue %q", a.scr.queueTxt[0].Text, q[0].Title)
	}
}

// The progress bars follow the song, but mpv's per-frame reports reach the
// screen only a couple of times per second.
func TestProgressBarThrottled(t *testing.T) {
	a := startedApp(t)
	press(a, fyne.KeyEnter)
	if a.scr.progress.hidden {
		t.Fatal("con una canción la barra se ve")
	}
	a.onProgress(30, 60)
	if a.scr.progress.w != designW/2 || a.vv.progress.w != designW/2 {
		t.Fatalf("a la mitad: lista %d, video %d", a.scr.progress.w, a.vv.progress.w)
	}
	var got []float64
	a.MPV.OnProgress = nil // the demo player must not interfere
	f := throttle(func(pos, dur float64) { got = append(got, pos) })
	for i := 0; i < 30; i++ { // one second of frames, all at once
		f(31+float64(i)/30, 60)
	}
	if len(got) != 1 {
		t.Fatalf("30 reportes seguidos deben llegar como 1, llegaron %d", len(got))
	}
	f(1, 60) // a new song (the position goes back): right away
	if len(got) != 2 || got[1] != 1 {
		t.Fatalf("canción nueva: %v", got)
	}
	a.box.Clear()
	_ = a.MPV.Stop()
	a.scr.refreshNowPlaying()
	if !a.scr.progress.hidden {
		t.Fatal("sin canción no hay barra")
	}
}

// Over the video: a hint says how to reach the list, then it goes away
// and the bottom shade with it (the lyrics stay clear).
func TestFullscreenHintFades(t *testing.T) {
	a := startedApp(t)
	hintFor = 150 * time.Millisecond
	t.Cleanup(func() { hintFor = 6 * time.Second })
	a.vv.Show(true)
	a.vv.setFull(true)
	if !strings.HasPrefix(a.vv.hint.Text, "7  ver la lista") || a.vv.botBg.Hidden {
		t.Fatalf("pista %q, sombra oculta %v", a.vv.hint.Text, a.vv.botBg.Hidden)
	}
	if a.vv.progress.hidden {
		t.Fatal("en pantalla completa la línea de avance se ve")
	}
	time.Sleep(300 * time.Millisecond)
	if a.vv.hint.Text != "" || !a.vv.botBg.Hidden {
		t.Fatalf("la pista debe irse: %q, sombra oculta %v", a.vv.hint.Text, a.vv.botBg.Hidden)
	}
	a.vv.setFull(false)
	if !a.vv.progress.hidden {
		t.Fatal("en el recuadro la línea del video no va (la de la lista sí)")
	}
}

// Music without video starts on its player screen (the cover), and works
// like the fullscreen video: any key → list, 6/7 → back, by itself after
// videoBackDelay; volume and pause don't leave it.
func TestMusicPlayerScreen(t *testing.T) {
	a := startedApp(t)
	press(a, "1", "6", fyne.KeyEnter) // mode screen → MÚSICA
	if a.mode != catalog.KindAudio {
		t.Fatalf("modo %q", a.mode)
	}
	press(a, fyne.KeyEnter, fyne.KeyEnter, fyne.KeyEnter) // letters → artists → songs → play
	if cur, _ := a.box.Snapshot(); cur == nil || !cur.Audio {
		t.Fatal("debe sonar una canción de audio")
	}
	if !a.audioFull || !a.au.root.Visible() || a.au.title.Text == "" {
		t.Fatal("la música debe empezar en su pantalla con la portada")
	}
	snapshot(t, a.window, "musica-portada")
	vol := a.MPV.Volume()
	press(a, "+", "*")
	if !a.audioFull || a.MPV.Volume() <= vol {
		t.Fatal("volumen y pausa funcionan sin salir de la pantalla de la canción")
	}
	press(a, "*")
	press(a, "8") // any key: to the list (consumed)
	if a.audioFull || a.au.root.Visible() {
		t.Fatal("una tecla debe llevar a la lista")
	}
	if got := a.scr.keypad.keys["7"].label.Text; got != "Canción" {
		t.Fatalf("en la lista la tecla 7 dice %q", got)
	}
	if got := a.scr.keypad.keys["6"].label.Text; got != "Canción" {
		t.Fatalf("en Canciones la tecla 6 dice %q", got)
	}
	press(a, "6")
	if !a.audioFull || !a.au.root.Visible() {
		t.Fatal("6 en Canciones debe regresar a la canción")
	}
	a.videoBackDelay = 50 * time.Millisecond
	press(a, "2")
	if a.audioFull {
		t.Fatal("una tecla debe llevar a la lista")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !a.audioFull && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !a.audioFull {
		t.Fatal("sin teclas debe volver sola a la canción")
	}
}
