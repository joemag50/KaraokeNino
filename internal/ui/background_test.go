package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func newThemedTestApp(t testing.TB, theme string) (*App, fyne.Window) {
	a := newTestApp(t)
	fyne.CurrentApp().Settings().SetTheme(newPartyTheme())
	a.pal = PaletteFor(theme)
	w := test.NewTempWindow(t, a.build())
	a.window = w
	w.Resize(fyne.NewSize(1280, 720))
	return a, w
}

// The main screen animates only while guests see it, and a frame moves the
// lights, pulses the selected row and runs the queue sparkle.
func TestPartyAnimation(t *testing.T) {
	a, _ := newThemedTestApp(t, "xv")
	startToBrowse(t, a)
	press(a, fyne.KeyRight, fyne.KeyRight) // songs column
	if !a.browseFXVisible() {
		t.Fatal("la pantalla principal debe animarse")
	}
	s := a.scr
	t0 := s.bg.t0
	glow := s.bg.glows[0].img
	s.animate(t0.Add(3 * time.Second))
	p1 := glow.Position()
	s.animate(t0.Add(9 * time.Second))
	if glow.Position() == p1 {
		t.Fatal("las luces del fondo no se mueven")
	}
	if s.songs.halo.Hidden {
		t.Fatal("la canción elegida debe brillar")
	}
	c1 := s.songs.halo.FillColor
	s.animate(t0.Add(9*time.Second + 800*time.Millisecond))
	if s.songs.halo.FillColor == c1 {
		t.Fatal("el brillo de la fila elegida debe latir")
	}

	// Queue a song: the sparks fly and then disappear.
	now := time.Now()
	s.songs.celebrate(now, true)
	s.songs.animate(now.Add(200*time.Millisecond), 0)
	if s.songs.sparks[0].Hidden {
		t.Fatal("faltan las chispas al agregar a la cola")
	}
	s.songs.animate(now.Add(sparkDuration+time.Millisecond), 0)
	if !s.songs.sparks[0].Hidden {
		t.Fatal("las chispas deben terminar")
	}

	// Keypad pop: the key grows and settles back.
	key := s.keypad.keys["8"]
	s.keypad.flash("8")
	s.keypad.animate(key.popAt.Add(keyPop / 2))
	if key.bg.Size().Width <= s.lay.size(key.r).Width {
		t.Fatal("la tecla presionada debe crecer")
	}
	s.keypad.animate(key.popAt.Add(keyPop + time.Millisecond))
	if key.bg.Size() != s.lay.size(key.r) {
		t.Fatal("la tecla debe volver a su tamaño")
	}

	// Covered by the idle screen or a video: no animation.
	a.idle.show()
	if a.browseFXVisible() {
		t.Fatal("con la pantalla de espera el fondo principal se detiene")
	}
	a.idle.hide()
}

// TestThemeSnapshots draws each theme to PNG (KN_SNAPSHOTS=dir) to look at
// the design without a TV.
func TestThemeSnapshots(t *testing.T) {
	for _, name := range ThemeNames() {
		a, w := newThemedTestApp(t, name)
		a.start()
		snapshot(t, w, name+"-0-modos")
		press(a, fyne.KeyEnter, fyne.KeyDown, fyne.KeyDown, fyne.KeyRight, fyne.KeyDown, fyne.KeyRight, fyne.KeyDown)
		s := a.scr
		s.animate(s.bg.t0.Add(6 * time.Second))
		snapshot(t, w, name+"-1-principal")
		now := time.Now()
		s.songs.celebrate(now, true)
		s.keypad.flash("enter")
		s.keypad.animate(now.Add(keyPop / 2))
		s.songs.animate(now.Add(250*time.Millisecond), 0)
		snapshot(t, w, name+"-2-agregar")
		a.idle.show()
		a.idle.bg.frame(a.idle.bg.t0.Add(20 * time.Second))
		snapshot(t, w, name+"-3-espera")
		a.idle.hide()
		press(a, "1", "6", fyne.KeyEnter, fyne.KeyEnter, fyne.KeyEnter, fyne.KeyEnter) // MÚSICA → play
		snapshot(t, w, name+"-4-musica")
	}
}

// BenchmarkPartyFrame measures the CPU work of one animation frame of the
// main screen on our side (moving ~40 objects and recoloring them). The GPU
// then composites the window once per frame, at fxFPS.
func BenchmarkPartyFrame(b *testing.B) {
	a, _ := newThemedTestApp(b, "xv")
	a.start()
	press(a, fyne.KeyEnter)
	s := a.scr
	now := s.bg.t0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		now = now.Add(time.Second / fxFPS)
		s.animate(now)
	}
}
