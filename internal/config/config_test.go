package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExampleConfigLoads(t *testing.T) {
	for _, f := range []string{"../../config/config.example.yaml", "../../config/config.dev.yaml"} {
		cfg, err := Load(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if cfg.Admin.PIN == "" || len(cfg.Keyboard.Bindings) < 16 || cfg.Library.DirName != "master.1000" {
			t.Fatalf("%s: %+v", f, cfg)
		}
	}
}

func TestBindingsMergeAndClamp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte("keyboard:\n  bindings:\n    ok: KEY_ENTER\nplayback:\n  max_volume: 500\n  volume: 400\nadmin:\n  pin: ''\n"), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Keyboard.Bindings["ok"] != "KEY_ENTER" || cfg.Keyboard.Bindings["up"] != "KEY_KP8" {
		t.Fatalf("bindings: %v", cfg.Keyboard.Bindings)
	}
	if cfg.Playback.MaxVolume != 130 || cfg.Playback.Volume != 130 || cfg.Admin.PIN != DefaultPIN {
		t.Fatalf("%+v %q", cfg.Playback, cfg.Admin.PIN)
	}
}

func TestRelPath(t *testing.T) {
	c := Default()
	cases := map[string]string{
		`D:\master.1000\karaokes\A\x.avi`: "karaokes/A/x.avi",
		`d:\MASTER.1000\musica\b.mp3`:     "musica/b.mp3",
		`E:\otro\c.mp3`:                   "E:/otro/c.mp3",
		`D:\master.10000\c.mp3`:           "D:/master.10000/c.mp3",
	}
	for in, want := range cases {
		if got := c.RelPath(in); got != want {
			t.Errorf("RelPath(%q)=%q, quería %q", in, got, want)
		}
	}
}

func TestAnimationsFlag(t *testing.T) {
	if !Default().UI.Animations {
		t.Fatal("las animaciones deben estar encendidas por defecto")
	}
	p := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(p, []byte("ui:\n  animations: false\n"), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Animations {
		t.Fatal("ui.animations: false no se respetó")
	}
}
