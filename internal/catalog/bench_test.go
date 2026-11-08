package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	bigOnce  sync.Once
	bigStore *Store
)

// bigBench is one 100k-song catalog shared by the read benchmarks.
func bigBench(b *testing.B) *Store {
	bigOnce.Do(func() {
		dir, err := os.MkdirTemp("", "kn-bench-")
		if err != nil {
			b.Fatal(err)
		}
		bigStore = fillBench(b, filepath.Join(dir, "catalog.db"), 100000)
	})
	return bigStore
}

// benchStore is a catalog with genres × artists × songs, like the real one
// (166k songs) but sized by n.
func benchStore(b *testing.B, n int) *Store {
	b.Helper()
	s := fillBench(b, filepath.Join(b.TempDir(), "catalog.db"), n)
	b.Cleanup(func() { s.Close() })
	return s
}

func fillBench(b *testing.B, path string, n int) *Store {
	ctx := context.Background()
	s, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	err = s.WithTx(ctx, func(tx *Store) error {
		gids := []int64{}
		for _, g := range []string{"KARAOKES", "MUSICA", "VIDEOS", "VALLENATOS"} {
			id, err := tx.UpsertGenre(ctx, g)
			if err != nil {
				return err
			}
			gids = append(gids, id)
		}
		var artist int64
		for i := 0; i < n; i++ {
			if i%18 == 0 {
				a := i / 18
				g := gids[a%len(gids)]
				name := fmt.Sprintf("%c Artista %d", 'A'+a%26, a)
				if artist, err = tx.UpsertArtist(ctx, g, name, fmt.Sprintf("g%d/%s", g, name), ""); err != nil {
					return err
				}
			}
			ext := [...]string{"MP3", "AVI", "MPG"}[i%3]
			if _, err := tx.UpsertTrack(ctx, NewTrack{ArtistID: artist, Name: fmt.Sprintf("Cancion %d amor corazon", i),
				File: fmt.Sprintf("x/%d.%s", i, ext), MediaType: ext}); err != nil {
				return err
			}
		}
		if err := tx.RefreshCounts(ctx); err != nil {
			return err
		}
		return tx.RebuildSearch(ctx)
	})
	if err != nil {
		b.Fatal(err)
	}
	return s
}

func BenchmarkUpsertTrack(b *testing.B) {
	ctx := context.Background()
	s := benchStore(b, 18)
	b.ResetTimer()
	err := s.WithTx(ctx, func(tx *Store) error {
		for i := 0; i < b.N; i++ {
			if _, err := tx.UpsertTrack(ctx, NewTrack{ArtistID: 1, Name: "Cancion", File: fmt.Sprintf("bench/%d.mp3", i), MediaType: "MP3"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func benchSearch(b *testing.B, q string) {
	ctx := context.Background()
	s := bigBench(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Search(ctx, q, nil, 60); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearch1Letter(b *testing.B)  { benchSearch(b, "a") }
func BenchmarkSearch2Letters(b *testing.B) { benchSearch(b, "co") }
func BenchmarkSearchWord(b *testing.B)     { benchSearch(b, "corazon") }

func BenchmarkRandomTrackGenre(b *testing.B) {
	ctx := context.Background()
	s := bigBench(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.RandomTrack(ctx, []int64{2}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStats(b *testing.B) {
	ctx := context.Background()
	s := bigBench(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Stats(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrackByID(b *testing.B) {
	ctx := context.Background()
	s := benchStore(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.TrackByID(ctx, int64(i%1000)+1); err != nil {
			b.Fatal(err)
		}
	}
}
