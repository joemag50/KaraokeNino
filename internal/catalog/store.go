package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// schemaVersion is stored in PRAGMA user_version. Version 0/1 was the MVP
// schema (absolute paths, one track per artist); it is rebuilt. Version 3
// adds artists.tracks (visible songs per artist) so pages don't count rows.
// Version 5 adds tracks.missing_since, play_totals (history kept after
// pruning plays), a covering duplicate index and FTS prefix indexes.
const schemaVersion = 5

// Kind is what a song is for the guest: karaoke, music video or audio.
type Kind string

const (
	KindAll     Kind = ""
	KindKaraoke Kind = "karaoke"
	KindVideo   Kind = "video"
	KindAudio   Kind = "audio"
)

// KindOf classifies a song: karaoke if its genre is karaoke or it is CD+G,
// else audio or video by file type.
func KindOf(karaoke bool, mediaType string) Kind {
	switch {
	case karaoke:
		return KindKaraoke
	case isAudioType(mediaType):
		return KindAudio
	}
	return KindVideo
}

func isAudioType(t string) bool {
	switch strings.ToUpper(t) {
	case "MP3", "WMA", "WAV", "M4A", "OGG", "FLAC", "AAC":
		return true
	}
	return false
}

// countCol is the artists column holding visible songs of a kind.
func countCol(k Kind) string {
	switch k {
	case KindKaraoke:
		return "a.n_karaoke"
	case KindVideo:
		return "a.n_video"
	case KindAudio:
		return "a.n_audio"
	}
	return "a.tracks"
}

func kindCond(k Kind) string {
	if k == KindAll {
		return ""
	}
	return " AND t.kind='" + string(k) + "'"
}

// querier is satisfied by both *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

type Store struct {
	db    *sql.DB
	q     querier
	path  string
	stmts *stmtCache
}

// stmtCache keeps prepared statements for the hot write paths (scan and
// import insert tens of thousands of rows; parsing the SQL each time was a
// third of the cost). A transaction's statements die with it.
type stmtCache struct {
	mu sync.Mutex
	m  map[string]*sql.Stmt
}

func newStmtCache() *stmtCache { return &stmtCache{m: map[string]*sql.Stmt{}} }

// stmt returns a prepared statement for query on this Store's db or tx.
func (s *Store) stmt(ctx context.Context, query string) (*sql.Stmt, error) {
	c := s.stmts
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.m[query]; ok {
		return st, nil
	}
	st, err := s.q.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	c.m[query] = st
	return st, nil
}

func (c *stmtCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, st := range c.m {
		st.Close()
		delete(c.m, k)
	}
}

// Genre is a top-level category (KARAOKES, MUSICA...).
type Genre struct {
	ID   int64
	Name string
}

// Artist is a folder of songs. Folder and Cover are relative to the library root.
// In the guest list, folders with the same name in different genres are one
// artist: IDs holds every folder (ID is the first one).
type Artist struct {
	ID      int64
	GenreID int64
	Name    string
	Folder  string
	Cover   string
	Tracks  int
	IDs     []int64
}

// HasID reports whether id is one of the artist's folders.
func (a Artist) HasID(id int64) bool {
	if a.ID == id {
		return true
	}
	for _, x := range a.IDs {
		if x == id {
			return true
		}
	}
	return false
}

// Track is one playable file. ID doubles as the song number guests can dial.
type Track struct {
	ID        int64
	ArtistID  int64
	Artist    string // singer: track_artist if set, else the folder artist
	Folder    string // artist folder, for the cover
	Cover     string
	Title     string
	File      string // relative to the library root (or absolute)
	MediaType string
	Karaoke   bool
	Kind      Kind
	Genre     string
}

// IsAudio reports songs without video (the UI stays on screen while they play).
func (t Track) IsAudio() bool { return isAudioType(t.MediaType) }

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db, q: db, path: path, stmts: newStmtCache()}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// dsnPragmas run on every connection. Sized for a 2 GB machine whose
// catalog is ~100 MB today (166k songs) and may reach ~600 MB (1M files):
//   - WAL: the screen reads while the disk scan writes;
//   - synchronous=NORMAL: safe with WAL (a power cut loses at most the last
//     commits, never corrupts) and avoids an fsync per play/batch;
//   - busy_timeout: concurrent writers wait instead of failing;
//   - cache_size 16 MB per connection (×4 connections) instead of 2 MB;
//   - mmap 128 MB: reads come straight from the OS page cache (fits a 32-bit build);
//   - temp_store=MEMORY: ORDER BY / GROUP BY scratch stays in RAM;
//   - journal_size_limit 64 MB: the WAL file shrinks back after a big scan.
const dsnPragmas = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)" +
	"&_pragma=synchronous(NORMAL)&_pragma=cache_size(-16000)&_pragma=mmap_size(134217728)" +
	"&_pragma=temp_store(MEMORY)&_pragma=journal_size_limit(67108864)"

// Path is the catalog file.
func (s *Store) Path() string { return s.path }

// Close runs PRAGMA optimize (refreshes planner statistics when they are
// stale; bounded to a few ms by analysis_limit) and closes the catalog.
func (s *Store) Close() error {
	if s.stmts != nil {
		s.stmts.close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = s.Optimize(ctx)
	cancel()
	return s.db.Close()
}

// WithTx runs fn with a Store bound to a single transaction.
// It commits if fn returns nil and rolls back otherwise.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Store{db: s.db, q: tx, path: s.path, stmts: newStmtCache()}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) addColumn(table, def string) error {
	name := strings.Fields(def)[0]
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + def)
	return err
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v >= 2 && v < schemaVersion {
		// v2 → v3: artists.tracks; v3 → v4: song kind + counts per kind;
		// v4 → v5: missing_since.
		for _, c := range []struct{ table, col string }{
			{"artists", "tracks INTEGER NOT NULL DEFAULT 0"},
			{"artists", "n_karaoke INTEGER NOT NULL DEFAULT 0"},
			{"artists", "n_video INTEGER NOT NULL DEFAULT 0"},
			{"artists", "n_audio INTEGER NOT NULL DEFAULT 0"},
			{"tracks", "kind TEXT NOT NULL DEFAULT ''"},
			{"tracks", "missing_since INTEGER NOT NULL DEFAULT 0"},
		} {
			if err := s.addColumn(c.table, c.col); err != nil {
				return err
			}
		}
		// Songs already hidden start counting their missing days now.
		if _, err := s.db.Exec(`UPDATE tracks SET missing_since=? WHERE missing=1 AND missing_since=0`, time.Now().Unix()); err != nil {
			return err
		}
		if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_tracks_dup`); err != nil {
			return err
		}
	}
	if v >= 2 && v < 4 {
		if _, err := s.db.Exec(`
UPDATE tracks SET karaoke = CASE WHEN upper(media_type) IN ('CDG','KAR') OR EXISTS (
    SELECT 1 FROM artists a JOIN genres g ON g.id=a.genre_id
    WHERE a.id=tracks.artist_id AND upper(g.name) LIKE '%KARAOKE%') THEN 1 ELSE 0 END;
UPDATE tracks SET kind = CASE WHEN karaoke=1 THEN 'karaoke'
  WHEN upper(media_type) IN ('MP3','WMA','WAV','M4A','OGG','FLAC','AAC') THEN 'audio' ELSE 'video' END;`); err != nil {
			return err
		}
		if err := s.RefreshCounts(context.Background()); err != nil {
			return err
		}
	}
	if v < 2 {
		var old int
		_ = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='tracks'`).Scan(&old)
		if old > 0 {
			log.Printf("catálogo: esquema v%d antiguo, se reconstruye (vuelve a importar o espera el escaneo del disco)", v)
			if _, err := s.db.Exec(`DROP TABLE IF EXISTS tracks; DROP TABLE IF EXISTS artists; DROP TABLE IF EXISTS genres; DROP TABLE IF EXISTS tracks_fts;`); err != nil {
				return err
			}
		}
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS genres (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS artists (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  genre_id INTEGER NOT NULL REFERENCES genres(id),
  name TEXT NOT NULL,
  folder TEXT NOT NULL,
  cover TEXT NOT NULL DEFAULT '',
  sort_key TEXT NOT NULL DEFAULT '',
  tracks INTEGER NOT NULL DEFAULT 0,
  n_karaoke INTEGER NOT NULL DEFAULT 0,
  n_video INTEGER NOT NULL DEFAULT 0,
  n_audio INTEGER NOT NULL DEFAULT 0,
  UNIQUE(genre_id, folder)
);
CREATE TABLE IF NOT EXISTS tracks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  artist_id INTEGER NOT NULL REFERENCES artists(id),
  title TEXT NOT NULL,
  track_artist TEXT NOT NULL DEFAULT '',
  file TEXT NOT NULL UNIQUE,
  file_key TEXT NOT NULL DEFAULT '',
  media_type TEXT NOT NULL,
  karaoke INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL DEFAULT '',
  missing INTEGER NOT NULL DEFAULT 0,
  missing_since INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'lista',
  sort_key TEXT NOT NULL DEFAULT '',
  dup_key TEXT NOT NULL DEFAULT '',
  added_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id, sort_key);
CREATE INDEX IF NOT EXISTS idx_tracks_kind ON tracks(kind, missing);
CREATE INDEX IF NOT EXISTS idx_tracks_file_key ON tracks(file_key);
CREATE INDEX IF NOT EXISTS idx_tracks_dup_missing ON tracks(dup_key, missing);
CREATE INDEX IF NOT EXISTS idx_artists_genre ON artists(genre_id, sort_key);
CREATE VIRTUAL TABLE IF NOT EXISTS tracks_fts USING fts5(` + ftsColumns + `);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  started_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS plays (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  track_id INTEGER NOT NULL,
  event_id INTEGER NOT NULL DEFAULT 0,
  played_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_plays_event ON plays(event_id, track_id);
CREATE INDEX IF NOT EXISTS idx_plays_played ON plays(played_at);
-- Plays folded in by PrunePlays: all-time "lo más pedido" keeps counting them.
CREATE TABLE IF NOT EXISTS play_totals (
  track_id INTEGER PRIMARY KEY,
  n INTEGER NOT NULL
);
`)
	if err != nil {
		return err
	}
	if err := s.upgradeSearch(); err != nil {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion))
	return err
}

// ftsColumns: prefix='2 3' keeps an index of 2- and 3-letter prefixes, so a
// search for "co" or "cor" does not merge thousands of term lists (half the
// time on 166k songs, for ~10 MB more).
const ftsColumns = `title, artist, tokenize='unicode61 remove_diacritics 2', prefix='2 3'`

// upgradeSearch rebuilds an older full-text index without prefix indexes.
func (s *Store) upgradeSearch() error {
	var def string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='tracks_fts'`).Scan(&def); err != nil {
		return err
	}
	if strings.Contains(def, "prefix=") {
		return nil
	}
	ctx := context.Background()
	return s.WithTx(ctx, func(tx *Store) error {
		if _, err := tx.q.ExecContext(ctx, `DROP TABLE tracks_fts; CREATE VIRTUAL TABLE tracks_fts USING fts5(`+ftsColumns+`);`); err != nil {
			return err
		}
		return tx.RebuildSearch(ctx)
	})
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.q.ExecContext(ctx, `
INSERT INTO meta(key,value) VALUES(?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) Meta(ctx context.Context, key string) string {
	var v string
	_ = s.q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	return v
}

// ---------- writes (importer and scanner) ----------

func (s *Store) UpsertGenre(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx, `
INSERT INTO genres(name) VALUES(?)
ON CONFLICT(name) DO UPDATE SET name=excluded.name
RETURNING id`, name).Scan(&id)
	return id, err
}

func (s *Store) UpsertArtist(ctx context.Context, genreID int64, name, folder, cover string) (int64, error) {
	st, err := s.stmt(ctx, `
INSERT INTO artists(genre_id,name,folder,cover,sort_key) VALUES(?,?,?,?,?)
ON CONFLICT(genre_id,folder) DO UPDATE SET name=excluded.name,
  cover=CASE WHEN excluded.cover<>'' THEN excluded.cover ELSE artists.cover END,
  sort_key=excluded.sort_key
RETURNING id`)
	if err != nil {
		return 0, err
	}
	var id int64
	err = st.QueryRowContext(ctx, genreID, name, folder, cover, SortKey(name)).Scan(&id)
	return id, err
}

// NewTrack is what importer and scanner write.
type NewTrack struct {
	ArtistID     int64
	FolderArtist string // artist folder name, to split "Title - Artist"
	Name         string // file name without extension
	File         string // catalog path
	MediaType    string
	Karaoke      bool
	Source       string
}

// UpsertTrack inserts or updates a track by file path and returns its id.
// Existing ids are kept, so song numbers and play history survive re-imports.
func (s *Store) UpsertTrack(ctx context.Context, t NewTrack) (int64, error) {
	title, artist := ParseTitle(t.FolderArtist, t.Name)
	if t.Source == "" {
		t.Source = "lista"
	}
	st, err := s.stmt(ctx, `
INSERT INTO tracks(artist_id,title,track_artist,file,file_key,media_type,karaoke,kind,missing,source,sort_key,dup_key,added_at)
VALUES(?,?,?,?,?,?,?,?,0,?,?,?,?)
ON CONFLICT(file) DO UPDATE SET artist_id=excluded.artist_id, title=excluded.title,
  track_artist=excluded.track_artist, media_type=excluded.media_type, karaoke=excluded.karaoke, kind=excluded.kind,
  sort_key=excluded.sort_key, dup_key=excluded.dup_key, missing=0, missing_since=0
RETURNING id`)
	if err != nil {
		return 0, err
	}
	var id int64
	err = st.QueryRowContext(ctx,
		t.ArtistID, title, artist, t.File, strings.ToLower(t.File), strings.ToUpper(t.MediaType), boolInt(t.Karaoke),
		string(KindOf(t.Karaoke, t.MediaType)), t.Source, SortKey(title), DupKey(title, firstNonEmpty(artist, t.FolderArtist)), time.Now().Unix()).Scan(&id)
	return id, err
}

// indexSearch adds (or refreshes) one song in the full-text index, so a scan
// does not have to rebuild the whole index for a few new songs.
func (s *Store) indexSearch(ctx context.Context, trackID int64) error {
	st, err := s.stmt(ctx, `
INSERT OR REPLACE INTO tracks_fts(rowid,title,artist)
SELECT t.id, t.title, CASE WHEN t.track_artist<>'' THEN t.track_artist || ' ' || a.name ELSE a.name END
FROM tracks t JOIN artists a ON a.id=t.artist_id WHERE t.id=?`)
	if err != nil {
		return err
	}
	_, err = st.ExecContext(ctx, trackID)
	return err
}

// refreshArtistCounts recomputes the visible songs of some artists.
func (s *Store) refreshArtistCounts(ctx context.Context, ids map[int64]bool) error {
	st, err := s.stmt(ctx, `UPDATE artists SET `+countsSet+` WHERE id=?`)
	if err != nil {
		return err
	}
	for id := range ids {
		if _, err := st.ExecContext(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// RebuildSearch refreshes the full-text index (a second or two for 166k songs).
func (s *Store) RebuildSearch(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `
DELETE FROM tracks_fts;
INSERT INTO tracks_fts(rowid,title,artist)
SELECT t.id, t.title, CASE WHEN t.track_artist<>'' THEN t.track_artist || ' ' || a.name ELSE a.name END
FROM tracks t JOIN artists a ON a.id=t.artist_id;`)
	return err
}

// RefreshCounts updates the visible song count of every artist (after an
// import or scan). Pages and genre lists read this instead of counting.
func (s *Store) RefreshCounts(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `UPDATE artists SET `+countsSet)
	return err
}

// countsSet recomputes an artist's visible songs, total and per kind.
const countsSet = `(tracks, n_karaoke, n_video, n_audio) = (
  SELECT COUNT(*), IFNULL(SUM(kind='karaoke'),0), IFNULL(SUM(kind='video'),0), IFNULL(SUM(kind='audio'),0)
  FROM tracks t WHERE t.artist_id=artists.id AND t.missing=0)`

// DeleteOrphans removes artists without tracks and genres without artists.
func (s *Store) DeleteOrphans(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `
DELETE FROM artists WHERE NOT EXISTS (SELECT 1 FROM tracks t WHERE t.artist_id=artists.id);
DELETE FROM genres WHERE NOT EXISTS (SELECT 1 FROM artists a WHERE a.genre_id=genres.id);`)
	return err
}

// SetMissing flags a file that is not on disk (hidden from guests) or back again.
func (s *Store) SetMissing(ctx context.Context, trackID int64, missing bool) error {
	since := int64(0)
	if missing {
		since = time.Now().Unix()
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE tracks SET missing=?,
  missing_since=CASE WHEN ?=0 THEN 0 WHEN missing_since>0 THEN missing_since ELSE ? END WHERE id=?`,
		boolInt(missing), boolInt(missing), since, trackID); err != nil {
		return err
	}
	_, err := s.q.ExecContext(ctx, `UPDATE artists SET `+countsSet+`
WHERE id=(SELECT artist_id FROM tracks WHERE id=?)`, trackID)
	return err
}

// ---------- reads (UI) ----------

// Genres returns genres that have playable songs of kind, by name.
func (s *Store) Genres(ctx context.Context, kind Kind) ([]Genre, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT g.id, g.name FROM genres g
WHERE EXISTS (SELECT 1 FROM artists a WHERE a.genre_id=g.id AND `+countCol(kind)+`>0)
ORDER BY g.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Genre
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func visibleArtist(k Kind) string { return `a.genre_id=? AND ` + countCol(k) + `>0` }

// KindCounts returns how many playable songs there are of each kind.
func (s *Store) KindCounts(ctx context.Context) (map[Kind]int, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT kind, COUNT(*) FROM tracks WHERE missing=0 GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[Kind]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[Kind(k)] = n
	}
	return out, rows.Err()
}

func (s *Store) ArtistCount(ctx context.Context, genreID int64, kind Kind) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM artists a WHERE `+visibleArtist(kind), genreID).Scan(&n)
	return n, err
}

func (s *Store) Artists(ctx context.Context, genreID int64, kind Kind, limit, offset int) ([]Artist, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.QueryContext(ctx, `
SELECT a.id, a.genre_id, a.name, a.folder, a.cover, `+countCol(kind)+`
FROM artists a WHERE `+visibleArtist(kind)+`
ORDER BY a.sort_key, a.id
LIMIT ? OFFSET ?`, genreID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artist
	for rows.Next() {
		var r Artist
		if err := rows.Scan(&r.ID, &r.GenreID, &r.Name, &r.Folder, &r.Cover, &r.Tracks); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Artist returns one artist by id.
func (s *Store) Artist(ctx context.Context, id int64) (Artist, error) {
	var r Artist
	err := s.q.QueryRowContext(ctx, `SELECT id, genre_id, name, folder, cover, tracks FROM artists WHERE id=?`, id).
		Scan(&r.ID, &r.GenreID, &r.Name, &r.Folder, &r.Cover, &r.Tracks)
	return r, err
}

// LetterCount is how many artists start with a letter ("#" = digits/symbols).
type LetterCount struct {
	Letter string
	Count  int
}

const letterExpr = `CASE WHEN substr(a.sort_key,1,1) BETWEEN 'a' AND 'z' THEN substr(a.sort_key,1,1) ELSE '#' END`

// Letters returns the letters that have artists in the given genres
// (nil = all), A..Z then "#". Same-name artists count once.
func (s *Store) Letters(ctx context.Context, genreIDs []int64, kind Kind) ([]LetterCount, error) {
	where, args := genreFilter(genreIDs)
	rows, err := s.q.QueryContext(ctx, `SELECT `+letterExpr+` l, COUNT(DISTINCT a.sort_key) FROM artists a
WHERE `+countCol(kind)+`>0`+where+`
GROUP BY l ORDER BY l='#', l`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LetterCount
	for rows.Next() {
		var lc LetterCount
		if err := rows.Scan(&lc.Letter, &lc.Count); err != nil {
			return nil, err
		}
		out = append(out, lc)
	}
	return out, rows.Err()
}

// ArtistsByLetter lists the artists of the given genres (nil = all) starting
// with letter, sorted, in one list: folders with the same name in different
// genres become one artist (see Artist.IDs).
func (s *Store) ArtistsByLetter(ctx context.Context, genreIDs []int64, kind Kind, letter string) ([]Artist, error) {
	where, args := genreFilter(genreIDs)
	rows, err := s.q.QueryContext(ctx, `
SELECT a.id, a.genre_id, a.name, a.folder, a.cover, `+countCol(kind)+`, a.sort_key
FROM artists a WHERE `+countCol(kind)+`>0`+where+` AND `+letterExpr+`=?
ORDER BY a.sort_key, a.id`, append(args, strings.ToLower(letter))...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artist
	last := ""
	for rows.Next() {
		var r Artist
		var key string
		if err := rows.Scan(&r.ID, &r.GenreID, &r.Name, &r.Folder, &r.Cover, &r.Tracks, &key); err != nil {
			return nil, err
		}
		if len(out) > 0 && key == last {
			m := &out[len(out)-1]
			m.IDs = append(m.IDs, r.ID)
			m.Tracks += r.Tracks
			if m.Cover == "" && r.Cover != "" {
				m.Folder, m.Cover = r.Folder, r.Cover // show a folder that has a cover
			}
			continue
		}
		r.IDs = []int64{r.ID}
		out = append(out, r)
		last = key
	}
	return out, rows.Err()
}

// ArtistIndexForLetter returns the position of the first artist whose name
// starts at letter or later ("#" = digits/symbols = 0).
func (s *Store) ArtistIndexForLetter(ctx context.Context, genreID int64, kind Kind, letter string) (int, error) {
	letter = strings.ToLower(letter)
	if letter == "#" || letter == "" {
		return 0, nil
	}
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM artists a WHERE `+visibleArtist(kind)+` AND a.sort_key < ?`, genreID, letter).Scan(&n)
	return n, err
}

// ArtistPosition returns the artist's index in its genre's sorted list.
func (s *Store) ArtistPosition(ctx context.Context, genreID int64, kind Kind, artistID int64) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM artists a WHERE `+visibleArtist(kind)+`
AND (a.sort_key, a.id) < (SELECT sort_key, id FROM artists WHERE id=?)`, genreID, artistID).Scan(&n)
	return n, err
}

const trackCols = `
SELECT t.id, t.artist_id, CASE WHEN t.track_artist<>'' THEN t.track_artist ELSE a.name END,
  a.folder, a.cover, t.title, t.file, t.media_type, t.karaoke, t.kind, g.name`

const trackSelect = trackCols + `
FROM tracks t JOIN artists a ON a.id=t.artist_id JOIN genres g ON g.id=a.genre_id`

// trackSeek walks tracks in id order first (CROSS JOIN fixes the join
// order): with a genre filter, SQLite otherwise collects and sorts every
// song of the genre (125k for MUSICA) to return one.
const trackSeek = trackCols + `
FROM tracks t CROSS JOIN artists a ON a.id=t.artist_id JOIN genres g ON g.id=a.genre_id`

func scanTracks(rows *sql.Rows) ([]Track, error) {
	defer rows.Close()
	var out []Track
	for rows.Next() {
		var t Track
		var k int
		if err := rows.Scan(&t.ID, &t.ArtistID, &t.Artist, &t.Folder, &t.Cover, &t.Title, &t.File, &t.MediaType, &k, &t.Kind, &t.Genre); err != nil {
			return nil, err
		}
		t.Karaoke = k == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

// Tracks lists an artist's playable songs of kind by title.
func (s *Store) Tracks(ctx context.Context, artistID int64, kind Kind) ([]Track, error) {
	rows, err := s.q.QueryContext(ctx, trackSelect+` WHERE t.artist_id=? AND t.missing=0`+kindCond(kind)+` ORDER BY t.sort_key, t.id`, artistID)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

// TracksOf lists the playable songs of kind of several artist folders (one
// merged artist) by title.
func (s *Store) TracksOf(ctx context.Context, artistIDs []int64, kind Kind) ([]Track, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}
	ph := strings.Repeat(",?", len(artistIDs))[1:]
	args := make([]any, len(artistIDs))
	for i, id := range artistIDs {
		args[i] = id
	}
	rows, err := s.q.QueryContext(ctx, trackSelect+` WHERE t.artist_id IN (`+ph+`) AND t.missing=0`+kindCond(kind)+` ORDER BY t.sort_key, t.id`, args...)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

// TrackByID finds a song by its number. Missing files are not returned.
func (s *Store) TrackByID(ctx context.Context, id int64) (Track, error) {
	rows, err := s.q.QueryContext(ctx, trackSelect+` WHERE t.id=? AND t.missing=0`, id)
	if err != nil {
		return Track{}, err
	}
	ts, err := scanTracks(rows)
	if err != nil {
		return Track{}, err
	}
	if len(ts) == 0 {
		return Track{}, sql.ErrNoRows
	}
	return ts[0], nil
}

// Search finds songs by words in the title or singer, ignoring accents and
// case ("corazon jenni"). genreIDs limits results (nil = all genres).
func (s *Store) Search(ctx context.Context, query string, genreIDs []int64, limit int) ([]Track, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	where, args := genreFilter(genreIDs)
	args = append([]any{match}, args...)
	args = append(args, limit)
	sqlq := trackSelect + `
JOIN tracks_fts f ON f.rowid=t.id
WHERE tracks_fts MATCH ? AND t.missing=0` + where + `
ORDER BY f.rank LIMIT ?`
	if shortQuery(query) {
		// "a" or "la" match half the catalog: ranking 80k rows to show 60
		// took ~130 ms on a fast PC (far more on the rockola). Return the first
		// matches instead; the next letter narrows the list anyway.
		sqlq = trackCols + `
FROM tracks_fts f CROSS JOIN tracks t ON t.id=f.rowid JOIN artists a ON a.id=t.artist_id JOIN genres g ON g.id=a.genre_id
WHERE tracks_fts MATCH ? AND t.missing=0` + where + `
LIMIT ?`
	}
	rows, err := s.q.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

// RandomTrack picks a playable song from the given genres (background music).
func (s *Store) RandomTrack(ctx context.Context, genreIDs []int64) (Track, error) {
	where, args := genreFilter(genreIDs)
	var maxID int64
	if err := s.q.QueryRowContext(ctx, `SELECT IFNULL(MAX(id),0) FROM tracks`).Scan(&maxID); err != nil || maxID == 0 {
		return Track{}, sql.ErrNoRows
	}
	// Seek from a random id instead of ORDER BY RANDOM() over 166k rows.
	pivot := time.Now().UnixNano()%maxID + 1
	for _, cmp := range []string{">=", "<"} {
		rows, err := s.q.QueryContext(ctx, trackSeek+` WHERE t.missing=0 AND t.id`+cmp+`?`+where+` ORDER BY t.id LIMIT 1`,
			append([]any{pivot}, args...)...)
		if err != nil {
			return Track{}, err
		}
		ts, err := scanTracks(rows)
		if err != nil {
			return Track{}, err
		}
		if len(ts) > 0 {
			return ts[0], nil
		}
	}
	return Track{}, sql.ErrNoRows
}

func genreFilter(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return ` AND a.genre_id IN (` + ph + `)`, args
}

// shortQuery reports a search whose words are all one or two letters.
func shortQuery(q string) bool {
	for _, w := range strings.Fields(q) {
		if utf8.RuneCountInString(w) > 2 {
			return false
		}
	}
	return true
}

// ftsQuery turns free text into an FTS5 prefix query: each word must match
// the start of a word ("cora jen" -> "cora"* "jen"*). Quotes are stripped so
// guests can't inject FTS syntax.
func ftsQuery(q string) string {
	var parts []string
	for _, w := range strings.Fields(q) {
		w = strings.Map(func(r rune) rune {
			if r == '"' || r == '*' || r == ':' || r == '^' || r == '(' || r == ')' {
				return -1
			}
			return r
		}, w)
		if w != "" {
			parts = append(parts, `"`+w+`"*`)
		}
	}
	return strings.Join(parts, " ")
}

// ---------- events and ranking ----------

func (s *Store) StartEvent(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx, `INSERT INTO events(name,started_at) VALUES(?,?) RETURNING id`, name, time.Now().Unix()).Scan(&id)
	return id, err
}

func (s *Store) RecordPlay(ctx context.Context, trackID, eventID int64) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO plays(track_id,event_id,played_at) VALUES(?,?,?)`, trackID, eventID, time.Now().Unix())
	return err
}

// Ranked is a song with its play count.
type Ranked struct {
	Track
	Plays int
}

// Top returns the most played songs for an event (0 = all time, including
// plays folded into play_totals by PrunePlays).
func (s *Store) Top(ctx context.Context, eventID int64, limit int) ([]Ranked, error) {
	counts := `SELECT track_id, SUM(n) n FROM (
  SELECT track_id, COUNT(*) n FROM plays GROUP BY track_id
  UNION ALL SELECT track_id, n FROM play_totals) GROUP BY track_id`
	args := []any{}
	if eventID > 0 {
		counts = `SELECT track_id, COUNT(*) n FROM plays WHERE event_id=? GROUP BY track_id`
		args = append(args, eventID)
	}
	args = append(args, limit)
	rows, err := s.q.QueryContext(ctx, `
WITH top AS (`+counts+` ORDER BY n DESC LIMIT ?)
SELECT t.id, t.artist_id, CASE WHEN t.track_artist<>'' THEN t.track_artist ELSE a.name END,
  a.folder, a.cover, t.title, t.file, t.media_type, t.karaoke, t.kind, g.name, top.n
FROM top JOIN tracks t ON t.id=top.track_id JOIN artists a ON a.id=t.artist_id JOIN genres g ON g.id=a.genre_id
ORDER BY top.n DESC, t.sort_key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ranked
	for rows.Next() {
		var r Ranked
		var k int
		if err := rows.Scan(&r.ID, &r.ArtistID, &r.Artist, &r.Folder, &r.Cover, &r.Title, &r.File, &r.MediaType, &k, &r.Kind, &r.Genre, &r.Plays); err != nil {
			return nil, err
		}
		r.Karaoke = k == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------- diagnostics ----------

// Stats summarizes the catalog for the admin diagnostics screen.
type Stats struct {
	Genres, Artists, Tracks, Karaoke, Missing, DupGroups, Plays int
}

func (s Stats) String() string {
	return fmt.Sprintf("géneros=%d artistas=%d pistas=%d karaoke=%d faltantes=%d duplicados=%d reproducciones=%d",
		s.Genres, s.Artists, s.Tracks, s.Karaoke, s.Missing, s.DupGroups, s.Plays)
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.q.QueryRowContext(ctx, `SELECT
  (SELECT COUNT(*) FROM genres),
  (SELECT COUNT(*) FROM artists),
  (SELECT COUNT(*) FROM tracks),
  (SELECT COUNT(*) FROM tracks WHERE karaoke=1 AND missing=0),
  (SELECT COUNT(*) FROM tracks WHERE missing=1),
  (SELECT COUNT(*) FROM (SELECT dup_key FROM tracks WHERE missing=0 GROUP BY dup_key HAVING COUNT(*)>1)),
  (SELECT COUNT(*) FROM plays) + (SELECT IFNULL(SUM(n),0) FROM play_totals)`).Scan(&st.Genres, &st.Artists, &st.Tracks, &st.Karaoke, &st.Missing, &st.DupGroups, &st.Plays)
	return st, err
}

// Duplicates lists songs stored more than once (same title and singer).
func (s *Store) Duplicates(ctx context.Context, limit int) ([][]Track, error) {
	rows, err := s.q.QueryContext(ctx, trackSelect+`
WHERE t.missing=0 AND t.dup_key IN (
  SELECT dup_key FROM tracks WHERE missing=0 GROUP BY dup_key HAVING COUNT(*)>1 ORDER BY dup_key LIMIT ?)
ORDER BY t.dup_key, t.id`, limit)
	if err != nil {
		return nil, err
	}
	ts, err := scanTracks(rows)
	if err != nil {
		return nil, err
	}
	var out [][]Track
	last := ""
	for _, t := range ts {
		k := DupKey(t.Title, t.Artist)
		if len(out) == 0 || k != last {
			out = append(out, nil)
			last = k
		}
		out[len(out)-1] = append(out[len(out)-1], t)
	}
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
