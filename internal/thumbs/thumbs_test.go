package thumbs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/bmp"
)

func TestMakesSmallThumbnailOnce(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "images.jpg")
	big := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for x := 0; x < 1200; x++ {
		big.Set(x, 400, color.White)
	}
	f, _ := os.Create(src)
	jpeg.Encode(f, big, nil)
	f.Close()

	c := New(filepath.Join(dir, "thumbs"), 240)
	ready := make(chan struct{}, 4)
	c.OnReady = func() { ready <- struct{}{} }
	if p := c.Get("artista", func() string { return src }); p != "" {
		t.Fatalf("no debería existir todavía: %s", p)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("no se generó la miniatura")
	}
	p := c.Get("artista", nil)
	if p == "" {
		t.Fatal("Get no devuelve la miniatura")
	}
	tf, _ := os.Open(p)
	cfg, err := jpeg.DecodeConfig(tf)
	tf.Close()
	if err != nil || cfg.Width != 240 || cfg.Height != 160 {
		t.Fatalf("tamaño %dx%d %v", cfg.Width, cfg.Height, err)
	}
	// A new cache (after a restart) finds it without making it again.
	if New(filepath.Join(dir, "thumbs"), 240).Get("artista", nil) != p {
		t.Fatal("la miniatura no persiste")
	}
}

func TestBrokenImageIsNotRetried(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "roto.jpg")
	os.WriteFile(src, []byte("no es imagen"), 0o644)
	c := New(filepath.Join(dir, "thumbs"), 240)
	c.Get("roto", func() string { return src })
	waitIdle(t, c)
	c.Get("roto", func() string { return src })
	c.mu.Lock()
	defer c.mu.Unlock()
	if at, ok := c.failed["roto"]; len(c.queue) != 0 || !ok || !at.IsZero() {
		t.Fatal("una imagen rota no debe reintentarse")
	}
}

// waitIdle waits until the worker has nothing left to do.
func waitIdle(t *testing.T, c *Cache) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		r := c.running
		c.mu.Unlock()
		if !r {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el trabajo de miniaturas no terminó")
}

func encodeTo(t *testing.T, path string, enc func(io.Writer) error) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// Many covers were downloaded from the web: "unnamed.jpg" is often WebP,
// PNG or GIF. The format must come from the contents, not the name.
func TestCoversOfAnyFormatNamedJPG(t *testing.T) {
	dir := t.TempDir()
	rgba := image.NewRGBA(image.Rect(0, 0, 480, 360))
	for i := range rgba.Pix {
		rgba.Pix[i] = 0xff
	}
	pal := image.NewPaletted(image.Rect(0, 0, 100, 400), color.Palette{color.Black, color.White})
	encodeTo(t, filepath.Join(dir, "png.jpg"), func(w io.Writer) error { return png.Encode(w, rgba) })
	encodeTo(t, filepath.Join(dir, "gif.jpg"), func(w io.Writer) error { return gif.Encode(w, pal, nil) })
	encodeTo(t, filepath.Join(dir, "bmp.jpg"), func(w io.Writer) error { return bmp.Encode(w, rgba) })

	cases := []struct {
		src  string
		w, h int
	}{
		{"testdata/webp-lossy.jpg", 240, 160},       // VP8 (lossy) WebP, 300×200
		{"testdata/webp-alpha.jpg", 64, 48},         // VP8L WebP with transparency, small: kept as is
		{"testdata/cmyk-progressive.jpg", 240, 160}, // CMYK progressive JPEG, 300×200
		{filepath.Join(dir, "png.jpg"), 240, 180},   // PNG
		{filepath.Join(dir, "gif.jpg"), 60, 240},    // GIF
		{filepath.Join(dir, "bmp.jpg"), 240, 180},   // BMP
	}
	c := New(filepath.Join(dir, "thumbs"), 240)
	for _, tc := range cases {
		c.Get(tc.src, func() string { return tc.src })
		waitIdle(t, c)
		p := c.Get(tc.src, nil)
		if p == "" {
			c.mu.Lock()
			_, failed := c.failed[tc.src]
			c.mu.Unlock()
			t.Errorf("%s: no se hizo la miniatura (falló: %v)", tc.src, failed)
			continue
		}
		f, _ := os.Open(p)
		img, err := jpeg.Decode(f)
		f.Close()
		if err != nil || img.Bounds().Dx() != tc.w || img.Bounds().Dy() != tc.h {
			t.Errorf("%s: miniatura %v %v, quería %dx%d", tc.src, img.Bounds(), err, tc.w, tc.h)
			continue
		}
		if tc.src == "testdata/webp-alpha.jpg" {
			// Transparent corners go on white, not black.
			if r, g, b, _ := img.At(0, 0).RGBA(); r < 0xe000 || g < 0xe000 || b < 0xe000 {
				t.Errorf("esquina transparente quedó %v", img.At(0, 0))
			}
		}
	}
}

// pngHeader is just the start of a PNG claiming w×h pixels: enough for
// DecodeConfig, which must stop a giant cover before it is decoded.
func pngHeader(w, h uint32) []byte {
	ihdr := make([]byte, 17)
	copy(ihdr, "IHDR")
	binary.BigEndian.PutUint32(ihdr[4:], w)
	binary.BigEndian.PutUint32(ihdr[8:], h)
	ihdr[12], ihdr[13] = 8, 6 // 8-bit RGBA
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(ihdr)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(ihdr))
	return b.Bytes()
}

func TestGiantImageIsSkipped(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "poster.jpg")
	os.WriteFile(src, pngHeader(12000, 9000), 0o644)
	if _, err := decode(src); !errors.Is(err, errTooBig) {
		t.Fatalf("esperaba «demasiado grande», fue %v", err)
	}
	if temporary(fmt.Errorf("%w: x", errTooBig)) {
		t.Fatal("una imagen gigante no se reintenta")
	}
}

func TestFailureReasons(t *testing.T) {
	dir := t.TempDir()
	html := filepath.Join(dir, "a.jpg")
	os.WriteFile(html, []byte("<!DOCTYPE html><html>Acceso denegado</html>"), 0o644)
	empty := filepath.Join(dir, "b.jpg")
	os.WriteFile(empty, nil, 0o644)
	cut := filepath.Join(dir, "c.jpg")
	b, _ := os.ReadFile("testdata/cmyk-progressive.jpg")
	os.WriteFile(cut, b[:len(b)/2], 0o644)
	avif := filepath.Join(dir, "d.jpg")
	os.WriteFile(avif, []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00"), 0o644)

	for src, want := range map[string]string{
		html:  "HTML",
		empty: "vacío",
		cut:   "jpeg dañado",
		avif:  "AVIF",
	} {
		_, err := decode(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, quería %q", filepath.Base(src), err, want)
		}
		if temporary(err) {
			t.Errorf("%s: no debe reintentarse", filepath.Base(src))
		}
	}

	// The log gets one summary line, not one line per file.
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	c := New(filepath.Join(dir, "thumbs"), 240)
	c.running = true // queue all four before the worker starts: one batch
	for _, src := range []string{html, avif, empty, cut} {
		c.Get(src, func() string { return src })
	}
	go c.work()
	waitIdle(t, c)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "formato no soportado 3") || !strings.Contains(lines[0], "imagen dañada 1") {
		t.Fatalf("resumen del log: %q", out.String())
	}
}

// A read error (disk busy, cable loose) is tried again a bit later in the
// same session; a missing cover is not.
func TestReadErrorIsRetriedLater(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	c := New(filepath.Join(dir, "thumbs"), 240)
	c.now = func() time.Time { return now }
	src := dir // a folder: opens, but cannot be read as a file
	c.Get("k", func() string { return src })
	waitIdle(t, c)
	c.mu.Lock()
	at := c.failed["k"]
	c.mu.Unlock()
	if at.IsZero() {
		t.Fatal("un error de lectura debe reintentarse más tarde")
	}
	c.Get("k", func() string { return src })
	if c.queuedLen() != 0 {
		t.Fatal("no se reintenta enseguida")
	}
	now = now.Add(retryAfter + time.Second)
	good := "testdata/webp-lossy.jpg"
	c.Get("k", func() string { return good })
	waitIdle(t, c)
	if c.Get("k", nil) == "" {
		t.Fatal("tras la espera debe reintentarse")
	}

	c.Get("sin", func() string { return "" })
	waitIdle(t, c)
	c.mu.Lock()
	at, ok := c.failed["sin"]
	c.mu.Unlock()
	if !ok || !at.IsZero() {
		t.Fatal("sin portada: no se reintenta hasta ResetFailed")
	}
	c.ResetFailed()
	c.mu.Lock()
	_, ok = c.failed["sin"]
	c.mu.Unlock()
	if ok {
		t.Fatal("ResetFailed no olvidó las fallas")
	}
}

func (c *Cache) queuedLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queue) + len(c.queued)
}

func TestRelativeDirIsMadeAbsolute(t *testing.T) {
	c := New("data/thumbs", 240)
	if !filepath.IsAbs(c.dir) {
		t.Fatalf("la carpeta de miniaturas depende del directorio actual: %q", c.dir)
	}
}
