package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// audioView is the player screen for music without video: the artist's
// cover big, the song, who comes next and a progress bar. It works like the
// fullscreen video (Reproducción): any key goes to the list (Lista), 7 or 6
// come back, and it comes back by itself after videoBackDelay without keys.
// It is static (no animation), so it costs nothing while the song plays.
type audioView struct {
	a    *App
	lay  *scaledLayout
	root *fyne.Container

	cover            *canvas.Image
	initials         *canvas.Text
	title, artist    *canvas.Text
	next, hint       *canvas.Text
	progress         *progressBar
	folder, coverRel string // the cover shown (for refreshCover)
	name             string
}

// bigCoverPx is the size of the large thumbnails made for this screen.
const bigCoverPx = 720

var audioCoverRect = Rect{440, 56, 400, 400}

func newAudioView(a *App) *audioView {
	v := &audioView{a: a, lay: newScaledLayout(designW, designH)}
	pal := &a.pal
	objs := []fyne.CanvasObject{
		v.lay.placeFull(canvas.NewLinearGradient(pal.Bg1, pal.Bg2, 0)),
	}
	// A soft light of the theme behind the cover.
	glow := canvas.NewImageFromImage(softDisc(pal.Glow[0], 0.55))
	glow.FillMode = canvas.ImageFillStretch
	glow.ScaleMode = canvas.ImageScaleSmooth
	c := audioCoverRect
	objs = append(objs, v.lay.place(glow, Rect{c.Left - 260, c.Top - 200, c.Width + 520, c.Height + 400}))
	objs = append(objs, v.lay.place(newPanel(pal.Panel, pal.PanelBorder, 18), Rect{c.Left - 10, c.Top - 10, c.Width + 20, c.Height + 20}))

	v.initials = newText("", pal.Text, true)
	v.initials.Alignment = fyne.TextAlignCenter
	objs = append(objs, v.lay.placeText(v.initials, Rect{c.Left, c.Top + c.Height/2 - 60, c.Width, 120}, 110))
	v.cover = canvas.NewImageFromResource(nil)
	v.cover.FillMode = canvas.ImageFillContain
	v.cover.ScaleMode = canvas.ImageScaleSmooth
	objs = append(objs, v.lay.place(v.cover, c))

	center := func(t *canvas.Text, r Rect, size float32) {
		t.Alignment = fyne.TextAlignCenter
		objs = append(objs, v.lay.placeText(t, r, size))
	}
	v.title = newText("", pal.Text, true)
	center(v.title, Rect{40, 486, designW - 80, 50}, 40)
	v.artist = newText("", pal.Accent2, true)
	center(v.artist, Rect{40, 540, designW - 80, 34}, 26)
	v.next = newText("", pal.TextDim, true)
	center(v.next, Rect{40, 590, designW - 80, 30}, 22)
	v.hint = newText("7  ver la lista y pedir otra canción  ·  + − volumen  ·  * pausa  ·  . .  saltar", pal.TextDim, false)
	center(v.hint, Rect{40, 640, designW - 80, 26}, 18)

	v.progress = newProgressBar(v.lay, pal, Rect{140, 686, designW - 280, 8})
	objs = append(objs, v.progress.bg, v.progress.fill)

	v.root = container.New(v.lay, objs...)
	v.root.Hide()
	return v
}

// start shows a new song: its title, artist and cover.
func (v *audioView) start(it Song) {
	v.title.Text = fit(it.Title, designW-80, 40)
	v.artist.Text = fit(it.Artist, designW-80, 26)
	v.next.Text = ""
	v.folder, v.coverRel, v.name = it.Folder, it.Cover, it.Artist
	v.progress.set(0)
	v.title.Refresh()
	v.artist.Refresh()
	v.next.Refresh()
	v.refreshCover()
}

// refreshCover shows the large thumbnail (made in the background the first
// time; the small one meanwhile), or the artist's initials.
func (v *audioView) refreshCover() {
	file := v.a.bigCoverFor(v.folder, v.coverRel)
	if file == "" {
		file = v.a.coverFor(v.folder, v.coverRel)
	}
	v.cover.Hidden = file == ""
	if v.cover.File != file {
		v.cover.File = file
		v.cover.Resource = nil
		v.cover.Refresh() // reload: canvas.Refresh would keep the old picture
	}
	v.initials.Text = ""
	if file == "" && v.name != "" {
		v.initials.Text = initials(v.name)
	}
	v.initials.Refresh()
}

func (v *audioView) show(on bool) {
	if on == v.root.Visible() {
		return
	}
	if on {
		v.root.Show()
	} else {
		v.root.Hide()
	}
}

func (v *audioView) setNext(text string) {
	if v.next.Text != text {
		v.next.Text = text
		v.next.Refresh()
	}
}

// Song is what the audio screen needs from a queue item.
type Song struct{ Title, Artist, Folder, Cover string }
