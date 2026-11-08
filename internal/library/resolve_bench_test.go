package library

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A genre folder with thousands of artists, named with different case than
// the catalog (Musica.data paths come from Windows): every play and cover
// lookup has to match "musica" against "MUSICA".
func BenchmarkResolveWrongCaseBigGenre(b *testing.B) {
	root := b.TempDir()
	const artists = 6000
	for i := 0; i < artists; i++ {
		d := filepath.Join(root, "MUSICA", fmt.Sprintf("Artista %04d", i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			b.Fatal(err)
		}
		if i%100 == 0 {
			if err := os.WriteFile(filepath.Join(d, "Song.MP3"), nil, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := (i % (artists / 100)) * 100
		p := filepath.Join(root, "musica", fmt.Sprintf("artista %04d", a), "song.mp3")
		if _, err := Resolve(p); err != nil {
			b.Fatal(err)
		}
	}
}
