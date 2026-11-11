package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Fullscreen bool `yaml:"fullscreen"`

	// Theme picks a color palette: moderno, boda, infantil, xv.
	Theme string `yaml:"theme"`

	// SQLite catalog (built from Musica.data and disk scans).
	CatalogDB string `yaml:"catalog_db"`
	// StateFile keeps the queue, volume and admin settings across restarts.
	StateFile string `yaml:"state_file"`
	// LogFile is rotated automatically; empty = stderr only.
	LogFile string `yaml:"log_file"`

	Library  LibraryConfig  `yaml:"library"`
	Playback PlaybackConfig `yaml:"playback"`
	Admin    AdminConfig    `yaml:"admin"`
	Event    EventConfig    `yaml:"event"`
	Business BusinessConfig `yaml:"business"`
	Idle     IdleConfig     `yaml:"idle"`
	UI       UIConfig       `yaml:"ui"`

	MpvPath string `yaml:"mpv_path"`
	// MpvArgs are extra mpv options (e.g. "--hwdec=auto", "--audio-device=alsa/hdmi").
	MpvArgs []string `yaml:"mpv_args"`

	// DemoMode: browse catalog without media files or mpv (development PC).
	DemoMode bool `yaml:"demo_mode"`

	Keyboard KeyboardConfig `yaml:"keyboard"`
}

// LibraryConfig locates the music disk. The catalog stores paths relative
// to the library root, so the disk can be mounted anywhere.
type LibraryConfig struct {
	// Roots are tried first, in order (e.g. /media/usb/master.1000).
	Roots []string `yaml:"roots"`
	// DirName is the library folder searched for under SearchIn mounts.
	DirName string `yaml:"dir_name"`
	// SearchIn lists mount parents scanned up to 3 levels deep.
	SearchIn []string `yaml:"search_in"`
	// WaitSeconds: how long to wait at startup for the disk to be mounted.
	WaitSeconds int `yaml:"wait_seconds"`
	// WindowsRoot is the Windows prefix of the paths in Musica.data
	// (stripped on import).
	WindowsRoot string `yaml:"windows_root"`
	// AutoMount mounts unmounted partitions read-only under /mnt to find the
	// library (Puppy Linux mounts nothing at boot). Needs root or udisks.
	AutoMount bool `yaml:"auto_mount"`
	// ScanOnStart scans the disk at every start. Off by default: scan from
	// the admin menu after copying songs (an empty catalog is always scanned).
	ScanOnStart bool `yaml:"scan_on_start"`
}

type PlaybackConfig struct {
	// Volume at first start (0-100). The last used volume is restored after that.
	Volume int `yaml:"volume"`
	// MaxVolume caps the volume guests can reach (admin can change it).
	MaxVolume int `yaml:"max_volume"`
	// NormalizeAudio evens out loudness between old and new recordings.
	NormalizeAudio bool `yaml:"normalize_audio"`
	// MaxQueue limits pending songs (0 = no limit).
	MaxQueue int `yaml:"max_queue"`
	// MaxPerArtistInRow stops the same artist from being queued more than N times in a row (0 = off).
	MaxPerArtistInRow int `yaml:"max_per_artist_in_row"`
	// BackgroundMusic plays random songs when the queue is empty.
	BackgroundMusic BackgroundMusicConfig `yaml:"background_music"`
	// EmbeddedVideo plays videos inside the KaraokeNino screen through
	// libmpv (package libmpv2); false or missing libmpv = mpv's own window.
	EmbeddedVideo bool `yaml:"embedded_video"`
	// NextUpSeconds: how long the "Siguiente: ..." notice stays on the video.
	NextUpSeconds int `yaml:"next_up_seconds"`
}

type BackgroundMusicConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Genres       []string `yaml:"genres"`
	AfterSeconds int      `yaml:"after_seconds"`
}

type AdminConfig struct {
	// PIN opens the admin menu. Required: change it before renting.
	PIN string `yaml:"pin"`
	// OpenSequence: key (or keys pressed quickly in a row) that asks for the PIN.
	// Default Esc; when a window is open, Esc closes it first.
	OpenSequence []string `yaml:"open_sequence"`
	// KidsGenres are the only genres shown in kids mode.
	KidsGenres []string `yaml:"kids_genres"`
	// HiddenGenres are never shown to guests.
	HiddenGenres []string `yaml:"hidden_genres"`
	// ShutdownCommand / RebootCommand run from the admin menu.
	ShutdownCommand []string `yaml:"shutdown_command"`
	RebootCommand   []string `yaml:"reboot_command"`
}

type EventConfig struct {
	// Welcome is shown on the idle screen, e.g. "¡Felices 15, Ana!".
	Welcome string `yaml:"welcome"`
}

type BusinessConfig struct {
	Name    string `yaml:"name"`
	Contact string `yaml:"contact"`
	// Logo image shown on the idle screen (path; optional).
	Logo string `yaml:"logo"`
}

type IdleConfig struct {
	// AfterSeconds without keys before the idle/attract screen (0 = off).
	AfterSeconds int `yaml:"after_seconds"`
	// RotateSeconds between idle slides.
	RotateSeconds int `yaml:"rotate_seconds"`
}

type UIConfig struct {
	// Animations: moving party background, glow on the selected row, key
	// "pop" and the sparkle when a song is queued. They pause by themselves
	// while a video plays; false turns them off (very slow PCs).
	Animations bool `yaml:"animations"`
}

type KeyboardConfig struct {
	// Logical action -> evdev KEY_* name.
	Bindings map[string]string `yaml:"bindings"`
}

func Default() Config {
	return Config{
		Fullscreen: true,
		CatalogDB:  "data/catalog.db",
		StateFile:  "data/state.json",
		LogFile:    "data/logs/karaokenino.log",
		MpvPath:    "mpv",
		Library: LibraryConfig{
			Roots:       []string{"/media/usb/master.1000"},
			DirName:     "master.1000",
			SearchIn:    []string{"/media", "/mnt", "/run/media"},
			WaitSeconds: 60,
			WindowsRoot: `D:\master.1000`,
			ScanOnStart: false,
			AutoMount:   true,
		},
		Playback: PlaybackConfig{
			Volume:            80,
			MaxVolume:         100,
			NormalizeAudio:    true,
			MaxQueue:          50,
			MaxPerArtistInRow: 0,
			NextUpSeconds:     8,
			EmbeddedVideo:     true,
			BackgroundMusic: BackgroundMusicConfig{
				Enabled:      false,
				Genres:       []string{"MUSICA"},
				AfterSeconds: 30,
			},
		},
		Admin: AdminConfig{
			PIN:             DefaultPIN,
			OpenSequence:    []string{"KEY_ESC"},
			KidsGenres:      []string{"z INFANTILES"},
			ShutdownCommand: []string{"systemctl", "poweroff"},
			RebootCommand:   []string{"systemctl", "reboot"},
		},
		Idle: IdleConfig{AfterSeconds: 90, RotateSeconds: 8},
		UI:   UIConfig{Animations: true},
		Keyboard: KeyboardConfig{
			Bindings: map[string]string{
				"up":          "KEY_KP8",
				"down":        "KEY_KP2",
				"left":        "KEY_KP4",
				"right":       "KEY_KP6",
				"ok":          "KEY_KPENTER",
				"next":        "KEY_KPDOT",
				"volume_up":   "KEY_KPPLUS",
				"volume_down": "KEY_KPMINUS",
				"pause":       "KEY_KPASTERISK",
				"playlist":    "KEY_KPSLASH",
				"abc":         "KEY_KP0",
				"reset":       "KEY_KP1",
				"top":         "KEY_KP7",
				"page_up":     "KEY_KP9",
				"page_down":   "KEY_KP3",
			},
		},
	}
}

// Load reads path over the defaults. A missing file returns the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	// Bindings merge key by key instead of replacing the whole map.
	defaults := cfg.Keyboard.Bindings
	cfg.Keyboard.Bindings = nil
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	for k, v := range defaults {
		if _, ok := cfg.Keyboard.Bindings[k]; !ok {
			if cfg.Keyboard.Bindings == nil {
				cfg.Keyboard.Bindings = map[string]string{}
			}
			cfg.Keyboard.Bindings[k] = v
		}
	}
	if strings.TrimSpace(cfg.Admin.PIN) == "" {
		cfg.Admin.PIN = DefaultPIN
	}
	cfg.Playback.MaxVolume = clamp(cfg.Playback.MaxVolume, 10, 130)
	cfg.Playback.Volume = clamp(cfg.Playback.Volume, 0, cfg.Playback.MaxVolume)
	return cfg, nil
}

// RelPath converts a Windows path from Musica.data into a path relative to the
// library root ("karaokes/Juan Gabriel/Amor Eterno.avi"). Paths outside
// WindowsRoot are returned with forward slashes, unchanged otherwise.
func (c Config) RelPath(winPath string) string {
	p := strings.ReplaceAll(winPath, `\`, `/`)
	root := strings.TrimRight(strings.ReplaceAll(c.Library.WindowsRoot, `\`, `/`), "/")
	if root != "" && len(p) > len(root) && strings.EqualFold(p[:len(root)], root) && p[len(root)] == '/' {
		return p[len(root)+1:]
	}
	return p
}

// IdleDuration is the idle screen delay (0 = disabled).
func (c Config) IdleDuration() time.Duration {
	return time.Duration(c.Idle.AfterSeconds) * time.Second
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

// DefaultPIN is used when the config has no admin.pin. The diagnostics screen
// warns while it is still in use.
const DefaultPIN = "1234"
