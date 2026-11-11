package ui

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/logx"
	"github.com/josegamboa/karaokenino/internal/state"
)

// ---------- PIN ----------

func (a *App) openPIN() {
	if wait := time.Until(a.pinLock); wait > 0 {
		a.flash(fmt.Sprintf("Demasiados intentos: espera %d s", int(wait.Seconds())+1))
		return
	}
	d := newDialog(a, "Administrador", 560, 300)
	pal := &a.pal
	d.keys("Escribe el PIN y presiona "+key("Intro"), Rect{Left: d.inner.Left, Top: d.inner.Top + 2, Width: d.inner.Width, Height: 32}, 22, pal.TextDim)
	dots := d.text("", Rect{Left: d.inner.Left, Top: d.inner.Top + 50, Width: d.inner.Width, Height: 70}, 56, pal.Accent2, true)
	msg := d.text("", Rect{Left: d.inner.Left, Top: d.inner.Top + 130, Width: d.inner.Width, Height: 26}, 20, pal.Accent, true)
	d.setHint(key(".") + " borrar  ·  " + key("/") + " cerrar")
	pin := ""
	tries := 0
	o := &overlay{root: d.container()}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action):
			a.closeOverlay()
			return
		case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
			if len(pin) < 12 {
				pin += string(name)
			}
		case name == fyne.KeyPeriod || action == "backspace" || action == "next":
			if pin != "" {
				pin = pin[:len(pin)-1]
			}
		case action == "ok":
			if pin == a.Cfg.Admin.PIN {
				a.pinFails = 0
				log.Printf("admin: acceso")
				a.openAdmin()
				return
			}
			tries++
			pin = ""
			msg.Text = "PIN incorrecto"
			msg.Refresh()
			log.Printf("admin: PIN incorrecto (%d)", tries)
			if tries >= 3 {
				a.pinFails++
				// Guessing gets slower each round: 1, 2, 4… minutes.
				a.pinLock = time.Now().Add(time.Minute << min(a.pinFails-1, 4))
				a.closeOverlay()
				return
			}
		}
		dots.Text = strings.Repeat("● ", len(pin))
		dots.Refresh()
	}
	a.showOverlay(o)
}

// ---------- menu ----------

type adminItem struct {
	label func() string
	ok    func()          // Intro
	adj   func(delta int) // 4/6
}

func (a *App) openAdmin() {
	d := newDialog(a, "Menú de administrador", 920, 640)
	l := d.list(d.inner, 42, 22, 20)
	d.setHint(key("8") + key("2") + " elegir  ·  " + key("4") + key("6") + " cambiar  ·  " + key("Intro") + " aceptar  ·  " + key("/") + " cerrar")

	yesNo := func(b bool) string {
		if b {
			return "Sí"
		}
		return "No"
	}
	var items []adminItem
	var render func()
	var o *overlay
	confirm := map[int]time.Time{}
	needConfirm := func(i int, fn func()) func() {
		return func() {
			if t, ok := confirm[i]; ok && time.Since(t) < 4*time.Second {
				fn()
				return
			}
			confirm[i] = time.Now()
			a.flash("Presiona " + key("Intro") + " otra vez para confirmar")
			d.setHint("Presiona " + key("Intro") + " otra vez para confirmar")
		}
	}
	items = []adminItem{
		{label: func() string { return fmt.Sprintf("Volumen máximo: %d", a.MPV.MaxVolume()) },
			adj: func(dl int) {
				v := a.MPV.SetMaxVolume(a.MPV.MaxVolume() + dl*5)
				a.updateState(func(s *state.State) { s.MaxVolume = &v })
			}},
		{label: func() string { return fmt.Sprintf("Volumen actual: %d", a.MPV.Volume()) },
			adj: func(dl int) { a.MPV.ChangeVolume(dl); a.saveState() }},
		{label: func() string {
			_, q := a.box.Snapshot()
			return fmt.Sprintf("Administrar cola (%d canciones)…", len(q))
		},
			ok: a.openAdminQueue},
		{label: func() string { return "Saltar canción actual" }, ok: func() { a.box.Next() }},
		{label: func() string { return "Vaciar la cola" }, ok: needConfirm(4, func() { a.box.Clear(); a.flash("Cola vacía") })},
		{label: func() string { return "Modo infantil: " + yesNo(a.kidsMode()) },
			ok: func() { a.setKidsMode(!a.kidsMode()) }, adj: func(int) { a.setKidsMode(!a.kidsMode()) }},
		{label: func() string { return "Música de fondo: " + yesNo(a.bgMusic()) },
			ok: func() { a.setBGMusic(!a.bgMusic()) }, adj: func(int) { a.setBGMusic(!a.bgMusic()) }},
		{label: func() string { return "Tema de colores: " + a.pal.Name + " (se aplica al reiniciar)" },
			adj: func(dl int) { a.cycleTheme(dl) }, ok: func() { a.cycleTheme(1) }},
		{label: func() string { return "Mensaje de bienvenida: " + orDash(a.welcomeText()) + "…" },
			ok: a.openWelcomeEditor},
		{label: func() string { return "Nuevo evento (reinicia «lo más pedido»): " + orDash(a.eventName()) },
			ok: needConfirm(9, a.newEvent)},
		{label: func() string {
			if a.scanning {
				return "Re-escanear disco (escaneando…)"
			}
			return "Re-escanear disco (canciones nuevas)"
		}, ok: func() {
			if a.rescan() {
				a.flash("Revisando el disco en segundo plano…")
			} else {
				a.flash("No hay disco de música conectado")
			}
		}},
		{label: func() string { return "Diagnóstico…" }, ok: a.openDiagnostics},
		{label: func() string { return "Reiniciar programa" }, ok: needConfirm(12, func() { a.exit(ExitRestart) })},
		{label: func() string { return "Reiniciar computadora" }, ok: needConfirm(13, func() { a.runPower(a.Cfg.Admin.RebootCommand) })},
		{label: func() string { return "Apagar rockola" }, ok: needConfirm(14, func() { a.runPower(a.Cfg.Admin.ShutdownCommand) })},
		{label: func() string { return "Salir al escritorio" }, ok: needConfirm(15, func() { a.exit(ExitDesktop) })},
		{label: func() string { return "Cerrar menú" }, ok: a.closeOverlay},
	}
	render = func() {
		rows := make([]rowItem, len(items))
		for i, it := range items {
			rows[i] = rowItem{Text: it.label()}
		}
		sel := l.sel
		l.set(rows, sel)
	}
	o = &overlay{name: "admin", root: d.container(), onQueue: render}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action):
			a.closeOverlay()
			return
		case action == "up":
			l.selectIdx(l.sel - 1)
			return
		case action == "down":
			l.selectIdx(l.sel + 1)
			return
		case action == "left" || action == "volume_down":
			if f := items[l.sel].adj; f != nil {
				f(-1)
			}
		case action == "right" || action == "volume_up":
			if f := items[l.sel].adj; f != nil {
				f(1)
			}
		case action == "ok":
			if f := items[l.sel].ok; f != nil {
				f()
			}
			if a.ov != o {
				return // the item opened another screen or closed the menu
			}
		}
		render()
	}
	render()
	a.showOverlay(o)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func (a *App) exit(code int) {
	log.Printf("salida solicitada (código %d)", code)
	a.saveState()
	_ = a.MPV.Stop()
	a.Exit(code)
}

func (a *App) runPower(cmd []string) {
	if len(cmd) == 0 {
		a.flash("No hay comando configurado")
		return
	}
	a.saveState()
	_ = a.MPV.Stop()
	log.Printf("admin: %v", cmd)
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		a.flash(fmt.Sprintf("No se pudo: %v %s", err, strings.TrimSpace(string(out))))
		log.Printf("admin: %v: %v %s", cmd, err, out)
	}
}

func (a *App) setKidsMode(on bool) {
	a.updateState(func(s *state.State) { s.KidsMode = on })
	if err := a.loadGenres(); err != nil {
		a.flash(err.Error())
	}
	a.box.SetOptions(a.jukeboxOptions())
}

func (a *App) setBGMusic(on bool) {
	a.updateState(func(s *state.State) { s.BGMusic = &on })
	a.box.SetOptions(a.jukeboxOptions())
	if !on {
		if cur, _ := a.box.Snapshot(); cur != nil && cur.Filler {
			a.box.Next()
		}
	}
}

func (a *App) cycleTheme(d int) {
	names := ThemeNames()
	i := 0
	for k, n := range names {
		if n == a.pal.Name {
			i = k
		}
	}
	next := names[((i+d)%len(names)+len(names))%len(names)]
	a.pal.Name = next // shown in the menu; colors apply after restart
	a.updateState(func(s *state.State) { s.Theme = next })
}

func (a *App) welcomeText() string {
	a.stMu.Lock()
	defer a.stMu.Unlock()
	if a.st.Welcome != nil {
		return *a.st.Welcome
	}
	return a.Cfg.Event.Welcome
}

func (a *App) eventName() string {
	a.stMu.Lock()
	defer a.stMu.Unlock()
	return a.st.EventName
}

func (a *App) newEvent() {
	name := "Evento " + time.Now().Format("02/01/2006 15:04")
	id, err := a.Store.StartEvent(a.ctx, name)
	if err != nil {
		a.flash(err.Error())
		return
	}
	a.updateState(func(s *state.State) { s.EventID, s.EventName = id, name })
	a.flash("Nuevo evento: " + name)
}

// ---------- admin queue ----------

func (a *App) openAdminQueue() {
	d := newDialog(a, "Administrar cola", 920, 600)
	l := d.list(d.inner, 44, 22, 60)
	l.empty = "La cola está vacía"
	d.setHint(key("8") + key("2") + " elegir  ·  " + key("4") + key("6") + " mover  ·  " + key("Intro") + " o " + key(".") + " quitar  ·  " + key("/") + " volver")
	render := func() {
		_, q := a.box.Snapshot()
		items := make([]rowItem, len(q))
		for i, it := range q {
			items[i] = rowItem{Num: fmt.Sprint(i + 1), Text: it.Title, Sub: it.Artist}
		}
		sel := min(l.sel, max(len(items)-1, 0))
		l.set(items, sel)
	}
	o := &overlay{root: d.container(), onQueue: render}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action):
			a.openAdmin()
		case action == "up":
			l.selectIdx(l.sel - 1)
		case action == "down":
			l.selectIdx(l.sel + 1)
		case action == "left":
			l.sel = a.box.Move(l.sel, -1)
			render()
		case action == "right":
			l.sel = a.box.Move(l.sel, 1)
			render()
		case action == "ok" || action == "next":
			a.box.Remove(l.sel)
		}
	}
	render()
	a.showOverlay(o)
}

// ---------- welcome editor ----------

func (a *App) openWelcomeEditor() {
	d := newDialog(a, "Mensaje de bienvenida", 900, 300)
	pal := &a.pal
	field := Rect{Left: d.inner.Left, Top: d.inner.Top + 30, Width: d.inner.Width, Height: 56}
	d.text("Ejemplo: ¡Felices 15, Ana! (necesita teclado completo)", Rect{Left: d.inner.Left, Top: d.inner.Top, Width: d.inner.Width, Height: 24}, 18, pal.TextDim, false)
	d.add(d.lay.place(newPanel(withAlpha(pal.Text, 20), pal.Accent2, 10), field))
	t := d.text("", Rect{Left: field.Left + 16, Top: field.Top + 12, Width: field.Width - 32, Height: 34}, 28, pal.Text, true)
	d.setHint("Escribe  ·  " + key("Intro") + " guardar  ·  " + key("Esc") + " cancelar  ·  " + key("Intro") + " vacío = quitar mensaje")
	text := a.welcomeText()
	show := func() { t.Text = text + "_"; t.Refresh() }
	o := &overlay{root: d.container()}
	o.rune = func(r rune) {
		if len([]rune(text)) < 60 {
			text += string(r)
			show()
		}
	}
	o.key = func(name fyne.KeyName, action string) {
		switch name {
		case fyne.KeyEscape:
			a.openAdmin()
		case fyne.KeyBackspace:
			if r := []rune(text); len(r) > 0 {
				text = string(r[:len(r)-1])
				show()
			}
		case fyne.KeyReturn, fyne.KeyEnter:
			v := strings.TrimSpace(text)
			a.updateState(func(s *state.State) { s.Welcome = &v })
			a.openAdmin()
		}
	}
	show()
	a.showOverlay(o)
}

// ---------- diagnostics ----------

func (a *App) openDiagnostics() {
	d := newDialog(a, "Diagnóstico", 1100, 660)
	l := d.list(d.inner, 30, 16, 10)
	d.setHint(key("8") + key("2") + " desplazar  ·  " + key("/") + " volver")
	var lines []string
	add := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }

	root := a.Library.Root()
	if root == "" {
		add("⚠ Disco de música: NO ENCONTRADO (busca en %v y %s)", a.Cfg.Library.Roots, a.Cfg.Library.DirName)
	} else {
		free := diskFree(root)
		add("Disco de música: %s  ·  libre: %s", root, free)
	}
	if st, err := a.Store.Stats(a.ctx); err == nil {
		add("Catálogo: %d géneros · %d artistas · %d canciones · %d karaoke", st.Genres, st.Artists, st.Tracks, st.Karaoke)
		add("Faltantes (ocultas): %d  ·  Duplicadas: %d grupos  ·  Reproducciones: %d", st.Missing, st.DupGroups, st.Plays)
	}
	add("Último escaneo: %s  ·  Importado de: %s", orDash(a.Store.Meta(a.ctx, "last_scan")), orDash(a.Store.Meta(a.ctx, "import_source")))
	add("Volumen: %d / máx %d  ·  Modo infantil: %v  ·  Música de fondo: %v", a.MPV.Volume(), a.MPV.MaxVolume(), a.kidsMode(), a.bgMusic())
	switch {
	case a.Cfg.DemoMode:
		add("Reproductor: modo demo")
	case a.MPV.Embedded():
		add("Reproductor: integrado (libmpv) — el video se dibuja dentro de KaraokeNino")
	default:
		if err := a.MPV.Available(); err != nil {
			add("⚠ %v", err)
		}
		add("⚠ Reproductor: mpv aparte (instala libmpv2 para el video integrado)")
	}
	if a.Cfg.Admin.PIN == config.DefaultPIN {
		add("⚠ El PIN es el de fábrica (%s): cámbialo en config.yaml (admin.pin)", config.DefaultPIN)
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	add("Memoria del programa: %d MB  ·  Go %s", m.Sys>>20, runtime.Version())
	if dups, err := a.Store.Duplicates(a.ctx, 5); err == nil && len(dups) > 0 {
		add("Ejemplos de duplicadas:")
		for _, g := range dups {
			add("   %s — %s (%d copias, números %s)", g[0].Title, g[0].Artist, len(g), trackNums(g))
		}
	}
	add("Últimas líneas del registro (%s):", a.Cfg.LogFile)
	for _, ln := range logx.Tail(a.Cfg.LogFile, 12) {
		add("   %s", ln)
	}
	items := make([]rowItem, len(lines))
	for i, s := range lines {
		items[i] = rowItem{Text: s}
	}
	l.set(items, 0)
	o := &overlay{root: d.container()}
	o.key = func(name fyne.KeyName, action string) {
		switch {
		case isClose(name, action) || action == "ok":
			a.openAdmin()
		case action == "up":
			l.selectIdx(l.sel - 1)
		case action == "down":
			l.selectIdx(l.sel + 1)
		}
	}
	a.showOverlay(o)
}

func trackNums(ts []catalog.Track) string {
	nums := make([]string, len(ts))
	for i, t := range ts {
		nums[i] = fmt.Sprint(t.ID)
	}
	return strings.Join(nums, ", ")
}
