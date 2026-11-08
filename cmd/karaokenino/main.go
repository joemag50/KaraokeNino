package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/library"
	"github.com/josegamboa/karaokenino/internal/logx"
	"github.com/josegamboa/karaokenino/internal/player"
	"github.com/josegamboa/karaokenino/internal/state"
	"github.com/josegamboa/karaokenino/internal/ui"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	cfgPath := flag.String("config", "config/config.yaml", "archivo de configuración")
	showVersion := flag.Bool("version", false, "mostrar versión y salir")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	closer, err := logx.Setup(cfg.LogFile)
	if err != nil {
		log.Printf("log: %v (solo pantalla)", err)
	} else {
		defer closer.Close()
	}
	// If the program ever crashes, keep the full report next to the log.
	if cfg.LogFile != "" {
		crash := filepath.Join(filepath.Dir(cfg.LogFile), "crash.log")
		// It gets a line per start and a full report per crash: a crash
		// loop in the kiosk would otherwise fill the disk.
		_, _ = logx.CapFile(crash, 1<<20)
		if f, err := os.OpenFile(crash, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			fmt.Fprintf(f, "\n=== KaraokeNino %s iniciado %s ===\n", version, time.Now().Format(time.RFC3339))
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
		}
	}
	ui.Version = version
	log.Printf("KaraokeNino %s — tema %s", version, cfg.Theme)

	mpv := player.New(player.MPVOptions{
		Path:           cfg.MpvPath,
		Fullscreen:     cfg.Fullscreen,
		NormalizeAudio: cfg.Playback.NormalizeAudio,
		Demo:           cfg.DemoMode,
		Volume:         cfg.Playback.Volume,
		MaxVolume:      cfg.Playback.MaxVolume,
		ExtraArgs:      cfg.MpvArgs,
	})
	if !cfg.DemoMode {
		player.KillLeftovers() // a restart starts silent
	}
	if cfg.DemoMode {
		log.Println("demo_mode=true: sin disco ni mpv (cada canción «suena» 10 s)")
	} else if err := mpv.Available(); err != nil {
		log.Printf("aviso: %v", err)
	}

	store, err := catalog.Open(cfg.CatalogDB)
	if err != nil {
		log.Fatalf("catálogo: %v", err)
	}
	defer store.Close()

	err = ui.Run(ui.Deps{
		Cfg:     cfg,
		Store:   store,
		MPV:     mpv,
		Library: library.New(cfg.Library),
		State:   state.NewStore(cfg.StateFile),
		Exit: func(code int) {
			store.Close()
			os.Exit(code)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
