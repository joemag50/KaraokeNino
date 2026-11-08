// Package state keeps what must survive a power cut: the volume and the
// settings changed from the admin menu. The queue is not kept: after a
// restart the rockola opens silent with an empty queue (nobody wants the
// last song of the previous party to start again).
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is saved as JSON. Pointers mean "not changed from config".
// Files from older versions also have "current" and "queue"; they are
// ignored on load and dropped on the next save.
type State struct {
	Volume    int     `json:"volume"`
	MaxVolume *int    `json:"max_volume,omitempty"`
	KidsMode  bool    `json:"kids_mode"`
	BGMusic   *bool   `json:"background_music,omitempty"`
	EventID   int64   `json:"event_id"`
	EventName string  `json:"event_name,omitempty"`
	Welcome   *string `json:"welcome,omitempty"`
	Theme     string  `json:"theme,omitempty"`

	SavedAt time.Time `json:"saved_at"`
}

// Store saves State atomically (write temp file + rename), so a power cut
// mid-write never leaves a broken file.
type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load returns the saved state, or a zero State if there is none or it is unreadable.
func (s *Store) Load() (State, error) {
	var st State
	if s.path == "" {
		return st, nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		// Keep the broken file for inspection and start clean.
		_ = os.Rename(s.path, s.path+".roto")
		return State{}, err
	}
	return st, nil
}

// Save writes st to disk.
func (s *Store) Save(st State) error {
	if s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st.SavedAt = time.Now()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
