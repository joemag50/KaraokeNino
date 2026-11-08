// Package thumbs keeps small copies of artist covers on the rockola's own
// disk. Covers on the music disk are big images on a slow USB/NTFS drive; a
// 240 px copy decodes instantly and is made only once, then kept forever.
//
// The format is found from the file contents, never from the name: many
// covers were downloaded from the web and "unnamed.jpg" is often a WebP,
// PNG or GIF.
package thumbs

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // decode GIF covers (first frame)
	"image/jpeg"
	_ "image/png" // decode PNG covers
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/bmp" // decode BMP covers
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decode WebP covers (often saved as .jpg)
)

// Largest images decoded, in pixels. A decoded JPEG takes 1.5–3 bytes per
// pixel, other formats 4–8; one 40 MP JPEG is ~120 MB, which a 2 GB machine
// playing video can afford once at a time. Bigger files are skipped.
const (
	maxJPEGPixels  = 40_000_000
	maxOtherPixels = 16_000_000
)

// Failures that may go away by themselves (disk busy, thumbnail folder
// full) are tried again after retryAfter; the rest wait for ResetFailed.
const retryAfter = 2 * time.Minute

// Cache makes and finds thumbnails in dir.
type Cache struct {
	dir  string
	size int

	// OnReady runs (on a background goroutine) after new thumbnails are made.
	OnReady func()

	mu      sync.Mutex
	queue   []job
	queued  map[string]bool
	failed  map[string]time.Time // key → when to try again (zero: not until ResetFailed)
	running bool
	report  report

	// The log gets a summary at most every logEvery (scrolling makes many
	// small batches).
	logEvery time.Duration
	lastLog  time.Time

	now func() time.Time // for tests
}

type job struct {
	key string
	src func() string
}

// New uses dir for thumbnails of at most size×size pixels. A relative dir
// is made absolute now, so it does not depend on later changes of the
// working directory.
func New(dir string, size int) *Cache {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &Cache{dir: dir, size: size, queued: map[string]bool{}, failed: map[string]time.Time{}, now: time.Now, logEvery: time.Minute}
}

func (c *Cache) pathFor(src string) string {
	h := sha1.Sum([]byte(src))
	return filepath.Join(c.dir, hex.EncodeToString(h[:])+".jpg")
}

// Get returns the thumbnail for key if it was made before (it only looks at
// the local thumbnail folder, never the slow music disk). Otherwise it
// queues it: src runs in the background and returns the image file to
// shrink ("" = no cover). The caller shows a placeholder until OnReady.
func (c *Cache) Get(key string, src func() string) string {
	if key == "" {
		return ""
	}
	p := c.pathFor(key)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if src == nil || c.queued[key] {
		return ""
	}
	if at, ok := c.failed[key]; ok {
		if at.IsZero() || c.now().Before(at) {
			return ""
		}
		delete(c.failed, key)
	}
	c.queued[key] = true
	c.queue = append(c.queue, job{key, src})
	if !c.running {
		c.running = true
		go c.work()
	}
	return ""
}

// ResetFailed forgets covers that could not be made, so they are tried
// again (after the music disk shows up or a rescan).
func (c *Cache) ResetFailed() {
	c.mu.Lock()
	c.failed = map[string]time.Time{}
	c.mu.Unlock()
}

// work makes queued thumbnails one at a time (the disk is slow; the screen
// must stay responsive).
func (c *Cache) work() {
	made := 0
	var told time.Time
	for {
		c.mu.Lock()
		if len(c.queue) == 0 {
			if c.now().Sub(c.lastLog) >= c.logEvery {
				c.report.log() // one quick line; under the lock so it is done when running is false
				c.report = report{}
				c.lastLog = c.now()
			}
			c.running = false
			c.mu.Unlock()
			if made > 0 && c.OnReady != nil {
				c.OnReady()
			}
			return
		}
		// Newest first: what is on screen now.
		j := c.queue[len(c.queue)-1]
		c.queue = c.queue[:len(c.queue)-1]
		c.mu.Unlock()

		src := j.src()
		err := errNoImage
		if src != "" {
			err = c.make(src, c.pathFor(j.key))
		}
		c.mu.Lock()
		delete(c.queued, j.key)
		if err != nil {
			var retry time.Time
			if temporary(err) {
				retry = c.now().Add(retryAfter)
			}
			c.failed[j.key] = retry
		}
		c.report.add(src, err)
		c.mu.Unlock()
		if err != nil {
			continue
		}
		made++
		// Show covers as they arrive, without redrawing for each one.
		if c.OnReady != nil && time.Since(told) > 300*time.Millisecond {
			told = time.Now()
			c.OnReady()
		}
	}
}

var (
	errNoImage = errors.New("sin portada")
	errTooBig  = errors.New("imagen demasiado grande")
)

// formatError: the file is not an image Go can read (HTML page saved as
// .jpg, AVIF/HEIC, an empty file…).
type formatError struct{ what string }

func (e formatError) Error() string { return "formato no soportado (" + e.what + ")" }

// decodeError: a known format that could not be decoded (cut file,
// unusual JPEG feature).
type decodeError struct {
	format string
	err    error
}

func (e decodeError) Error() string { return fmt.Sprintf("%s dañado: %v", e.format, e.err) }

// temporary reports failures worth trying again later in this session.
func temporary(err error) bool {
	var fe formatError
	var de decodeError
	switch {
	case errors.Is(err, errNoImage), errors.Is(err, errTooBig), errors.As(err, &fe), errors.As(err, &de):
		return false
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return false
	}
	return true // I/O error reading the disk or writing the thumbnail
}

// make writes a thumbnail of src to dstPath.
func (c *Cache) make(src, dstPath string) error {
	img, err := decode(src)
	if err != nil {
		return err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(c.size) / float64(max(w, h))
	if scale > 1 {
		scale = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(int(float64(w)*scale), 1), max(int(float64(h)*scale), 1)))
	// JPEG has no transparency: put transparent covers (PNG/GIF/WebP logos)
	// on white instead of black.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	if w*h > 8_000_000 {
		img = nil
		debug.FreeOSMemory() // give the big decode back to the system now
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dir, ".thumb-*.jpg")
	if err != nil {
		return err
	}
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: 82}); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), dstPath)
}

// decode reads an image of any supported format, looking at the size
// first so a giant file is skipped before it eats the memory.
func decode(src string) (image.Image, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, err := br.Peek(32)
	if err != nil && err != io.EOF {
		return nil, err // the disk could not be read
	}
	cfg, format, err := image.DecodeConfig(br)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil, formatError{sniff(head)}
		}
		if len(head) == 0 {
			return nil, formatError{"archivo vacío"}
		}
		return nil, decodeError{sniffName(head), err}
	}
	limit := maxOtherPixels
	if format == "jpeg" {
		limit = maxJPEGPixels
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, decodeError{format, errors.New("tamaño 0")}
	}
	if int64(cfg.Width)*int64(cfg.Height) > int64(limit) {
		return nil, fmt.Errorf("%w: %dx%d", errTooBig, cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bufio.NewReader(f))
	if err != nil {
		return nil, decodeError{format, err}
	}
	return img, nil
}

// sniffName names a known format from its first bytes.
func sniffName(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("\xff\xd8")):
		return "jpeg"
	case bytes.HasPrefix(head, []byte("\x89PNG")):
		return "png"
	case bytes.HasPrefix(head, []byte("GIF8")):
		return "gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	case bytes.HasPrefix(head, []byte("BM")):
		return "bmp"
	}
	return "imagen"
}

// sniff says what an unreadable file seems to be, for the log.
func sniff(head []byte) string {
	t := strings.ToLower(strings.TrimSpace(string(head)))
	switch {
	case len(head) == 0:
		return "archivo vacío"
	case strings.HasPrefix(t, "<"):
		return "página web (HTML) guardada como imagen"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return "AVIF/HEIC (" + strings.TrimSpace(string(head[8:12])) + ")"
	case bytes.HasPrefix(head, []byte("II*\x00")) || bytes.HasPrefix(head, []byte("MM\x00*")):
		return "TIFF"
	case bytes.HasPrefix(head, []byte("\x00\x00\x01\x00")):
		return "ICO"
	case len(head) >= 12 && string(head[:4]) == "RIFF":
		return "RIFF " + string(head[8:12])
	}
	n := min(len(head), 8)
	return fmt.Sprintf("empieza con % x", head[:n])
}

// report sums up failures so the log gets one line per batch, not one per
// file (thousands of covers can fail the same way).
type report struct {
	made  int
	kinds map[string]*kindCount
}

type kindCount struct {
	n       int
	example string
}

func (r *report) add(src string, err error) {
	if err == nil {
		r.made++
		return
	}
	if err == errNoImage {
		return // no cover on the disk: nothing to fix
	}
	kind := "no se pudo leer"
	var fe formatError
	var de decodeError
	switch {
	case errors.As(err, &fe):
		kind = "formato no soportado"
	case errors.As(err, &de):
		kind = "imagen dañada"
	case errors.Is(err, errTooBig):
		kind = "demasiado grande"
	case errors.Is(err, fs.ErrNotExist):
		kind = "no existe"
	}
	if r.kinds == nil {
		r.kinds = map[string]*kindCount{}
	}
	k := r.kinds[kind]
	if k == nil {
		k = &kindCount{example: fmt.Sprintf("%s: %v", src, err)}
		r.kinds[kind] = k
	}
	k.n++
}

func (r report) log() {
	if len(r.kinds) == 0 {
		if r.made > 0 {
			log.Printf("miniaturas: %d nuevas", r.made)
		}
		return
	}
	names := make([]string, 0, len(r.kinds))
	for k := range r.kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, k := range names {
		parts = append(parts, fmt.Sprintf("%s %d (p. ej. %s)", k, r.kinds[k].n, r.kinds[k].example))
	}
	log.Printf("miniaturas: %d nuevas; sin miniatura: %s", r.made, strings.Join(parts, "; "))
}
