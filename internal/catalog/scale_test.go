package catalog

// Scale test for terabyte libraries. Skipped unless KN_SCALE_N is set:
//
//	KN_SCALE_N=1000000 KN_SCALE_DIR=/tmp/kn-tree go test -run TestScanAtScale -v ./internal/catalog
//
// It builds (once, reused across runs) a tree of N empty media files laid out
// as genre/artist/song, then measures time and peak Go heap of a first scan
// (empty catalog) and a rescan (everything known).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const scaleSongsPerArtist = 18

func scaleTree(t *testing.T, dir string, n int) {
	t.Helper()
	marker := filepath.Join(dir, fmt.Sprintf(".kn-scale-%d", n))
	if _, err := os.Stat(marker); err == nil {
		return
	}
	start := time.Now()
	artists := (n + scaleSongsPerArtist - 1) / scaleSongsPerArtist
	jobs := make(chan int)
	var wg sync.WaitGroup
	var made atomic.Int64
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range jobs {
				d := filepath.Join(dir, fmt.Sprintf("genero %d", a%7), fmt.Sprintf("Artista %06d", a))
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Error(err)
					return
				}
				for s := 0; s < scaleSongsPerArtist && a*scaleSongsPerArtist+s < n; s++ {
					ext := [...]string{".mp3", ".avi", ".mpg"}[s%3]
					f, err := os.Create(filepath.Join(d, fmt.Sprintf("Cancion numero %02d de prueba%s", s, ext)))
					if err != nil {
						t.Error(err)
						return
					}
					f.Close()
					made.Add(1)
				}
			}
		}()
	}
	for a := 0; a < artists; a++ {
		jobs <- a
	}
	close(jobs)
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	_ = os.WriteFile(marker, nil, 0o644)
	t.Logf("árbol de %d archivos creado en %s", made.Load(), time.Since(start).Round(time.Second))
}

// peakHeap samples HeapInuse while fn runs.
func peakHeap(fn func()) (peak uint64) {
	runtime.GC()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peak {
				peak = ms.HeapInuse
			}
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	fn()
	close(stop)
	<-done
	return peak
}

func TestScanAtScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("KN_SCALE_N"))
	if n == 0 {
		t.Skip("KN_SCALE_N no definido")
	}
	dir := os.Getenv("KN_SCALE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "kn-scale-tree")
	}
	root := filepath.Join(dir, fmt.Sprintf("n%d", n))
	scaleTree(t, root, n)

	ctx := context.Background()
	store := newStore(t)
	for _, pass := range []string{"primer escaneo", "re-escaneo"} {
		var res ScanResult
		var err error
		peak := peakHeap(func() { res, err = scaleScan(ctx, store, root) })
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d archivos, %d nuevas en %s, heap máx %d MB", pass, res.Files, res.Added,
			res.Took.Round(time.Millisecond), peak>>20)
	}
	st, _ := store.Stats(ctx)
	t.Logf("catálogo: %s", st)

	// Live memory of what the scanner keeps about known songs.
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	k, err := loadKnown(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("canciones conocidas en memoria: %d MB para %d", (after.HeapAlloc-before.HeapAlloc)>>20, len(k.tracks))
	runtime.KeepAlive(k)
}

func scaleScan(ctx context.Context, store *Store, root string) (ScanResult, error) {
	return ScanWith(ctx, store, root, ScanOptions{Fast: true})
}
