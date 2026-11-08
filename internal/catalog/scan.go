package catalog

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MediaExts are the file types the scanner adds (mpv plays all of them).
var MediaExts = map[string]bool{
	".mp3": true, ".wma": true, ".wav": true, ".m4a": true, ".ogg": true, ".flac": true, ".aac": true,
	".avi": true, ".mpg": true, ".mpeg": true, ".mp4": true, ".mkv": true, ".wmv": true,
	".flv": true, ".vob": true, ".webm": true, ".mov": true, ".asf": true, ".cdg": true,
}

// skipDirs are never scanned (Windows system folders, our own updates).
var skipDirs = map[string]bool{
	"system volume information": true, "$recycle.bin": true, "recycler": true,
	"karaokenino-update": true, "lost+found": true,
}

// ScanProgress is reported while scanning.
type ScanProgress struct {
	Files int
	Added int
}

// ScanResult summarizes a scan.
type ScanResult struct {
	Files, Added, Missing, Restored, Renamed int
	MissingSkipped                           bool
	// ReadErrors counts folders that could not be read (disk unplugged,
	// I/O errors). With any, nothing is marked missing.
	ReadErrors int
	Took       time.Duration
}

// ScanOptions tune a scan. The zero value is what the rockola uses.
type ScanOptions struct {
	Progress func(ScanProgress)
	// Busy reports whether a song is playing. The video is read from the
	// same slow USB disk the scan walks, so the scan then leaves the disk
	// idle two thirds of the time. nil = assume something may be playing.
	Busy func() bool
	// Fast turns pacing off (importcatalog on a PC, tests).
	Fast bool
	// BatchSize is how many new or renamed songs are written per
	// transaction (default 2000): memory stays flat, the screen sees new
	// songs as they are found, plays are recorded between batches, and an
	// interrupted scan keeps what it already wrote.
	BatchSize int
}

// Scan walks the library root and syncs the catalog with what is on disk:
//   - new files are added (folder = artist; the genre comes from sibling
//     folders already in the catalog, else from the top-level folder name);
//   - files that disappeared are hidden (missing=1), and shown again if they return;
//   - case differences from Windows paths are fixed in the catalog.
//
// To add songs: copy "Genre/Artist/Song.mp4" onto the disk and restart, or
// use "Re-escanear disco" in the admin menu.
func Scan(ctx context.Context, store *Store, root string, progress func(ScanProgress)) (ScanResult, error) {
	return ScanWith(ctx, store, root, ScanOptions{Progress: progress})
}

// ScanWith is Scan with options.
//
// Cost on a multi-TB disk: media files are never opened or stat'ed; each
// folder is read once and its names compared with the catalog. Memory is
// ~50 bytes per known song (hashes, not paths) plus one batch of new songs.
// Nothing is marked missing unless the whole tree was read without errors,
// so unplugging the disk mid-scan never hides the catalog. On Linux the
// walk runs on a thread with low CPU and I/O priority.
func ScanWith(ctx context.Context, store *Store, root string, opts ScanOptions) (ScanResult, error) {
	start := time.Now()
	if opts.BatchSize <= 0 {
		opts.BatchSize = 2000
	}
	known, err := loadKnown(ctx, store)
	if err != nil {
		return ScanResult{}, err
	}
	sc := &scanner{
		ctx: ctx, store: store, root: root, opts: opts, known: known,
		seen:     make([]uint64, known.maxID/64+1),
		cdgBases: map[string]bool{},
		genreIDs: map[string]int64{},
		pace:     pacer{busy: opts.Busy, off: opts.Fast, last: time.Now()},
	}
	err = runLowPriority(func() error {
		if err := filepath.WalkDir(root, sc.visit); err != nil {
			return err
		}
		return sc.flush()
	})
	if err != nil {
		sc.res.Took = time.Since(start)
		return sc.res, err
	}
	if !readable(root) {
		sc.res.ReadErrors++
	}

	// A wrong, half-mounted or unplugged disk would hide the whole catalog:
	// only mark files missing when the walk read everything and found most
	// of the known catalog.
	switch {
	case sc.res.ReadErrors > 0:
		sc.res.MissingSkipped = true
		log.Printf("escaneo: %d carpetas no se pudieron leer en %s; no se marcan faltantes (¿se desconectó el disco?)", sc.res.ReadErrors, root)
	case len(known.tracks) > 200 && sc.seenCount < len(known.tracks)/5:
		sc.res.MissingSkipped = true
		log.Printf("escaneo: solo %d de %d canciones conocidas están en %s; no se marcan faltantes (¿disco equivocado?)", sc.seenCount, len(known.tracks), root)
	}
	err = store.WithTx(ctx, func(tx *Store) error {
		if !sc.res.MissingSkipped {
			if err := sc.syncMissing(tx); err != nil {
				return err
			}
		}
		if sc.res.Missing+sc.res.Restored > 0 {
			if err := tx.RefreshCounts(ctx); err != nil {
				return err
			}
		}
		return tx.SetMeta(ctx, "last_scan", time.Now().Format(time.RFC3339))
	})
	res := sc.res
	if err == nil && res.Added+res.Missing+res.Restored+res.Renamed > 0 {
		_ = store.Optimize(ctx)
	}
	res.Took = time.Since(start)
	return res, err
}

type found struct{ rel, name, ext string }

type rename struct {
	id  int64
	rel string
}

type scanner struct {
	ctx   context.Context
	store *Store
	root  string
	opts  ScanOptions
	known *knownCatalog
	res   ScanResult

	seen      []uint64 // one bit per track id found on disk
	seenCount int
	pending   []found
	renames   []rename
	cdgBases  map[string]bool // "folder/name" of .cdg files, to skip their .mp3 twin
	genreIDs  map[string]int64
	pace      pacer
}

func (sc *scanner) visit(p string, d fs.DirEntry, err error) error {
	if err != nil {
		log.Printf("escaneo: %v", err)
		if !errors.Is(err, fs.ErrPermission) {
			sc.res.ReadErrors++
		}
		if d != nil && d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if ctxErr := sc.ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	name := d.Name()
	if d.IsDir() {
		if p != sc.root && (strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)]) {
			return fs.SkipDir
		}
		// About to read this folder: give the disk back to the video first.
		return sc.pace.pause(sc.ctx)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !MediaExts[ext] || strings.HasPrefix(name, ".") {
		return nil
	}
	rel, err := filepath.Rel(sc.root, p)
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	sc.res.Files++
	if sc.opts.Progress != nil && sc.res.Files%2000 == 0 {
		sc.opts.Progress(ScanProgress{Files: sc.res.Files, Added: sc.res.Added + len(sc.pending)})
	}
	if ext == ".cdg" {
		sc.cdgBases[strings.ToLower(strings.TrimSuffix(rel, filepath.Ext(rel)))] = true
	}
	if t, ok := sc.known.tracks[hashLower(rel)]; ok {
		if w, bit := t.id/64, uint64(1)<<(t.id%64); sc.seen[w]&bit == 0 {
			sc.seen[w] |= bit
			sc.seenCount++
		}
		if t.exact != hashExact(rel) {
			sc.renames = append(sc.renames, rename{t.id, rel})
		}
	} else {
		sc.pending = append(sc.pending, found{rel: rel, name: strings.TrimSuffix(name, filepath.Ext(name)), ext: ext})
	}
	if len(sc.pending)+len(sc.renames) >= sc.opts.BatchSize {
		return sc.flush()
	}
	return nil
}

// flush writes the pending new songs and case fixes in one transaction.
func (sc *scanner) flush() error {
	if len(sc.pending) == 0 && len(sc.renames) == 0 {
		return nil
	}
	ctx, k := sc.ctx, sc.known
	added, renamed := 0, 0
	err := sc.store.WithTx(ctx, func(tx *Store) error {
		// file_key re-checks the match (the scanner compared hashes).
		upd, err := tx.stmt(ctx, `UPDATE tracks SET file=? WHERE id=? AND file_key=?`)
		if err != nil {
			return err
		}
		for _, r := range sc.renames {
			res, err := upd.ExecContext(ctx, r.rel, r.id, strings.ToLower(r.rel))
			if err != nil {
				// Another row already has the exact spelling: keep the old one.
				log.Printf("escaneo: no se pudo renombrar %q: %v", r.rel, err)
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				renamed++
			}
		}
		touched := map[int64]bool{}
		for _, f := range sc.pending {
			if f.ext == ".mp3" && sc.cdgBases[strings.ToLower(strings.TrimSuffix(f.rel, path.Ext(f.rel)))] {
				continue // audio of a CDG karaoke; played together with the .cdg
			}
			folder := path.Dir(f.rel)
			artistID, artistName, err := k.artistFor(ctx, tx, folder, sc.genreIDs)
			if err != nil {
				return err
			}
			mediaType := strings.ToUpper(strings.TrimPrefix(f.ext, "."))
			genre := k.genreName[k.artistGenre[artistID]]
			id, err := tx.UpsertTrack(ctx, NewTrack{
				ArtistID:     artistID,
				FolderArtist: artistName,
				Name:         f.name,
				File:         f.rel,
				MediaType:    mediaType,
				Karaoke:      isKaraokeGenre(genre) || isKaraokeType(mediaType),
				Source:       "scan",
			})
			if err != nil {
				return err
			}
			if err := tx.indexSearch(ctx, id); err != nil {
				return err
			}
			touched[artistID] = true
			added++
		}
		return tx.refreshArtistCounts(ctx, touched)
	})
	if err != nil {
		return err
	}
	sc.res.Added += added
	sc.res.Renamed += renamed
	sc.pending = sc.pending[:0]
	sc.renames = sc.renames[:0]
	return nil
}

// syncMissing hides known songs that were not found and shows again the
// ones that came back.
func (sc *scanner) syncMissing(tx *Store) error {
	ctx := sc.ctx
	hide, err := tx.stmt(ctx, `UPDATE tracks SET missing=1, missing_since=? WHERE id=?`)
	if err != nil {
		return err
	}
	show, err := tx.stmt(ctx, `UPDATE tracks SET missing=0, missing_since=0 WHERE id=?`)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, t := range sc.known.tracks {
		seen := sc.seen[t.id/64]&(uint64(1)<<(t.id%64)) != 0
		switch {
		case !seen && !t.missing:
			if _, err := hide.ExecContext(ctx, now, t.id); err != nil {
				return err
			}
			sc.res.Missing++
		case seen && t.missing:
			if _, err := show.ExecContext(ctx, t.id); err != nil {
				return err
			}
			sc.res.Restored++
		}
	}
	return nil
}

// pacer spaces out folder reads so the scan never saturates the disk the
// video plays from: after each stretch of work it sleeps a fraction of it.
type pacer struct {
	busy func() bool
	off  bool
	last time.Time
}

func (p *pacer) pause(ctx context.Context) error {
	if p.off {
		return nil
	}
	work := time.Since(p.last)
	factor := 1.0 // not told: a song may be playing (disk busy half the time)
	if p.busy != nil {
		factor = 0.25 // nothing playing: ~80% busy
		if p.busy() {
			factor = 2 // playing: ~33% busy
		}
	}
	if d := min(time.Duration(float64(work)*factor), 250*time.Millisecond); d >= time.Millisecond {
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	p.last = time.Now()
	return nil
}

// readable reports whether the root can still be listed (the disk is there).
func readable(root string) bool {
	f, err := os.Open(root)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == nil
}

// knownTrack is a catalog song as the scanner remembers it. Paths are kept
// as 64-bit hashes: 1M full paths took ~500 MB of the rockola's 2 GB.
type knownTrack struct {
	id      int64
	exact   uint64 // hash of the stored spelling, to fix case differences
	missing bool
}

type knownCatalog struct {
	tracks      map[uint64]knownTrack // hashLower(file) -> track
	maxID       int64
	artists     map[string]int64 // lower(folder) -> artist id
	artistNames map[int64]string
	artistGenre map[int64]int64
	parentGenre map[string]int64 // lower(parent folder) -> genre id
	genreName   map[int64]string
}

func loadKnown(ctx context.Context, s *Store) (*knownCatalog, error) {
	k := &knownCatalog{
		artists:     map[string]int64{},
		artistNames: map[int64]string{},
		artistGenre: map[int64]int64{},
		parentGenre: map[string]int64{},
		genreName:   map[int64]string{},
	}
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*), IFNULL(MAX(id),0) FROM tracks`).Scan(&n, &k.maxID); err != nil {
		return nil, err
	}
	k.tracks = make(map[uint64]knownTrack, n)
	rows, err := s.q.QueryContext(ctx, `SELECT id, file, missing FROM tracks`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t knownTrack
		var file sql.RawBytes
		var m int
		if err := rows.Scan(&t.id, &file, &m); err != nil {
			rows.Close()
			return nil, err
		}
		t.missing = m == 1
		t.exact = hashExactBytes(file)
		k.tracks[hashLowerBytes(file)] = t
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.q.QueryContext(ctx, `SELECT a.id, a.folder, a.name, a.genre_id, g.name FROM artists a JOIN genres g ON g.id=a.genre_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, gid int64
		var folder, name, gname string
		if err := rows.Scan(&id, &folder, &name, &gid, &gname); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSuffix(folder, "/"))
		k.artists[key] = id
		k.artistNames[id] = name
		k.artistGenre[id] = gid
		k.genreName[gid] = gname
		if parent := path.Dir(key); parent != "." {
			if _, ok := k.parentGenre[parent]; !ok {
				k.parentGenre[parent] = gid
			}
		}
	}
	return k, rows.Err()
}

// FNV-1a of a path, exact or lowercased rune by rune, without allocating.
// A collision (odds ~1 in 10^7 with 1M songs) would make one new file look
// known; renames are re-checked against file_key before writing.
const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func hashExact(s string) uint64 {
	h := uint64(fnvOffset)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * fnvPrime
	}
	return h
}

func hashExactBytes(b []byte) uint64 {
	h := uint64(fnvOffset)
	for _, c := range b {
		h = (h ^ uint64(c)) * fnvPrime
	}
	return h
}

func hashLower(s string) uint64 {
	h := uint64(fnvOffset)
	for _, r := range s {
		h = hashRune(h, r)
	}
	return h
}

func hashLowerBytes(b []byte) uint64 {
	h := uint64(fnvOffset)
	for _, r := range string(b) { // ranging over string(b) does not copy b
		h = hashRune(h, r)
	}
	return h
}

func hashRune(h uint64, r rune) uint64 {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return (h ^ uint64(r)) * fnvPrime
	}
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], unicode.ToLower(r))
	for _, c := range buf[:n] {
		h = (h ^ uint64(c)) * fnvPrime
	}
	return h
}

// artistFor returns the artist of a folder, creating it (and its genre) if new.
func (k *knownCatalog) artistFor(ctx context.Context, tx *Store, folder string, genreIDs map[string]int64) (int64, string, error) {
	key := strings.ToLower(folder)
	if id, ok := k.artists[key]; ok {
		return id, k.artistNames[id], nil
	}
	parts := strings.Split(folder, "/")
	name := parts[len(parts)-1]
	var genreID int64
	if folder == "." {
		name = "Varios"
	}
	if gid, ok := k.parentGenre[strings.ToLower(path.Dir(folder))]; ok && folder != "." {
		genreID = gid
	} else {
		genre := "OTROS"
		if folder != "." {
			genre = strings.ToUpper(parts[0])
		}
		if id, ok := genreIDs[genre]; ok {
			genreID = id
		} else {
			id, err := tx.UpsertGenre(ctx, genre)
			if err != nil {
				return 0, "", err
			}
			genreIDs[genre] = id
			genreID = id
			k.genreName[id] = genre
		}
	}
	id, err := tx.UpsertArtist(ctx, genreID, name, folder, "")
	if err != nil {
		return 0, "", err
	}
	k.artists[key] = id
	k.artistNames[id] = name
	k.artistGenre[id] = genreID
	return id, name, nil
}

// CoverCandidates are the cover file names looked for in an artist folder.
var CoverCandidates = []string{"images.jpg", "cover.jpg", "folder.jpg", "picture.jpg", "portada.jpg", "cover.png", "folder.png", "picture.png", "picture.bmp"}
