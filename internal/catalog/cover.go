package catalog

import (
	"bufio"
	"image"
	_ "image/gif" // covers are recognized by their contents, not their name
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/josegamboa/karaokenino/internal/library"
)

// Covers finds artist cover images and remembers the answer, so paging
// through artists does not hit a slow USB disk again and again.
type Covers struct {
	abs      func(rel string) string
	fallback string

	mu    sync.Mutex
	cache map[string]string
}

// NewCovers builds a cover finder. abs maps catalog paths to real paths.
func NewCovers(abs func(string) string, fallback string) *Covers {
	return &Covers{abs: abs, fallback: fallback, cache: map[string]string{}}
}

// For returns the cover of an artist folder: the cover listed in Musica.data,
// else the first CoverCandidates file found, else any other image in the
// folder, else the fallback image. Only files that really are images count
// (a web page saved as "unnamed.jpg" is skipped). It reads the disk: call it
// off the screen thread.
func (c *Covers) For(folder, cover string) string {
	key := folder + "|" + cover
	c.mu.Lock()
	if p, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return p
	}
	c.mu.Unlock()

	p := c.find(folder, cover)
	c.mu.Lock()
	c.cache[key] = p
	c.mu.Unlock()
	return p
}

// Reset forgets cached answers (after a disk change or rescan).
func (c *Covers) Reset() {
	c.mu.Lock()
	c.cache = map[string]string{}
	c.mu.Unlock()
}

func (c *Covers) find(folder, cover string) string {
	// Names come from Windows: "karaokes/zoé/IMAGES.JPG" may be
	// "Karaokes/Zoé/images.jpg" on the Linux disk. Match every part ignoring case.
	var dirs []string
	if cover != "" {
		if p, err := library.Resolve(c.abs(cover)); err == nil {
			if isImage(p) {
				return p
			}
			dirs = append(dirs, filepath.Dir(p))
		}
	}
	if folder != "" {
		if dir, err := library.Resolve(c.abs(folder)); err == nil && (len(dirs) == 0 || dirs[0] != dir) {
			dirs = append([]string{dir}, dirs...)
		}
	}
	for _, dir := range dirs {
		if p := imageIn(dir); p != "" {
			return p
		}
	}
	return c.fallback
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true}

// imageIn returns the best cover image in dir: a CoverCandidates name
// first, then any image file (alphabetical, so the answer is stable).
func imageIn(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	names := make(map[string]string, len(entries))
	var others []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		low := strings.ToLower(e.Name())
		names[low] = e.Name()
		if imageExts[filepath.Ext(low)] && !strings.HasPrefix(low, ".") {
			others = append(others, e.Name())
		}
	}
	tried := map[string]bool{}
	for _, cand := range CoverCandidates {
		if real, ok := names[cand]; ok {
			tried[real] = true
			if p := filepath.Join(dir, real); isImage(p) {
				return p
			}
		}
	}
	sort.Strings(others)
	for _, n := range others {
		if !tried[n] {
			if p := filepath.Join(dir, n); isImage(p) {
				return p
			}
		}
	}
	return ""
}

// isImage reports whether p is an image Go can decode, from its header
// (cheap: it does not decode the pixels).
func isImage(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	_, _, err = image.DecodeConfig(bufio.NewReader(f))
	return err == nil
}
