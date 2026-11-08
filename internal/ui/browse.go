package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/josegamboa/karaokenino/internal/catalog"
)

// Screen layout (design pixels, 1280×720).
var (
	headerRect  = Rect{0, 0, designW, 72}
	lettersRect = Rect{40, 112, 92, 530}
	artistsRect = Rect{146, 112, 406, 530}
	songsRect   = Rect{566, 112, 384, 352}
	queueRect   = Rect{566, 486, 384, 154} // under the songs: what comes next
	previewRect = Rect{966, 112, 274, 154}
	keypadRect  = Rect{966, 356, 274, 286}
	statusRect  = Rect{0, 662, designW, 58}
	// The song's progress runs along the top edge of the status bar, full
	// width: at the very bottom of the screen without risking the TV's
	// overscan cutting it off.
	progressRect = Rect{0, statusRect.Top - 2, designW, 4}
)

// queueRows is how many lines the upcoming-queue panel has.
const queueRows = 5

// Columns of the browser; 4/6 move between them.
const (
	colLetters = iota
	colArtists
	colSongs
)

// browseScreen is the main screen: letters → artists → songs, a preview of
// the artist or the video, what is playing, and the keypad.
type browseScreen struct {
	a    *App
	lay  *scaledLayout
	root *fyne.Container
	pal  *Palette
	bg   *partyBackground // animated party background (background.go)

	genre, genreInfo        *canvas.Text
	colTitle                [3]*canvas.Text
	colPanel                [3]*canvas.Rectangle
	letters, artists, songs *rowList

	prevImg              *canvas.Image
	prevInitials         *canvas.Text
	npTag, npTitle, npBy *canvas.Text
	countdown            *canvas.Text // " en 14 s": video back to fullscreen
	progress             *progressBar
	progTrack            int64 // song the progress bar belongs to
	keypad               *keypad

	// upcoming queue under the songs
	queueTitle         *canvas.Text
	queueNum, queueTxt [queueRows]*canvas.Text

	status, queueInfo *canvas.Text
	sticky            string
	statusTimer       *time.Timer
}

func newBrowseScreen(a *App) *browseScreen {
	s := &browseScreen{a: a, pal: &a.pal, lay: newScaledLayout(designW, designH)}
	s.lay.onResize = func() { a.updatePreviewGeometry() }
	pal := s.pal
	var objs []fyne.CanvasObject
	add := func(o ...fyne.CanvasObject) { objs = append(objs, o...) }

	s.bg = newPartyBackground(s.lay, pal)
	add(s.bg.objects...)
	line := canvas.NewLinearGradient(pal.Accent, pal.Accent2, 90)
	add(s.lay.place(line, Rect{0, headerRect.Y2() - 3, designW, 3}))

	// Header: mode and genre only (the business name and the event
	// message are on the welcome and idle screens, not here).
	s.genre = newText("", pal.Text, true)
	add(s.lay.placeText(s.genre, Rect{40, 12, 900, 32}, 26))
	s.genreInfo = newText("", pal.TextDim, false)
	add(s.lay.placeText(s.genreInfo, Rect{40, 44, 900, 18}, 13))

	// Three columns.
	cols := []Rect{lettersRect, artistsRect, songsRect}
	for i, r := range cols {
		s.colPanel[i] = newPanel(pal.Panel, pal.PanelBorder, 14)
		s.colPanel[i].StrokeWidth = 2
		add(s.lay.place(s.colPanel[i], grow(r, 6)))
		s.colTitle[i] = newText("", pal.TextDim, true)
		add(s.lay.placeText(s.colTitle[i], Rect{r.Left, 82, r.Width + 6, 22}, 15))
	}
	s.letters = newList(s.lay, pal, lettersRect, 53, 30, 0, 0, 0)
	s.artists = newList(s.lay, pal, artistsRect, 53, 20, 0, 42, 0)
	s.artists.empty = "Sin artistas"
	s.songs = newList(s.lay, pal, songsRect, 44, 19, 60, 0, 78)
	s.songs.empty = "Sin canciones"
	add(s.letters.objects()...)
	add(s.artists.objects()...)
	add(s.songs.objects()...)

	// Upcoming queue: a guest who adds a song sees it land right below.
	qr := queueRect
	add(s.lay.place(newPanel(pal.Panel, pal.PanelBorder, 14), grow(qr, 6)))
	s.queueTitle = newText("", pal.TextDim, true)
	add(s.lay.placeText(s.queueTitle, Rect{qr.Left + 8, qr.Top + 2, qr.Width - 16, 20}, 15))
	for i := range queueRows {
		y := qr.Top + 28 + i*25
		s.queueNum[i] = newText("", pal.TextDim, true)
		s.queueNum[i].Alignment = fyne.TextAlignTrailing
		add(s.lay.placeText(s.queueNum[i], Rect{qr.Left, y, 30, 22}, 15))
		s.queueTxt[i] = newText("", pal.Text, false)
		add(s.lay.placeText(s.queueTxt[i], Rect{qr.Left + 40, y, qr.Width - 48, 22}, 16))
	}

	// Right: cover or video, what is playing, the keypad.
	pr := previewRect
	add(s.lay.place(newPanel(pal.Panel, pal.PanelBorder, 14), grow(pr, 6)))
	box := color.NRGBA{0, 0, 0, 170}
	if pal.light() {
		box = rgba(255, 255, 255, 200)
	}
	add(s.lay.place(newPanel(box, color.NRGBA{}, 8), pr))
	s.prevInitials = newText("", withAlpha(pal.Text, 180), true)
	s.prevInitials.Alignment = fyne.TextAlignCenter
	add(s.lay.placeText(s.prevInitials, Rect{pr.Left, pr.Top + pr.Height/2 - 34, pr.Width, 68}, 56))
	s.prevImg = canvas.NewImageFromResource(nil)
	s.prevImg.FillMode = canvas.ImageFillContain
	add(s.lay.place(s.prevImg, pr))
	s.npTag = newText("", pal.Accent2, true)
	add(s.lay.placeText(s.npTag, Rect{pr.Left, pr.Y2() + 12, pr.Width, 16}, 13))
	s.countdown = newText("", pal.Text, true)
	s.countdown.Alignment = fyne.TextAlignTrailing
	add(s.lay.placeText(s.countdown, Rect{pr.Left, pr.Y2() + 12, pr.Width, 16}, 13))
	s.npTitle = newText("", pal.Text, true)
	add(s.lay.placeText(s.npTitle, Rect{pr.Left, pr.Y2() + 30, pr.Width, 24}, 19))
	s.npBy = newText("", pal.TextDim, false)
	add(s.lay.placeText(s.npBy, Rect{pr.Left, pr.Y2() + 54, pr.Width, 20}, 15))
	var padObjs []fyne.CanvasObject
	s.keypad, padObjs = newKeypad(s.lay, pal, keypadRect)
	add(padObjs...)

	// Status bar: messages on the left, the queue on the right.
	sb := statusRect
	add(s.lay.place(canvas.NewRectangle(pal.Band), sb))
	s.status = newText("", pal.TextDim, false)
	add(s.lay.placeText(s.status, Rect{20, sb.Top + 18, 700, 24}, 18))
	s.queueInfo = newText("", pal.Text, true)
	s.queueInfo.Alignment = fyne.TextAlignTrailing
	add(s.lay.placeText(s.queueInfo, Rect{720, sb.Top + 18, 540, 24}, 18))
	s.progress = newProgressBar(s.lay, pal, progressRect)
	add(s.progress.bg, s.progress.fill)

	s.root = container.New(s.lay, objs...)
	s.restoreStatus()
	s.refreshNowPlaying()
	return s
}

// ---------- status ----------

func (s *browseScreen) setStatus(msg string) {
	s.status.Text = fit(msg, 700, 18)
	s.status.Color = s.pal.Text
	s.status.Refresh()
	if s.statusTimer != nil {
		s.statusTimer.Stop()
	}
	s.statusTimer = time.AfterFunc(5*time.Second, func() { fyne.Do(s.restoreStatus) })
}

func (s *browseScreen) restoreStatus() {
	switch {
	case s.sticky != "":
		s.status.Text = fit(s.sticky, 700, 18)
		s.status.Color = s.pal.Accent2
	default:
		s.status.Text = "Elige letra → artista → canción  ·  las teclas de la derecha dicen qué hace cada una"
		s.status.Color = s.pal.TextDim
	}
	s.status.Refresh()
}

func (s *browseScreen) setStatusSticky(msg string) {
	s.sticky = msg
	s.restoreStatus()
}

func (s *browseScreen) clearSticky() {
	s.sticky = ""
	s.restoreStatus()
}

// ---------- now playing / preview ----------

func (s *browseScreen) refreshNowPlaying() {
	cur, q := s.a.box.Snapshot()
	switch {
	case cur == nil && len(q) > 0:
		s.queueInfo.Text = fmt.Sprintf("%d en cola · presiona . para empezar", len(q))
	case cur == nil:
		s.queueInfo.Text = "Cola vacía · elige una canción y presiona Intro"
	case cur.Filler:
		s.queueInfo.Text = "♪ Música de fondo"
	default:
		txt := fmt.Sprintf("%d en cola", len(q))
		if len(q) > 0 {
			txt = "Sigue: " + q[0].Title + "  ·  " + txt
		}
		s.queueInfo.Text = fit(txt, 540, 18)
	}
	s.queueInfo.Refresh()
	s.refreshQueue()
	s.refreshPreview()
	s.a.updateKeypad()
}

// refreshQueue lists the next songs under the songs column: up to
// queueRows, or queueRows-1 and "+N más" when there are more.
func (s *browseScreen) refreshQueue() {
	cur, q := s.a.box.Snapshot()
	switch {
	case cur == nil && len(q) > 0:
		s.queueTitle.Text = fmt.Sprintf("EN COLA (%d) · . para empezar", len(q))
	default:
		s.queueTitle.Text = fmt.Sprintf("SIGUEN EN LA COLA (%d)", len(q))
	}
	s.queueTitle.Text = fit(s.queueTitle.Text, queueRect.Width-16, 15)
	shown := len(q)
	if shown > queueRows {
		shown = queueRows - 1
	}
	textW := queueRect.Width - 48
	for i := range queueRows {
		num, txt := s.queueNum[i], s.queueTxt[i]
		num.Text, txt.Text = "", ""
		txt.Color, txt.TextStyle.Bold = s.pal.Text, false
		num.Color = s.pal.TextDim
		switch {
		case i < shown:
			num.Text = fmt.Sprint(i + 1)
			label := q[i].Title
			if q[i].Artist != "" {
				label += "  ·  " + q[i].Artist
			}
			txt.Text = fit(label, textW, 16)
			if i == 0 { // the next to sing stands out
				num.Color, txt.TextStyle.Bold = s.pal.Accent2, true
			}
		case i == shown && len(q) > shown:
			txt.Text = fmt.Sprintf("+%d más · / para ver toda la cola", len(q)-shown)
			txt.Color = s.pal.TextDim
		case i == 0 && len(q) == 0:
			txt.Text = fit("Vacía · Intro agrega la canción", textW, 16)
			txt.Color = s.pal.TextDim
		}
		num.Refresh()
		txt.Refresh()
	}
	s.queueTitle.Refresh()
}

// refreshPreview shows what is playing, or the selected artist's cover.
func (s *browseScreen) refreshPreview() {
	a := s.a
	cur, _ := a.box.Snapshot()
	switch {
	case cur != nil:
		s.npTag.Text = "AHORA SUENA"
		if cur.Filler {
			s.npTag.Text = "MÚSICA DE FONDO"
		}
		s.npTitle.Text = fit(cur.Title, previewRect.Width, 19)
		s.npBy.Text = fit(cur.Artist, previewRect.Width, 15)
		if s.progTrack != cur.TrackID {
			s.progTrack = cur.TrackID
			s.progress.set(0) // a new song starts at the left
			s.a.vv.setProgress(0)
		}
		s.progress.show(true)
		if cur.Audio {
			s.setPreviewImage(a.coverFor(cur.Folder, cur.Cover), cur.Artist)
		} else {
			s.setPreviewImage("", "") // the video is drawn over this box
		}
	default:
		s.npTag.Text, s.npTitle.Text, s.npBy.Text = "", "", ""
		s.progTrack = 0
		s.progress.show(false)
		if ar, ok := a.selectedArtist(); ok {
			s.setPreviewImage(a.coverFor(ar.Folder, ar.Cover), ar.Name)
		} else {
			s.setPreviewImage("", "")
		}
	}
	s.npTag.Refresh()
	s.npTitle.Refresh()
	s.npBy.Refresh()
}

func (s *browseScreen) setPreviewImage(file, name string) {
	if s.prevImg.File != file {
		s.prevImg.File = file
		s.prevImg.Resource = nil
	}
	s.prevImg.Hidden = file == ""
	s.prevImg.Refresh()
	s.prevInitials.Text = ""
	if file == "" && name != "" {
		s.prevInitials.Text = initials(name)
	}
	s.prevInitials.Refresh()
}

func (s *browseScreen) setProgress(pos, dur float64) {
	if dur > 0 {
		s.progress.set(pos / dur)
		s.a.vv.setProgress(pos / dur)
	}
}

// bigCoverFor is coverFor with the large thumbnails (music player screen).
func (a *App) bigCoverFor(folder, cover string) string {
	if folder == "" && cover == "" {
		return ""
	}
	if !a.diskOK {
		return a.bigThumbs.Get(folder+"|"+cover, nil)
	}
	return a.bigThumbs.Get(folder+"|"+cover, func() string { return a.covers.For(folder, cover) })
}

// coverFor returns the artist's thumbnail ("" while it is being made or if
// there is no cover). It never reads the music disk on the screen thread.
func (a *App) coverFor(folder, cover string) string {
	if folder == "" && cover == "" {
		return ""
	}
	if !a.diskOK {
		// No disk yet: don't look (and don't remember "no cover").
		return a.thumbs.Get(folder+"|"+cover, nil)
	}
	return a.thumbs.Get(folder+"|"+cover, func() string { return a.covers.For(folder, cover) })
}

// ---------- data ----------

func (a *App) currentGenre() catalog.Genre {
	if a.genreIdx < 0 || a.genreIdx >= len(a.genres) {
		return catalog.Genre{}
	}
	return a.genres[a.genreIdx]
}

func (a *App) currentLetter() string {
	if a.letterIdx < 0 || a.letterIdx >= len(a.letters) {
		return ""
	}
	return a.letters[a.letterIdx].Letter
}

func (a *App) selectedArtist() (catalog.Artist, bool) {
	if a.artistIdx < 0 || a.artistIdx >= len(a.artists) {
		return catalog.Artist{}, false
	}
	return a.artists[a.artistIdx], true
}

func (a *App) loadGenres() error {
	all, err := a.Store.Genres(a.ctx, a.mode)
	if err != nil {
		return err
	}
	a.genres = a.visibleGenreFilter(all)
	if len(a.genres) == 0 && a.mode != catalog.KindAll {
		// Nothing of this kind (e.g. kids mode has no karaoke): show everything.
		if any, _ := a.Store.Genres(a.ctx, catalog.KindAll); len(a.visibleGenreFilter(any)) > 0 {
			a.mode = catalog.KindAll
			return a.loadGenres()
		}
	}
	a.genreIdx = 0
	for i, g := range a.genres {
		if strings.Contains(strings.ToUpper(g.Name), "KARAOKE") && (a.mode == catalog.KindKaraoke || a.mode == catalog.KindAll) {
			a.genreIdx = i
			break
		}
	}
	a.focus = colLetters
	if err := a.loadLetters(""); err != nil {
		return err
	}
	if len(a.genres) == 0 {
		if len(all) == 0 {
			return fmt.Errorf("catálogo vacío: conecta el disco o importa Musica.data")
		}
		return fmt.Errorf("no hay géneros visibles (revisa el modo infantil)")
	}
	return nil
}

// loadLetters fills the letter column of the current genre, keeping letter if possible.
func (a *App) loadLetters(keep string) error {
	g := a.currentGenre()
	a.letters = nil
	if g.ID != 0 {
		ls, err := a.Store.Letters(a.ctx, g.ID, a.mode)
		if err != nil {
			return err
		}
		a.letters = ls
	}
	a.letterIdx = 0
	for i, l := range a.letters {
		if l.Letter == keep {
			a.letterIdx = i
		}
	}
	items := make([]rowItem, len(a.letters))
	for i, l := range a.letters {
		items[i] = rowItem{Text: "  " + strings.ToUpper(l.Letter)}
	}
	a.scr.letters.set(items, a.letterIdx)
	a.renderHeader()
	return a.loadArtists(0)
}

func (a *App) loadArtists(sel int) error {
	a.artists = nil
	if l := a.currentLetter(); l != "" {
		as, err := a.Store.ArtistsByLetter(a.ctx, a.currentGenre().ID, a.mode, l)
		if err != nil {
			return err
		}
		a.artists = as
	}
	a.artistIdx = min(max(sel, 0), max(len(a.artists)-1, 0))
	items := make([]rowItem, len(a.artists))
	for i, ar := range a.artists {
		items[i] = rowItem{Text: ar.Name, Initials: initials(ar.Name)}
	}
	a.scr.artists.iconFn = func(i int) string {
		if i < len(a.artists) {
			return a.coverFor(a.artists[i].Folder, a.artists[i].Cover)
		}
		return ""
	}
	a.scr.artists.set(items, a.artistIdx)
	return a.loadTracks(0)
}

func (a *App) loadTracks(sel int) error {
	a.tracks = nil
	if ar, ok := a.selectedArtist(); ok {
		ts, err := a.Store.Tracks(a.ctx, ar.ID, a.mode)
		if err != nil {
			return err
		}
		a.tracks = ts
	}
	a.trackIdx = min(max(sel, 0), max(len(a.tracks)-1, 0))
	artist := ""
	if ar, ok := a.selectedArtist(); ok {
		artist = ar.Name
	}
	items := make([]rowItem, len(a.tracks))
	for i, t := range a.tracks {
		badge, col := badgeFor(&a.pal, t.Kind)
		sub := ""
		if t.Artist != artist {
			sub = t.Artist
		}
		items[i] = rowItem{Num: fmt.Sprint(t.ID), Text: t.Title, Sub: sub, Badge: badge, Color: col}
	}
	a.scr.songs.set(items, a.trackIdx)
	a.renderColumns()
	a.scr.refreshPreview()
	return nil
}

// refreshArtists reloads after a disk change, keeping letter and artist.
func (a *App) refreshArtists() {
	letter := a.currentLetter()
	var artistID int64
	if ar, ok := a.selectedArtist(); ok {
		artistID = ar.ID
	}
	focus := a.focus
	_ = a.loadLetters(letter)
	for i, ar := range a.artists {
		if ar.ID == artistID {
			a.artistIdx = i
			a.scr.artists.selectIdx(i)
			_ = a.loadTracks(0)
		}
	}
	a.focus = focus
	a.renderColumns()
}

// ---------- rendering ----------

func (a *App) renderHeader() {
	s := a.scr
	g := a.currentGenre()
	s.genre.Text = strings.ToUpper(modeName(a.mode)) + "  ·  " + g.Name
	s.genre.Refresh()
	s.genreInfo.Text = fmt.Sprintf("género %d de %d", a.genreIdx+1, max(len(a.genres), 1))
	s.genreInfo.Refresh()
}

// renderColumns updates column titles, focus marks and the keypad labels.
func (a *App) renderColumns() {
	s := a.scr
	letter := strings.ToUpper(a.currentLetter())
	s.colTitle[colLetters].Text = "LETRA"
	s.colTitle[colArtists].Text = fit(fmt.Sprintf("ARTISTAS · %s (%d)", letter, len(a.artists)), artistsRect.Width, 15)
	artist := ""
	if ar, ok := a.selectedArtist(); ok {
		artist = ar.Name
	}
	s.colTitle[colSongs].Text = fit(fmt.Sprintf("CANCIONES · %s (%d)", artist, len(a.tracks)), songsRect.Width, 15)
	lists := []*rowList{s.letters, s.artists, s.songs}
	for i := range lists {
		active := i == a.focus
		lists[i].setActive(active)
		if active {
			s.colTitle[i].Color = s.pal.Accent2
			s.colPanel[i].StrokeColor = s.pal.Accent
		} else {
			s.colTitle[i].Color = s.pal.TextDim
			s.colPanel[i].StrokeColor = s.pal.PanelBorder
		}
		s.colTitle[i].Refresh()
		s.colPanel[i].Refresh()
	}
	a.updateKeypad()
}

// updateKeypad shows what each key does in the current column.
func (a *App) updateKeypad() {
	if a.scr == nil || a.scr.keypad == nil {
		return
	}
	labels := map[string]string{
		"next": "Saltar", "genres": "Género", "volume_up": "Vol +", "volume_down": "Vol −",
		"pause": "Pausa", "playlist": "Cola", "abc": "Número", "reset": "Modo", "top": "Top",
		"page_up": "Pág ↑", "page_down": "Pág ↓",
	}
	switch a.focus {
	case colLetters:
		labels["up"], labels["down"], labels["right"], labels["ok"] = "Letra ↑", "Letra ↓", "Artistas", "Artistas"
	case colArtists:
		labels["up"], labels["down"], labels["left"], labels["right"], labels["ok"] = "Artista ↑", "Artista ↓", "Letras", "Canciones", "Canciones"
	case colSongs:
		labels["up"], labels["down"], labels["left"], labels["ok"] = "Canción ↑", "Canción ↓", "Artistas", "Agregar"
	}
	if a.MPV.Paused() {
		labels["pause"] = "Continuar"
	}
	if a.playerOn() && !a.playerFull() {
		// 7 toggles Lista → Reproducción (the player on the whole screen).
		labels["top"] = "Ver video"
		if cur, _ := a.box.Snapshot(); cur != nil && cur.Karaoke {
			labels["top"] = "Cantar"
		} else if cur != nil && cur.Audio {
			labels["top"] = "Canción"
		}
		if a.focus == colSongs {
			labels["right"] = labels["top"] // 6 past the last column: back to the video
		}
	}
	if cur, q := a.box.Snapshot(); cur == nil {
		delete(labels, "pause")
		if len(q) > 0 {
			labels["next"] = "Continuar"
		} else {
			delete(labels, "next")
		}
	}
	cells := map[string]string{}
	for act, lbl := range labels {
		if c := a.actionCells[act]; c != "" {
			cells[c] = lbl
		}
	}
	a.scr.keypad.setLabels(cells)
}

// ---------- navigation ----------

// navKey moves through letters → artists → songs.
func (a *App) navKey(action string) bool {
	s := a.scr
	switch action {
	case "up", "down", "page_up", "page_down":
		d := map[string]int{"up": -1, "down": 1, "page_up": -8, "page_down": 8}[action]
		page := action == "page_up" || action == "page_down"
		switch a.focus {
		case colLetters:
			a.letterIdx = stepIdx(a.letterIdx, d, len(a.letters), page)
			s.letters.selectIdx(a.letterIdx)
			_ = a.loadArtists(0)
		case colArtists:
			a.artistIdx = stepIdx(a.artistIdx, d, len(a.artists), page)
			s.artists.selectIdx(a.artistIdx)
			_ = a.loadTracks(0)
		case colSongs:
			a.trackIdx = stepIdx(a.trackIdx, d, len(a.tracks), page)
			s.songs.selectIdx(a.trackIdx)
			a.renderColumns()
		}
	case "right":
		if a.focus < colSongs {
			a.focus++
			a.renderColumns()
		} else {
			a.backToVideo() // nothing further right: back to the player
		}
	case "left":
		if a.focus > colLetters {
			a.focus--
			a.renderColumns()
		}
	case "ok":
		if a.focus < colSongs {
			a.focus++
			a.renderColumns()
		} else if a.trackIdx < len(a.tracks) {
			a.enqueue(a.tracks[a.trackIdx])
		}
	default:
		return false
	}
	return true
}

// stepIdx moves i by d in a list of n: single steps wrap around, page jumps stop at the ends.
func stepIdx(i, d, n int, page bool) int {
	if n == 0 {
		return 0
	}
	if page {
		return min(max(i+d, 0), n-1)
	}
	return ((i+d)%n + n) % n
}

func (a *App) nextGenre() {
	if len(a.genres) == 0 {
		return
	}
	a.genreIdx = (a.genreIdx + 1) % len(a.genres)
	a.focus = colLetters
	_ = a.loadLetters(a.currentLetter())
	a.flash("Género: " + a.currentGenre().Name)
}

// showTrack moves the browser to a song (from the search or "lo más pedido").
func (a *App) showTrack(t catalog.Track) {
	if a.mode != catalog.KindAll && t.Kind != a.mode {
		a.mode = t.Kind
		_ = a.loadGenres()
	}
	ar, err := a.Store.Artist(a.ctx, t.ArtistID)
	if err != nil {
		return
	}
	for gi, g := range a.genres {
		if g.ID != ar.GenreID {
			continue
		}
		a.genreIdx = gi
		_ = a.loadLetters(catalog.LetterOf(catalog.SortKey(ar.Name)))
		for i, x := range a.artists {
			if x.ID == ar.ID {
				a.artistIdx = i
				a.scr.artists.selectIdx(i)
				_ = a.loadTracks(0)
			}
		}
		for i, x := range a.tracks {
			if x.ID == t.ID {
				a.trackIdx = i
				a.scr.songs.selectIdx(i)
			}
		}
		a.focus = colSongs
		a.renderColumns()
		return
	}
}

func (a *App) genreVisible(name string) bool {
	for _, g := range a.genres {
		if g.Name == name {
			return true
		}
	}
	return false
}
