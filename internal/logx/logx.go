// Package logx sends the standard logger to a size-rotated file, so the
// rockola can run for months without filling the disk.
package logx

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rotating is an io.Writer that rotates file at maxBytes, keeping keep old copies
// (file.1 is the newest).
type Rotating struct {
	path     string
	maxBytes int64
	keep     int

	mu   sync.Mutex
	f    *os.File
	size int64
}

func NewRotating(path string, maxBytes int64, keep int) (*Rotating, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &Rotating{path: path, maxBytes: maxBytes, keep: keep}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Rotating) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *Rotating) rotate() error {
	r.f.Close()
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	if r.keep > 0 {
		_ = os.Rename(r.path, r.path+".1")
	} else {
		_ = os.Remove(r.path)
	}
	return r.open()
}

func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

// Setup logs to path (5 MB × 3 files) and stderr. Empty path = stderr only.
//
// When stderr is itself a file (the kiosk loop appends it to kiosk.log),
// lines go only to the rotating log: copying them there too made kiosk.log
// grow without limit. Crashes still reach stderr (and crash.log).
func Setup(path string) (io.Closer, error) {
	if path == "" {
		return io.NopCloser(nil), nil
	}
	r, err := NewRotating(path, 5<<20, 3)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stderr.Stat(); err == nil && st.Mode().IsRegular() {
		log.SetOutput(r)
	} else {
		log.SetOutput(io.MultiWriter(os.Stderr, r))
	}
	log.SetFlags(log.LstdFlags)
	return r, nil
}

// CapFile cuts a log that only ever grows (crash.log, kiosk.log) to its last
// maxBytes/2 bytes once it is bigger than maxBytes. Programs appending to it
// keep working (they append at the new end). It returns the bytes freed.
func CapFile(path string, maxBytes int64) (int64, error) {
	st, err := os.Stat(path)
	if err != nil || st.Size() <= maxBytes {
		if os.IsNotExist(err) {
			err = nil
		}
		return 0, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	keep := make([]byte, maxBytes/2)
	n, err := f.ReadAt(keep, st.Size()-int64(len(keep)))
	if err != nil && err != io.EOF {
		return 0, err
	}
	keep = keep[:n]
	if i := bytes.IndexByte(keep, '\n'); i >= 0 {
		keep = keep[i+1:] // start at a whole line
	}
	if err := f.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := f.WriteAt(keep, 0); err != nil {
		return 0, err
	}
	return st.Size() - int64(len(keep)), nil
}

// Tidy keeps a log folder small: rotated copies ("x.log.2") older than
// maxAge are deleted and any log bigger than maxBytes is cut with CapFile.
// It returns how many files were deleted or cut and the bytes freed.
func Tidy(dir string, maxAge time.Duration, maxBytes int64, dryRun bool) (n int, freed int64, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		switch {
		case isRotated(e.Name()) && time.Since(info.ModTime()) > maxAge:
			if !dryRun && os.Remove(p) != nil {
				continue
			}
			n++
			freed += info.Size()
		case strings.HasSuffix(e.Name(), ".log") && info.Size() > maxBytes:
			cut := info.Size() - maxBytes/2
			if !dryRun {
				if cut, err = CapFile(p, maxBytes); err != nil {
					continue
				}
			}
			n++
			freed += cut
		}
	}
	return n, freed, nil
}

// isRotated reports names like "karaokenino.log.3".
func isRotated(name string) bool {
	i := strings.LastIndex(name, ".log.")
	if i < 0 {
		return false
	}
	_, err := strconv.Atoi(name[i+len(".log."):])
	return err == nil
}

// Tail returns the last n lines of the log file (for the diagnostics screen).
func Tail(path string, n int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(b) > 64<<10 {
		b = b[len(b)-64<<10:]
	}
	var lines []string
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				lines = append(lines, string(b[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, string(b[start:]))
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
