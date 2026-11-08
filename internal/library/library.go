// Package library finds the music disk wherever it is mounted and turns
// catalog paths (relative to the library root) into real files.
package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/josegamboa/karaokenino/internal/config"
)

// ErrNotFound means no library root is mounted.
var ErrNotFound = errors.New("no se encontró el disco de música")

// Library resolves catalog paths against the detected root.
type Library struct {
	cfg config.LibraryConfig

	mu    sync.RWMutex
	root  string
	tried map[string]bool // partitions AutoMount already tried
}

func New(cfg config.LibraryConfig) *Library {
	return &Library{cfg: cfg}
}

// Root returns the detected root ("" if none yet).
func (l *Library) Root() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.root
}

// Detect looks for the library once and remembers it. With AutoMount on,
// unmounted partitions are mounted read-only to look inside them.
func (l *Library) Detect() (string, error) {
	root := find(l.cfg)
	if root == "" && l.cfg.AutoMount {
		root = l.AutoMount()
	}
	l.mu.Lock()
	l.root = root
	l.mu.Unlock()
	if root == "" {
		return "", ErrNotFound
	}
	return root, nil
}

// Wait retries Detect every second until the disk shows up, the timeout
// passes or ctx ends. USB disks often mount a few seconds after login.
func (l *Library) Wait(ctx context.Context, timeout time.Duration, onTry func(attempt int)) (string, error) {
	deadline := time.Now().Add(timeout)
	for attempt := 1; ; attempt++ {
		if root, err := l.Detect(); err == nil {
			return root, nil
		}
		if onTry != nil {
			onTry(attempt)
		}
		if time.Now().After(deadline) {
			return "", ErrNotFound
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Abs turns a catalog path into an absolute path on the detected disk.
// Absolute catalog paths are returned unchanged.
func (l *Library) Abs(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	root := l.Root()
	if root == "" {
		return rel
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

// Rel turns an absolute path under the root into a catalog path.
func (l *Library) Rel(abs string) (string, bool) {
	root := l.Root()
	if root == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func find(cfg config.LibraryConfig) string {
	for _, r := range cfg.Roots {
		if isDir(r) && hasEntries(r) {
			return r
		}
	}
	if cfg.DirName == "" {
		return ""
	}
	for _, base := range cfg.SearchIn {
		if found := search(base, cfg.DirName, 3); found != "" {
			return found
		}
	}
	return ""
}

// search walks base up to depth levels looking for a folder named name
// (case-insensitive), e.g. /media/<user>/<LABEL>/master.1000.
func search(base, name string, depth int) string {
	if depth == 0 {
		return ""
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		p := filepath.Join(base, e.Name())
		if strings.EqualFold(e.Name(), name) && isDir(p) && hasEntries(p) {
			return p
		}
	}
	for _, e := range entries {
		if e.IsDir() {
			if found := search(filepath.Join(base, e.Name()), name, depth-1); found != "" {
				return found
			}
		}
	}
	return ""
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// hasEntries skips empty mount points left behind when the disk is unplugged.
func hasEntries(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) > 0
}
