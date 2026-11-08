package catalog

// Maintenance of the catalog file. Every function here is safe to call while
// the rockola is running (WAL: the screen keeps reading); the slow ones say so.
// The admin menu and `importcatalog -mantenimiento` call them; see
// docs/MANTENIMIENTO.md.

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Optimize refreshes the query planner statistics when they are stale.
// It takes milliseconds (analysis_limit bounds the sampling) and runs on
// every Close and after each scan.
func (s *Store) Optimize(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `PRAGMA analysis_limit=400; PRAGMA optimize;`)
	return err
}

// Analyze recomputes all planner statistics (well under a second for 166k songs).
func (s *Store) Analyze(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `PRAGMA analysis_limit=0; ANALYZE;`)
	return err
}

// QuickCheck runs PRAGMA quick_check (seconds; reads the whole file but does
// not check index contents). It returns nil when the catalog is healthy.
func (s *Store) QuickCheck(ctx context.Context) ([]string, error) {
	return s.check(ctx, `PRAGMA quick_check`)
}

// IntegrityCheck runs the full PRAGMA integrity_check plus the full-text
// index check (slower than QuickCheck: also verifies every index).
func (s *Store) IntegrityCheck(ctx context.Context) ([]string, error) {
	problems, err := s.check(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO tracks_fts(tracks_fts) VALUES('integrity-check')`); err != nil {
		problems = append(problems, "índice de búsqueda: "+err.Error())
	}
	return problems, nil
}

func (s *Store) check(ctx context.Context, pragma string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, pragma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		if line != "ok" {
			out = append(out, line)
		}
	}
	return out, rows.Err()
}

// OptimizeSearch merges the full-text index segments (scans add songs in
// small batches, which leaves many segments). A second or two.
func (s *Store) OptimizeSearch(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tracks_fts(tracks_fts) VALUES('optimize')`)
	return err
}

// RebuildSearchIndex rebuilds the full-text index from the songs, in its own
// transaction (use it when IntegrityCheck reports a search index problem).
func (s *Store) RebuildSearchIndex(ctx context.Context) error {
	return s.WithTx(ctx, func(tx *Store) error { return tx.RebuildSearch(ctx) })
}

// Vacuum rewrites the catalog file without free pages (after purging many
// songs or plays) and truncates the WAL. It needs free space for a second
// copy of the catalog and blocks writers (plays, scan) while it runs: do it
// with the rockola idle, not during an event.
func (s *Store) Vacuum(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// Backup writes a consistent, compacted copy of the catalog to path (which
// must not exist), even while the rockola is playing and recording plays.
func (s *Store) Backup(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}

// FileSizes returns the size of the catalog and of its free (reusable) pages.
func (s *Store) FileSizes(ctx context.Context) (total, free int64, err error) {
	var pageSize, pages, freePages int64
	err = s.db.QueryRowContext(ctx, `SELECT page_size, page_count, freelist_count
FROM pragma_page_size, pragma_page_count, pragma_freelist_count`).Scan(&pageSize, &pages, &freePages)
	return pageSize * pages, pageSize * freePages, err
}

// PrunePlays deletes play history older than before, adding those plays to
// play_totals first: the all-time "lo más pedido" stays the same, only the
// ranking of old events is lost. Returns how many plays were folded.
func (s *Store) PrunePlays(ctx context.Context, before time.Time) (int64, error) {
	var n int64
	err := s.WithTx(ctx, func(tx *Store) error {
		cut := before.Unix()
		if _, err := tx.q.ExecContext(ctx, `
INSERT INTO play_totals(track_id, n)
SELECT track_id, COUNT(*) FROM plays WHERE played_at < ? GROUP BY track_id
ON CONFLICT(track_id) DO UPDATE SET n = n + excluded.n`, cut); err != nil {
			return err
		}
		res, err := tx.q.ExecContext(ctx, `DELETE FROM plays WHERE played_at < ?`, cut)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		// Events without plays left are not worth keeping either.
		_, err = tx.q.ExecContext(ctx, `DELETE FROM events WHERE started_at < ?
AND NOT EXISTS (SELECT 1 FROM plays p WHERE p.event_id=events.id)`, cut)
		return err
	})
	return n, err
}

// MissingTrack is a hidden song whose file is not on the disk.
type MissingTrack struct {
	ID     int64
	File   string
	Since  time.Time // when it was first seen missing (zero = unknown)
	Artist string
	Title  string
}

// MissingTracks lists songs hidden because their file was not on the disk
// since before `before` (zero time = all missing songs), oldest first.
func (s *Store) MissingTracks(ctx context.Context, before time.Time, limit int) ([]MissingTrack, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.file, t.missing_since, a.name, t.title FROM tracks t JOIN artists a ON a.id=t.artist_id
WHERE t.missing=1 AND (?=0 OR t.missing_since < ?) ORDER BY t.missing_since, t.id LIMIT ?`,
		unixOrZero(before), unixOrZero(before), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MissingTrack
	for rows.Next() {
		var m MissingTrack
		var since int64
		if err := rows.Scan(&m.ID, &m.File, &since, &m.Artist, &m.Title); err != nil {
			return nil, err
		}
		if since > 0 {
			m.Since = time.Unix(since, 0)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PurgeMissing deletes songs missing from the disk since before `before`
// (with their plays). Song numbers are never reused (AUTOINCREMENT), so the
// printed catalog stays valid for every other song. Run it only with the
// right disk connected: a scan of a wrong or half-mounted disk does not mark
// songs missing, but songs can stay missing from an older disk.
func (s *Store) PurgeMissing(ctx context.Context, before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, fmt.Errorf("PurgeMissing: falta la fecha límite")
	}
	var n int64
	err := s.WithTx(ctx, func(tx *Store) error {
		cut := before.Unix()
		const doomed = `SELECT id FROM tracks WHERE missing=1 AND missing_since>0 AND missing_since < ?`
		for _, q := range []string{
			`DELETE FROM tracks_fts WHERE rowid IN (` + doomed + `)`,
			`DELETE FROM plays WHERE track_id IN (` + doomed + `)`,
			`DELETE FROM play_totals WHERE track_id IN (` + doomed + `)`,
		} {
			if _, err := tx.q.ExecContext(ctx, q, cut); err != nil {
				return err
			}
		}
		res, err := tx.q.ExecContext(ctx, `DELETE FROM tracks WHERE missing=1 AND missing_since>0 AND missing_since < ?`, cut)
		if err != nil {
			return err
		}
		if n, _ = res.RowsAffected(); n == 0 {
			return nil
		}
		if err := tx.DeleteOrphans(ctx); err != nil {
			return err
		}
		return tx.RefreshCounts(ctx)
	})
	return n, err
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// ThumbName is the file name internal/thumbs uses for an artist cover:
// sha1(folder|cover).jpg (the UI asks thumbs for folder+"|"+cover).
func ThumbName(folder, cover string) string {
	h := sha1.Sum([]byte(folder + "|" + cover))
	return hex.EncodeToString(h[:]) + ".jpg"
}

// ThumbNames returns the thumbnail file names of every artist in the catalog.
func (s *Store) ThumbNames(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT folder, cover FROM artists`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var folder, cover string
		if err := rows.Scan(&folder, &cover); err != nil {
			return nil, err
		}
		if folder != "" || cover != "" {
			out[ThumbName(folder, cover)] = true
		}
	}
	return out, rows.Err()
}

// PlaysOlderThan counts plays recorded before t (what PrunePlays would fold).
func (s *Store) PlaysOlderThan(ctx context.Context, t time.Time) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plays WHERE played_at < ?`, t.Unix()).Scan(&n)
	return n, err
}

// IsThumbName reports whether name looks like a thumbnail made by internal/thumbs.
func IsThumbName(name string) bool {
	base, ok := strings.CutSuffix(name, ".jpg")
	if !ok || len(base) != 40 {
		return false
	}
	_, err := hex.DecodeString(base)
	return err == nil
}
