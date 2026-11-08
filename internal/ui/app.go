// Package ui is the KaraokeNino screen: browse genres → artists → songs with
// the keypad, queue songs, and the admin menu. Everything is drawn with
// canvas primitives on a scaled design so it fits any TV.
package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/keyboard"
	"github.com/josegamboa/karaokenino/internal/library"
	"github.com/josegamboa/karaokenino/internal/player"
	"github.com/josegamboa/karaokenino/internal/state"
	"github.com/josegamboa/karaokenino/internal/thumbs"
)

// Exit codes understood by deploy/kiosk-start.sh.
const (
	ExitRestart = 3  // start again (after an update or "Reiniciar programa")
	ExitDesktop = 10 // admin chose "Salir al escritorio": stop the kiosk loop
)

// Deps are the services the UI drives.
type Deps struct {
	Cfg     config.Config
	Store   *catalog.Store
	MPV     *player.MPV
	Library *library.Library
	State   *state.Store
	// Exit ends the program with a code (os.Exit by default).
	Exit func(code int)
}

// App holds the screen state. All fields are touched on the Fyne thread
// only; background work hands results over with fyne.Do.
type App struct {
	Deps
	pal      Palette
	keys     map[fyne.KeyName]string
	box      *player.Jukebox
	covers   *catalog.Covers
	ctx      context.Context
	cancel   context.CancelFunc
	window   fyne.Window
	root     *fyne.Container // main screen + overlays
	st       state.State
	stMu     sync.Mutex
	adminSeq []fyne.KeyName

	scr  *browseScreen
	ov   *overlay
	idle *idleScreen
	vv   *videoView // built-in player picture
	au   *audioView // player screen for music without video
	// audioFull: music without video is on its player screen (Reproducción),
	// not the list.
	audioFull bool
	layer     *overlay // full-screen screen (welcome, mode choice) under dialogs

	splash    *splashScreen
	splashMin time.Duration // welcome screen stays at least this long
	mode      catalog.Kind  // karaoke, videos or music

	// browse state: genre → letter → artist → song
	genres    []catalog.Genre
	genreIdx  int
	letters   []catalog.LetterCount
	letterIdx int
	artists   []catalog.Artist
	artistIdx int
	tracks    []catalog.Track
	trackIdx  int
	focus     int // colLetters, colArtists or colSongs

	thumbs      *thumbs.Cache
	bigThumbs   *thumbs.Cache     // large covers for the music player screen
	actionCells map[string]string // action → keypad cell ("up" → "8")

	diskOK     bool
	scanning   bool
	lastKey    time.Time
	idleTimer  *time.Timer
	videoTimer *time.Timer
	statusTill time.Time
	nextShown  int64 // track id whose "Siguiente" notice was shown near the end
	recent     []keyStamp
	skipArmed  time.Time // first "." pressed (skip needs two)
	// video waiting in the preview box goes back to fullscreen at videoBackAt
	videoBackAt    time.Time
	videoBackDelay time.Duration
	countdownOn    bool
	lastSkip       time.Time
	pinLock        time.Time // too many wrong PINs: wait until then
	pinFails       int
	diskNote       string // lasting status messages (see refreshNotes)
	scanNote       string

	// fxOn: animations allowed (config ui.animations; set by Run, so the
	// tests draw still screens). fxLoop animates the main screen.
	fxOn   bool
	fxLoop *fxLoop
}

type keyStamp struct {
	k fyne.KeyName
	t time.Time
}

// Run builds the window and blocks until the program exits.
func Run(d Deps) error {
	a, err := newApp(d)
	if err != nil {
		return err
	}
	fyneApp := app.NewWithID("com.josegamboa.karaokenino")
	fyneApp.Settings().SetTheme(newPartyTheme()) // embedded party fonts
	a.fxOn = d.Cfg.UI.Animations
	w := fyneApp.NewWindow("KaraokeNino")
	a.window = w
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 720))
	if d.Cfg.Fullscreen {
		w.SetFullScreen(true)
	}
	// Guests must not close the rockola: only the admin menu exits.
	w.SetCloseIntercept(func() { a.flash("Para salir usa el menú de administrador") })
	w.SetContent(a.build())
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) { a.onKey(ev.Name, false) })
	w.Canvas().SetOnTypedRune(a.onRune)
	if a.Cfg.Playback.EmbeddedVideo && !a.Cfg.DemoMode {
		if err := a.MPV.UseScreen(a.vv); err != nil {
			log.Printf("reproductor integrado no disponible (%v): se usa mpv aparte", err)
		} else {
			log.Printf("reproductor integrado (libmpv): el video se dibuja dentro de KaraokeNino")
		}
	}
	a.start()
	a.startFX()
	w.ShowAndRun()
	a.fxLoop.Stop()
	a.shutdown()
	return nil
}

func newApp(d Deps) (*App, error) {
	if d.Exit == nil {
		d.Exit = os.Exit
	}
	st, err := d.State.Load()
	if err != nil {
		log.Printf("estado: %v (se empieza de cero)", err)
	}
	theme := d.Cfg.Theme
	if st.Theme != "" {
		theme = st.Theme
	}
	if st.MaxVolume != nil {
		d.MPV.SetMaxVolume(*st.MaxVolume)
	}
	if st.Volume > 0 {
		d.MPV.SetVolume(st.Volume) // the last volume, right away (no disk needed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		Deps:   d,
		pal:    PaletteFor(theme),
		keys:   keyboard.Actions(d.Cfg.Keyboard.Bindings),
		ctx:    ctx,
		cancel: cancel,
		st:     st,

		splashMin:      4 * time.Second, // long enough to read the name and the welcome
		videoBackDelay: 10 * time.Second,
		mode:           catalog.KindKaraoke,
	}
	for _, k := range d.Cfg.Admin.OpenSequence {
		if fk := keyboard.FyneKey(k); fk != "" {
			a.adminSeq = append(a.adminSeq, fk)
		}
	}
	a.covers = catalog.NewCovers(d.Library.Abs, "")
	a.thumbs = thumbs.New(filepath.Join(filepath.Dir(d.Cfg.CatalogDB), "thumbs"), 240)
	a.thumbs.OnReady = func() { fyne.Do(a.refreshCovers) }
	a.bigThumbs = thumbs.New(filepath.Join(filepath.Dir(d.Cfg.CatalogDB), "thumbs", "grandes"), bigCoverPx)
	a.bigThumbs.OnReady = func() { fyne.Do(a.au.refreshCover) }
	a.actionCells = map[string]string{}
	for act, ev := range d.Cfg.Keyboard.Bindings {
		if c := padCell(ev); c != "" {
			a.actionCells[act] = c
		}
	}
	a.box = player.NewJukebox(d.MPV, a.jukeboxOptions())
	a.box.OnChange = func() { fyne.Do(a.onQueueChange) }
	a.box.OnError = func(it player.Item, err error) { fyne.Do(func() { a.onPlayError(it, err) }) }
	a.box.OnStart = func(cur player.Item, next *player.Item) { a.onSongStart(cur, next) }
	a.box.OnFinished = func() { fyne.Do(a.backToMenu) }
	d.MPV.OnKey = func(name string) { fyne.Do(func() { a.onKey(fyne.KeyName(name), true) }) }
	d.MPV.OnProgress = a.progressThrottled()
	d.MPV.OnOSD = func(text string, dur time.Duration) { fyne.Do(func() { a.vv.message(text, dur) }) }
	return a, nil
}

// build creates the main screen and the overlay stack.
func (a *App) build() fyne.CanvasObject {
	a.scr = newBrowseScreen(a)
	a.idle = newIdleScreen(a)
	a.vv = newVideoView(a)
	a.au = newAudioView(a)
	a.root = container.NewStack(a.scr.root, a.au.root, a.vv.root)
	a.splash = a.newSplash()
	// The pointer is hidden over the whole window, above every screen.
	return container.NewStack(a.root, newNoCursor())
}

// start loads the catalog and looks for the disk in the background.
func (a *App) start() {
	t0 := time.Now()
	a.splash.show()
	if err := a.loadGenres(); err != nil {
		a.flash(err.Error())
	}
	log.Printf("pantalla lista: %d géneros, %d artistas en %s (%s)", len(a.genres), len(a.artists),
		a.currentGenre().Name, time.Since(t0).Round(time.Millisecond))
	if counts, err := a.Store.KindCounts(a.ctx); err == nil {
		total := counts[catalog.KindKaraoke] + counts[catalog.KindVideo] + counts[catalog.KindAudio]
		msg := "Buscando el disco de música…"
		if a.Cfg.DemoMode {
			msg = "Modo demo (sin disco de música)"
		}
		a.splash.setStatus(msg, fmt.Sprintf("Catálogo: %s canciones", thousands(total)))
	}
	a.resetIdle()
	if a.Cfg.DemoMode {
		a.diskOK = true
		a.splash.diskDone = true
		a.checkReady()
		return
	}
	a.diskNote = "Buscando el disco de música…"
	a.refreshNotes()
	go a.findDisk()
}

func (a *App) findDisk() {
	wait := time.Duration(a.Cfg.Library.WaitSeconds) * time.Second
	t0 := time.Now()
	root, err := a.Library.Wait(a.ctx, wait, func(attempt int) {
		if attempt%3 == 0 {
			fyne.Do(func() {
				a.splash.setStatus("Buscando el disco de música…", fmt.Sprintf("Revisando discos (%d s)", attempt))
			})
		}
	})
	if err != nil {
		fyne.Do(func() {
			a.diskOK = false
			a.diskNote = "⚠ No se encontró el disco de música. Conéctalo; se buscará solo."
			a.refreshNotes()
			a.splash.diskDone = true
			a.checkReady()
		})
		// Keep looking: the disk may be plugged in later.
		for a.ctx.Err() == nil {
			time.Sleep(10 * time.Second)
			if root, err = a.Library.Detect(); err == nil {
				break
			}
		}
		if a.ctx.Err() != nil {
			return
		}
	}
	log.Printf("disco de música: %s (encontrado en %s)", root, time.Since(t0).Round(time.Millisecond))
	fyne.Do(func() {
		a.diskOK = true
		a.covers.Reset()
		a.thumbs.ResetFailed()
		a.bigThumbs.ResetFailed()
		a.refreshCovers()
		a.diskNote = ""
		a.refreshNotes()
		a.flash("Disco de música listo")
		a.refreshArtists()
		a.splash.setStatus("Disco listo", root)
		a.splash.diskDone = true
		a.checkReady()
	})
	// The disk is scanned only from the admin menu, unless the catalog is
	// empty (nothing to show yet) or scan_on_start is set.
	if a.Cfg.Library.ScanOnStart || len(a.genres) == 0 {
		// Let the first minute be snappy: scan a bit later, in the background.
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(a.scanDelay()):
		}
		fyne.Do(func() { a.rescan() })
	}
}

// rescan syncs the catalog with the disk in the background. It reports
// whether a scan is running (false: no disk).
func (a *App) rescan() bool {
	root := a.Library.Root()
	if a.scanning {
		return true
	}
	if root == "" {
		return false
	}
	a.scanning = true
	go func() {
		fyne.Do(func() { a.splash.setStatus("Preparando el catálogo…", "Revisando las canciones del disco") })
		res, err := catalog.Scan(a.ctx, a.Store, root, func(p catalog.ScanProgress) {
			fyne.Do(func() {
				a.scanNote = fmt.Sprintf("Revisando el disco… %s archivos", thousands(p.Files))
				a.refreshNotes()
				a.splash.setStatus("Preparando el catálogo…", fmt.Sprintf("%s archivos revisados", thousands(p.Files)))
			})
		})
		fyne.Do(func() {
			defer a.checkReady()
			a.splash.scanned = true
			a.scanning = false
			a.scanNote = ""
			a.refreshNotes()
			if err != nil {
				log.Printf("escaneo: %v", err)
				a.flash("Error al escanear el disco: " + err.Error())
				return
			}
			log.Printf("escaneo: %d archivos, %d nuevas, %d faltantes, %d recuperadas en %s",
				res.Files, res.Added, res.Missing, res.Restored, res.Took.Round(time.Second))
			if res.Added+res.Missing+res.Restored+res.Renamed > 0 {
				a.covers.Reset()
				a.thumbs.ResetFailed()
				a.bigThumbs.ResetFailed()
				a.bigThumbs.ResetFailed()
				_ = a.loadGenres()
				a.flash(fmt.Sprintf("Disco revisado: %d canciones nuevas, %d faltantes", res.Added, res.Missing))
			}
		})
	}()
	return true
}

// scanDelay waits before the startup scan (shorter on an empty catalog).
func (a *App) scanDelay() time.Duration {
	if len(a.genres) == 0 {
		return time.Second
	}
	return 15 * time.Second
}

func (a *App) shutdown() {
	a.cancel()
	a.saveState()
	_ = a.MPV.Stop()
	a.MPV.Close()
}

// ---------- genres (kids mode / hidden) ----------

func (a *App) visibleGenreFilter(gs []catalog.Genre) []catalog.Genre {
	kids := a.kidsMode()
	in := func(list []string, name string) bool {
		for _, g := range list {
			if strings.EqualFold(strings.TrimSpace(g), name) {
				return true
			}
		}
		return false
	}
	var out []catalog.Genre
	for _, g := range gs {
		switch {
		case kids && !in(a.Cfg.Admin.KidsGenres, g.Name):
		case !kids && in(a.Cfg.Admin.HiddenGenres, g.Name):
		default:
			out = append(out, g)
		}
	}
	return out
}

func (a *App) kidsMode() bool {
	a.stMu.Lock()
	defer a.stMu.Unlock()
	return a.st.KidsMode
}

func (a *App) genreIDs() []int64 {
	ids := make([]int64, len(a.genres))
	for i, g := range a.genres {
		ids[i] = g.ID
	}
	return ids
}

// ---------- jukebox ----------

func (a *App) jukeboxOptions() player.Options {
	o := player.Options{
		MaxQueue:          a.Cfg.Playback.MaxQueue,
		MaxPerArtistInRow: a.Cfg.Playback.MaxPerArtistInRow,
		FillerAfter:       time.Duration(a.Cfg.Playback.BackgroundMusic.AfterSeconds) * time.Second,
	}
	if a.bgMusic() {
		o.Filler = a.pickFiller
	}
	return o
}

func (a *App) bgMusic() bool {
	a.stMu.Lock()
	defer a.stMu.Unlock()
	if a.st.BGMusic != nil {
		return *a.st.BGMusic
	}
	return a.Cfg.Playback.BackgroundMusic.Enabled
}

// pickFiller chooses a random audio song for background music. It runs on
// a background goroutine.
func (a *App) pickFiller() (player.Item, bool) {
	all, err := a.Store.Genres(a.ctx, catalog.KindAudio)
	if err != nil {
		return player.Item{}, false
	}
	want := map[string]bool{}
	for _, g := range a.Cfg.Playback.BackgroundMusic.Genres {
		want[strings.ToUpper(strings.TrimSpace(g))] = true
	}
	var ids []int64
	for _, g := range a.visibleGenreFilter(all) {
		if len(want) == 0 || want[strings.ToUpper(g.Name)] || a.kidsMode() {
			ids = append(ids, g.ID)
		}
	}
	if len(ids) == 0 {
		return player.Item{}, false
	}
	for i := 0; i < 15; i++ {
		t, err := a.Store.RandomTrack(a.ctx, ids)
		if err != nil {
			return player.Item{}, false
		}
		if t.IsAudio() {
			return a.itemFor(t), true
		}
	}
	return player.Item{}, false
}

func (a *App) itemFor(t catalog.Track) player.Item {
	return player.Item{
		TrackID: t.ID, Artist: t.Artist, Title: t.Title,
		File: a.Library.Abs(t.File), Type: t.MediaType, Audio: t.IsAudio(),
		Karaoke: t.Kind == catalog.KindKaraoke,
		Folder:  t.Folder, Cover: t.Cover,
	}
}

// enqueue adds a song and reports where it landed.
func (a *App) enqueue(t catalog.Track) {
	if !a.diskOK {
		a.flash("Todavía no está el disco de música")
		return
	}
	// A guest who presses Intro again and again gets the song only once.
	cur, q := a.box.Snapshot()
	if cur != nil && !cur.Filler && cur.TrackID == t.ID {
		a.flash("Ya está sonando: " + t.Title)
		return
	}
	for i, it := range q {
		if it.TrackID == t.ID {
			a.flash(fmt.Sprintf("Ya está en la cola (#%d): %s", i+1, t.Title))
			return
		}
	}
	pos, err := a.box.Add(a.itemFor(t))
	switch {
	case err != nil:
		a.flash("No se agregó: " + err.Error())
	case pos == 0:
		a.celebrate()
		a.flash("▶ " + t.Title)
	case pos == 1:
		a.celebrate()
		a.flash("Sigue: " + t.Title)
		a.MPV.ShowText("Sigue: "+t.Title+" — "+t.Artist, 4*time.Second)
	default:
		a.celebrate()
		a.flash(fmt.Sprintf("En cola (#%d): %s", pos, t.Title))
		a.MPV.ShowText(fmt.Sprintf("En cola #%d: %s", pos, t.Title), 4*time.Second)
	}
}

// backToMenu: the queue ran out — show the menu of the current mode
// (Karaoke, Videos or Música) where the guest left it.
func (a *App) backToMenu() {
	a.vv.Show(false)
	a.scr.root.Show()
	switch {
	case a.ov != nil && (a.ov.name == "cola" || a.ov.name == "top" || a.ov.name == "numero" || a.ov.name == "busqueda"):
		a.closeOverlay()
	}
	if a.idle.visible {
		a.idle.hide()
	}
	a.refreshNotes()
	a.scr.refreshNowPlaying()
	a.flash("Se terminó la cola · elige otra canción")
}

// skipKey: "." skips the current song only when pressed twice within 3 s,
// so one stray press doesn't cut someone's song.
func (a *App) skipKey() {
	cur, q := a.box.Snapshot()
	switch {
	case cur == nil && len(q) > 0:
		a.box.Next()
	case cur == nil:
		a.flash("No hay nada sonando")
	case cur.Filler:
		a.box.Next()
	case time.Since(a.lastSkip) < 5*time.Second:
		// Mashing "." must not run through everybody's songs.
		a.skipArmed = time.Time{}
		a.flash("Espera un momento antes de saltar otra canción")
	case time.Since(a.skipArmed) < 3*time.Second:
		a.skipArmed = time.Time{}
		a.lastSkip = time.Now()
		a.box.Next()
	default:
		a.skipArmed = time.Now()
		msg := "Presiona . otra vez para saltar «" + cur.Title + "»"
		a.flash(msg)
		a.MPV.ShowText(msg, 3*time.Second)
	}
}

// refreshNotes keeps lasting conditions visible in the status bar (a
// forgotten pause or volume 0 make the rockola look broken).
func (a *App) refreshNotes() {
	if a.scr == nil {
		return
	}
	cur, _ := a.box.Snapshot()
	switch {
	case a.diskNote != "":
		a.scr.setStatusSticky(a.diskNote)
	case cur != nil && a.MPV.Paused():
		a.scr.setStatusSticky("⏸ En pausa · presiona * para continuar")
	case a.MPV.Volume() == 0:
		a.scr.setStatusSticky("🔇 Volumen en 0 · presiona + para subir")
	case a.scanNote != "":
		a.scr.setStatusSticky(a.scanNote)
	default:
		a.scr.clearSticky()
	}
	a.updateKeypad()
}

func (a *App) onQueueChange() {
	if !a.audioOn() && a.audioFull {
		a.audioFull = false // the music ended: back to the menu
		a.au.show(false)
	}
	a.refreshNotes()
	a.scr.refreshNowPlaying()
	if a.ov != nil && a.ov.onQueue != nil {
		a.ov.onQueue()
	}
	a.saveState()
}

func (a *App) onSongStart(cur player.Item, next *player.Item) {
	if !cur.Filler && cur.TrackID > 0 {
		a.stMu.Lock()
		ev := a.st.EventID
		a.stMu.Unlock()
		if err := a.Store.RecordPlay(a.ctx, cur.TrackID, ev); err != nil {
			log.Printf("ranking: %v", err)
		}
	}
	if cur.Audio {
		if !cur.Filler {
			fyne.Do(func() {
				a.flash("♪ Empezó: " + cur.Label())
				// A guest's song starts on its player screen (the cover).
				a.idle.hide()
				a.au.start(Song{cur.Title, cur.Artist, cur.Folder, cur.Cover})
				a.audioFull = true
				a.au.show(true)
				a.refreshNotes()
				a.updateCountdown()
			})
		}
		return
	}
	fyne.Do(func() {
		a.idle.hide() // the idle screen would cover the video
		a.vv.startSong(cur.Title, cur.Artist)
		a.refreshNotes()
	})
	// The title over the video, once it is playing. Who comes next is
	// told near the end (onProgress), not now.
	time.Sleep(1500 * time.Millisecond)
	a.MPV.ShowText("▶ "+cur.Label(), time.Duration(a.Cfg.Playback.NextUpSeconds)*time.Second)
}

// progressThrottled hands mpv's position (it reports every frame) to the
// screen at most progressEvery, so the progress bars cost nothing while a
// video plays on a slow machine. A jump back (a new song) goes through.
func (a *App) progressThrottled() func(pos, dur float64) {
	return throttle(func(pos, dur float64) { fyne.Do(func() { a.onProgress(pos, dur) }) })
}

// throttle passes on one report per progressEvery (and any jump back).
func throttle(fn func(pos, dur float64)) func(pos, dur float64) {
	var mu sync.Mutex
	var last time.Time
	var lastPos float64
	return func(pos, dur float64) {
		mu.Lock()
		now := time.Now()
		ok := now.Sub(last) >= progressEvery || pos < lastPos
		if ok {
			last = now
		}
		lastPos = pos
		mu.Unlock()
		if ok {
			fn(pos, dur)
		}
	}
}

// nextUpBefore: the "Siguiente: …" notice shows this long before the end.
const nextUpBefore = 10 * time.Second

// progressEvery: the progress bars move twice a second.
const progressEvery = 500 * time.Millisecond

func (a *App) onProgress(pos, dur float64) {
	a.scr.setProgress(pos, dur)
	if dur > 0 && a.audioOn() {
		a.au.progress.set(pos / dur)
		left := dur - pos
		next := ""
		if _, q := a.box.Snapshot(); len(q) > 0 && dur > 2*nextUpBefore.Seconds() && left > 0 && left <= nextUpBefore.Seconds() {
			next = "Siguiente: " + q[0].Label()
		}
		a.au.setNext(next)
	}
	// Tell what comes next in the last nextUpBefore of the song, until it ends.
	if left := dur - pos; dur > 2*nextUpBefore.Seconds() && left > 0 && left <= nextUpBefore.Seconds() {
		cur, q := a.box.Snapshot()
		if cur != nil && len(q) > 0 && a.nextShown != cur.TrackID {
			a.nextShown = cur.TrackID
			a.MPV.ShowText("Siguiente: "+q[0].Label(), time.Duration(left*float64(time.Second)))
		}
	}
}

func (a *App) onPlayError(it player.Item, err error) {
	log.Printf("reproducción: %v", err)
	msg := "No se pudo reproducir: " + it.Title
	// Hide songs whose file is gone, but only if the disk itself is still
	// there (an unplugged disk would otherwise hide the whole catalog).
	if strings.Contains(err.Error(), "no está en el disco") && it.TrackID > 0 {
		if _, derr := a.Library.Detect(); derr == nil {
			_ = a.Store.SetMissing(a.ctx, it.TrackID, true)
			msg = "Falta el archivo de " + it.Title + "; se ocultó"
		} else {
			a.diskOK = false
			msg = "⚠ Se desconectó el disco de música"
			go a.findDisk()
		}
	}
	a.flash(msg)
}

// ---------- state ----------

// saveState keeps the volume and admin settings. The queue is never saved:
// after a restart the rockola opens silent with an empty queue.
func (a *App) saveState() {
	a.stMu.Lock()
	a.st.Volume = a.MPV.Volume()
	st := a.st
	a.stMu.Unlock()
	if err := a.State.Save(st); err != nil {
		log.Printf("estado: %v", err)
	}
}

func (a *App) updateState(fn func(*state.State)) {
	a.stMu.Lock()
	fn(&a.st)
	a.stMu.Unlock()
	a.saveState()
}

// ---------- keys ----------

// onKey handles a key from the Fyne window or forwarded by mpv.
func (a *App) onKey(name fyne.KeyName, fromMPV bool) {
	a.lastKey = time.Now()
	a.resetIdle()
	if a.scr != nil {
		a.scr.keypad.flash(a.keyCell(name))
	}

	// mpv forwards letters and digits as key names; Fyne sends them as runes too.
	if fromMPV && len(name) == 1 {
		r := []rune(strings.ToLower(string(name)))[0]
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			defer a.onRune(r)
		}
	}
	if fromMPV && name == fyne.KeySpace {
		defer a.onRune(' ')
	}

	if a.idle.visible {
		a.idle.hide()
		return
	}
	if a.checkAdminSequence(name) {
		return
	}
	action := a.actionFor(name)

	// Two modes while a video (karaoke or music video) plays:
	//   - Reproducción: the video on the whole screen (singing).
	//   - Lista: the menu, with the video going on in the preview box.
	// 7 always toggles between them (see browseKey "top"). In Reproducción,
	// volume and pause work as usual and ANY other key also opens the list
	// (the key is consumed); "." also prepares the skip, so a second "."
	// skips. The list goes back to Reproducción by itself after
	// videoBackDelay without keys.
	if a.playerOn() {
		a.armVideoFullscreen()
		if action != "volume_up" && action != "volume_down" && action != "pause" && a.leavePlayer() {
			a.refreshNotes()
			a.updateCountdown()
			if action == "next" {
				a.skipKey()
			} else {
				a.flash(a.listModeMsg())
			}
			return
		}
	}

	if a.ov != nil {
		a.ov.key(name, action)
		return
	}
	if a.layer != nil {
		a.layer.key(name, action)
		return
	}
	a.browseKey(name, action)
}

// listModeMsg tells the guest the song goes on and how to return to it.
func (a *App) listModeMsg() string {
	cur, _ := a.box.Snapshot()
	if cur != nil && cur.Karaoke {
		return "El karaoke sigue en el recuadro · 7 para volver a cantar"
	}
	if cur != nil && cur.Audio {
		return "La música sigue sonando · 7 para ver la canción"
	}
	return "El video sigue en el recuadro · 7 para verlo en grande"
}

// onRune receives typed characters (full keyboard): letters open the search.
func (a *App) onRune(r rune) {
	if a.idle.visible {
		return
	}
	if a.ov != nil {
		if a.ov.rune != nil {
			a.ov.rune(r)
		}
		return
	}
	if a.layer != nil {
		return
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("áéíóúñÁÉÍÓÚÑ", r) {
		a.openSearch(string(r))
	}
}

func (a *App) actionFor(name fyne.KeyName) string {
	if act, ok := a.keys[name]; ok {
		return act
	}
	switch name {
	case fyne.KeyUp:
		return "up"
	case fyne.KeyDown:
		return "down"
	case fyne.KeyLeft:
		return "left"
	case fyne.KeyRight:
		return "right"
	case fyne.KeyReturn, fyne.KeyEnter:
		return "ok"
	case fyne.KeyPageUp:
		return "page_up"
	case fyne.KeyPageDown:
		return "page_down"
	case fyne.KeyEscape:
		return "back"
	case fyne.KeyBackspace:
		return "backspace"
	}
	return ""
}

// checkAdminSequence opens the PIN prompt after OpenSequence (default Esc).
func (a *App) checkAdminSequence(name fyne.KeyName) bool {
	if len(a.adminSeq) == 0 {
		return false
	}
	now := time.Now()
	a.recent = append(a.recent, keyStamp{name, now})
	if len(a.recent) > len(a.adminSeq) {
		a.recent = a.recent[len(a.recent)-len(a.adminSeq):]
	}
	if len(a.recent) < len(a.adminSeq) || now.Sub(a.recent[0].t) > 3*time.Second {
		return false
	}
	// A single key (Esc) also closes windows: only open the admin from the
	// main screen.
	if len(a.adminSeq) == 1 && a.ov != nil {
		return false
	}
	for i, k := range a.adminSeq {
		if a.recent[i].k != k {
			return false
		}
	}
	a.recent = nil
	a.openPIN()
	return true
}

func (a *App) browseKey(name fyne.KeyName, action string) {
	if a.navKey(action) {
		return
	}
	switch action {
	case "next":
		a.skipKey()
	case "genres":
		a.nextGenre()
	case "volume_up", "volume_down":
		d := 1
		if action == "volume_down" {
			d = -1
		}
		v := a.MPV.ChangeVolume(d)
		a.flash(fmt.Sprintf("Volumen: %d de %d", v, a.MPV.MaxVolume()))
		a.MPV.ShowText(fmt.Sprintf("Volumen %d", v), 2*time.Second)
		a.saveState()
		a.refreshNotes()
	case "pause":
		if cur, _ := a.box.Snapshot(); cur != nil {
			if a.MPV.TogglePause() {
				a.MPV.ShowText("⏸ En pausa · * para continuar", 3*time.Second)
			} else {
				a.flash("▶ Continúa")
			}
			a.refreshNotes()
		}
	case "playlist":
		a.openQueue()
	case "abc":
		a.openNumber()
	case "top":
		if a.backToVideo() {
			return
		}
		a.openTop()
	case "reset":
		a.openModes()
	}
}

// backToVideo puts the player back on the whole screen: the video waiting
// in the preview box, or the music's cover screen (7, or 6 from the songs
// column). False if nothing is playing there.
func (a *App) backToVideo() bool {
	if !a.playerOn() || a.playerFull() {
		return false
	}
	a.setPlayerFull(true)
	a.refreshNotes()
	a.updateCountdown()
	return true
}

// The player (Reproducción) is a video, or a guest's music without video
// (its cover screen). Background music is not: it plays under the menu.

func (a *App) audioOn() bool {
	cur, _ := a.box.Snapshot()
	return cur != nil && cur.Audio && !cur.Filler
}

func (a *App) playerOn() bool { return a.MPV.VideoPlaying() || a.audioOn() }

func (a *App) playerFull() bool {
	if a.MPV.VideoPlaying() {
		return a.MPV.VideoFull()
	}
	return a.audioFull && a.audioOn()
}

func (a *App) setPlayerFull(full bool) {
	if a.MPV.VideoPlaying() {
		a.MPV.SetSmall(!full)
		return
	}
	a.audioFull = full && a.audioOn()
	a.au.show(a.audioFull)
}

// leavePlayer goes from Reproducción to Lista and reports whether it did
// (the key that woke the menu is then consumed).
func (a *App) leavePlayer() bool {
	if a.MPV.VideoPlaying() {
		return a.MPV.SetSmallIfFull()
	}
	if !a.playerFull() {
		return false
	}
	a.setPlayerFull(false)
	return true
}

// ---------- idle / video timers ----------

func (a *App) resetIdle() {
	if a.idleTimer != nil {
		a.idleTimer.Stop()
	}
	d := a.Cfg.IdleDuration()
	if d <= 0 {
		return
	}
	a.idleTimer = time.AfterFunc(d, func() {
		fyne.Do(func() {
			if a.ov == nil && a.layer == nil && !a.playerOn() {
				a.idle.show()
			}
		})
	})
}

// armVideoFullscreen sends the video back to fullscreen after videoBackDelay
// (10 s) without keys.
// The countdown restarts with every key; with a dialog open it waits and
// starts again when the dialog closes. It is shown under the preview box.
func (a *App) armVideoFullscreen() {
	if a.videoTimer != nil {
		a.videoTimer.Stop()
	}
	d := a.videoBackDelay
	a.videoBackAt = time.Now().Add(d)
	a.videoTimer = time.AfterFunc(d, func() {
		fyne.Do(func() {
			if a.ov == nil && a.layer == nil && !time.Now().Before(a.videoBackAt) {
				a.setPlayerFull(true)
				a.refreshNotes()
			}
			a.updateCountdown()
		})
	})
	a.startCountdown()
	a.updateCountdown()
}

// rearmVideoFullscreen restarts the countdown after a dialog closes.
func (a *App) rearmVideoFullscreen() {
	if a.playerOn() && !a.playerFull() {
		a.armVideoFullscreen()
	}
}

// startCountdown ticks once a second while a video waits in the preview box.
func (a *App) startCountdown() {
	if a.countdownOn {
		return
	}
	a.countdownOn = true
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			stop := false
			fyne.DoAndWait(func() {
				a.updateCountdown()
				if !a.playerOn() || a.playerFull() {
					a.countdownOn = false
					stop = true
				}
			})
			if stop || a.ctx.Err() != nil {
				return
			}
		}
	}()
}

// updateCountdown shows how long until the video takes the whole screen.
func (a *App) updateCountdown() {
	if a.scr == nil {
		return
	}
	txt := ""
	if a.playerOn() && !a.playerFull() {
		if a.ov != nil || a.layer != nil {
			txt = "Pantalla completa al cerrar"
		} else {
			secs := int(time.Until(a.videoBackAt).Seconds() + 0.999)
			txt = fmt.Sprintf("Pantalla completa en %d s · 7 ya", max(secs, 0))
		}
	}
	if a.scr.countdown.Text != txt {
		a.scr.countdown.Text = txt
		a.scr.countdown.Refresh()
		// Same line as "AHORA SUENA": show one or the other.
		a.scr.npTag.Hidden = txt != ""
		a.scr.npTag.Refresh()
	}
}

// updatePreviewGeometry tells mpv where the preview box is on screen.
func (a *App) updatePreviewGeometry() {
	if a.window == nil {
		return
	}
	pos, size := a.scr.lay.screenRect(previewRect)
	sc := a.window.Canvas().Scale()
	cs := a.window.Canvas().Size()
	a.vv.setTargets([2]int{int(size.Width * sc), int(size.Height * sc)}, [2]int{int(cs.Width * sc), int(cs.Height * sc)})
	_ = pos
}

func (a *App) flash(msg string) {
	if a.scr != nil {
		a.scr.setStatus(msg)
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// refreshCovers redraws covers after new thumbnails were made.
func (a *App) refreshCovers() {
	if a.scr == nil {
		return
	}
	a.scr.artists.refresh()
	a.scr.refreshPreview()
}
