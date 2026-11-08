package catalog

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/josegamboa/karaokenino/internal/config"
)

// ImportOptions controls ImportMusicaData.
type ImportOptions struct {
	// KaraokeOnly imports only genres whose name contains "KARAOKE".
	KaraokeOnly bool
	// OnSkip is called for every malformed line (optional).
	OnSkip func(line int, text, reason string)
}

// ImportResult summarizes an import.
type ImportResult struct {
	Genres, Artists, Tracks, Removed, Skipped int
}

func (r ImportResult) String() string {
	return fmt.Sprintf("géneros=%d artistas=%d pistas=%d quitadas=%d omitidas=%d", r.Genres, r.Artists, r.Tracks, r.Removed, r.Skipped)
}

// ImportMusicaData loads a Musica.data song list into the catalog.
//
// It is safe to run again: songs keep their numbers (ids) and play history,
// songs found by the disk scanner are kept, and imported songs that are no
// longer listed are removed. Everything runs in one transaction.
func ImportMusicaData(ctx context.Context, cfg config.Config, musicaPath string, store *Store, opts ImportOptions) (ImportResult, error) {
	var res ImportResult
	f, err := os.Open(musicaPath)
	if err != nil {
		return res, err
	}
	defer f.Close()

	source := musicaPath
	if opts.KaraokeOnly {
		source += "#karaoke-only"
	}

	err = store.WithTx(ctx, func(tx *Store) error {
		if _, err := tx.q.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS seen(id INTEGER PRIMARY KEY); DELETE FROM seen;`); err != nil {
			return err
		}
		var (
			genreID    int64
			genreName  string
			artistID   int64
			artistName string
		)
		skip := func(line int, text, reason string) {
			res.Skipped++
			if opts.OnSkip != nil {
				opts.OnSkip(line, text, reason)
			}
		}
		err := ParseMusicaData(f, func(e Entry) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			switch e.Kind {
			case EntryGenre:
				genreName, genreID, artistID = e.Name, 0, 0
				if opts.KaraokeOnly && !isKaraokeGenre(genreName) {
					return nil
				}
				id, err := tx.UpsertGenre(ctx, genreName)
				if err != nil {
					return err
				}
				genreID = id
				res.Genres++
			case EntryArtist:
				artistID = 0
				if genreID == 0 {
					return nil
				}
				folder := strings.TrimSuffix(cfg.RelPath(e.Folder), "/")
				cover := ""
				if e.Image != "" {
					cover = cfg.RelPath(e.Image)
				}
				id, err := tx.UpsertArtist(ctx, genreID, e.Name, folder, cover)
				if err != nil {
					return err
				}
				artistID, artistName = id, e.Name
				res.Artists++
			case EntryTrack:
				if genreID == 0 {
					return nil
				}
				if artistID == 0 {
					skip(e.Line, e.Name, "pista sin artista")
					return nil
				}
				file := cfg.RelPath(e.File())
				id, err := tx.UpsertTrack(ctx, NewTrack{
					ArtistID:     artistID,
					FolderArtist: firstNonEmpty(artistName, path.Base(path.Dir(file))),
					Name:         e.Name,
					File:         file,
					MediaType:    e.Type,
					Karaoke:      isKaraokeGenre(genreName) || isKaraokeType(e.Type),
					Source:       "lista",
				})
				if err != nil {
					return fmt.Errorf("línea %d: %w", e.Line, err)
				}
				if _, err := tx.q.ExecContext(ctx, `INSERT OR IGNORE INTO seen(id) VALUES(?)`, id); err != nil {
					return err
				}
				res.Tracks++
			}
			return nil
		}, skip)
		if err != nil {
			return err
		}
		del, err := tx.q.ExecContext(ctx, `DELETE FROM tracks WHERE source<>'scan' AND id NOT IN (SELECT id FROM seen)`)
		if err != nil {
			return err
		}
		n, _ := del.RowsAffected()
		res.Removed = int(n)
		if err := tx.DeleteOrphans(ctx); err != nil {
			return err
		}
		if err := tx.RefreshCounts(ctx); err != nil {
			return err
		}
		if err := tx.RebuildSearch(ctx); err != nil {
			return err
		}
		return tx.SetMeta(ctx, "import_source", source)
	})
	return res, err
}

func isKaraokeGenre(genre string) bool {
	return strings.Contains(strings.ToUpper(genre), "KARAOKE")
}

// isKaraokeType: file types that are always karaoke.
func isKaraokeType(t string) bool {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "KAR", "CDG": // AVI/MPG outside karaoke genres are music videos
		return true
	default:
		return false
	}
}
