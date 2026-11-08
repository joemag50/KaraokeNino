// Package maint runs the rockola's maintenance tasks: catalog health check,
// planner statistics, search index, play history, songs missing from the
// disk, orphan thumbnails, logs and free disk space. `importcatalog
// -mantenimiento` runs it; the admin menu can call Run (or each step) too.
// What to run and when: docs/MANTENIMIENTO.md.
package maint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/logx"
)

// Options choose the tasks. The zero value plus the folders is the safe
// routine: check, optimize, report missing songs, clean thumbnails and logs,
// check free space. Nothing in it deletes songs or history.
type Options struct {
	ThumbsDir string // data/thumbs ("" = skip thumbnails)
	LogDir    string // data/logs ("" = skip logs)
	MusicRoot string // music disk, for its free space ("" = skip)

	// Full runs PRAGMA integrity_check and the search index check instead
	// of quick_check (minutes on the rockola instead of seconds).
	Full bool
	// PrunePlaysMonths folds plays older than this into the all-time totals
	// (0 = keep all). "Lo más pedido" of all time does not change.
	PrunePlaysMonths int
	// MissingDays: songs hidden for longer than this are reported (default 30).
	MissingDays int
	// PurgeMissing deletes those songs from the catalog.
	PurgeMissing bool
	// Vacuum rewrites the catalog file to give back free space. Blocks
	// plays while it runs: only with the rockola idle.
	Vacuum bool
	// RebuildSearch rebuilds the full-text index from scratch.
	RebuildSearch bool
	// DryRun reports what would be done without changing anything.
	DryRun bool
	// LogMaxAge deletes rotated logs older than this (default 90 days).
	LogMaxAge time.Duration
}

// Report says what was done, in Spanish, ready for the screen or a terminal.
type Report struct {
	Lines    []string // one line per task
	Problems []string // things the owner should look at
}

func (r *Report) add(format string, args ...any) {
	r.Lines = append(r.Lines, fmt.Sprintf(format, args...))
}
func (r *Report) warn(format string, args ...any) {
	r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
}

// Free space below this on any disk is reported as a problem.
const (
	lowFreeBytes   = 1 << 30 // 1 GB
	lowFreePercent = 5
)

// Run runs the tasks in opts, in a safe order: a damaged catalog stops
// everything that writes.
func Run(ctx context.Context, store *catalog.Store, opts Options) (Report, error) {
	var r Report
	if opts.MissingDays <= 0 {
		opts.MissingDays = 30
	}
	if opts.LogMaxAge <= 0 {
		opts.LogMaxAge = 90 * 24 * time.Hour
	}
	now := time.Now()

	// 1. Health.
	var problems []string
	var err error
	if opts.Full {
		problems, err = store.IntegrityCheck(ctx)
	} else {
		problems, err = store.QuickCheck(ctx)
	}
	if err != nil {
		return r, fmt.Errorf("revisión del catálogo: %w", err)
	}
	if len(problems) > 0 {
		for i, p := range problems {
			if i == 5 {
				r.warn("… y %d problemas más", len(problems)-5)
				break
			}
			r.warn("catálogo dañado: %s", p)
		}
		r.warn("no se cambió nada: restaura data/catalog.db.prev, copia un catálogo bueno o vuelve a importar (docs/MANTENIMIENTO.md)")
		return r, nil
	}
	r.add("Catálogo sano (%s)", map[bool]string{true: "integrity_check + índice de búsqueda", false: "quick_check"}[opts.Full])

	// 2. Play history.
	if opts.PrunePlaysMonths > 0 {
		cut := now.AddDate(0, -opts.PrunePlaysMonths, 0)
		if opts.DryRun {
			n, err := store.PlaysOlderThan(ctx, cut)
			if err != nil {
				return r, err
			}
			r.add("Historial: se juntarían %d reproducciones anteriores a %s", n, cut.Format("2006-01-02"))
		} else {
			n, err := store.PrunePlays(ctx, cut)
			if err != nil {
				return r, fmt.Errorf("historial: %w", err)
			}
			r.add("Historial: %d reproducciones anteriores a %s pasaron al total (lo más pedido no cambia)", n, cut.Format("2006-01-02"))
		}
	}

	// 3. Songs missing from the disk.
	cut := now.AddDate(0, 0, -opts.MissingDays)
	old, err := store.MissingTracks(ctx, cut, 100000)
	if err != nil {
		return r, err
	}
	all, err := store.MissingTracks(ctx, time.Time{}, 100000)
	if err != nil {
		return r, err
	}
	switch {
	case len(old) == 0:
		r.add("Faltantes: %d canciones ocultas, ninguna desde hace más de %d días", len(all), opts.MissingDays)
	case opts.PurgeMissing && !opts.DryRun:
		n, err := store.PurgeMissing(ctx, cut)
		if err != nil {
			return r, fmt.Errorf("faltantes: %w", err)
		}
		r.add("Faltantes: se borraron %d canciones que no están en el disco desde hace más de %d días", n, opts.MissingDays)
	default:
		r.add("Faltantes: %d canciones no están en el disco desde hace más de %d días (importcatalog -faltantes las lista; -purgar-faltantes las borra)", len(old), opts.MissingDays)
	}

	// 4. Search index and planner statistics.
	if !opts.DryRun {
		if opts.RebuildSearch {
			if err := store.RebuildSearchIndex(ctx); err != nil {
				return r, fmt.Errorf("índice de búsqueda: %w", err)
			}
			r.add("Índice de búsqueda reconstruido")
		} else if err := store.OptimizeSearch(ctx); err != nil {
			return r, fmt.Errorf("índice de búsqueda: %w", err)
		} else {
			r.add("Índice de búsqueda compactado")
		}
		if err := store.Analyze(ctx); err != nil {
			return r, fmt.Errorf("estadísticas: %w", err)
		}
		r.add("Estadísticas de consultas actualizadas (ANALYZE)")
	}

	// 5. Catalog file size.
	total, free, err := store.FileSizes(ctx)
	if err == nil {
		r.add("Catálogo: %s (%s libres dentro del archivo)", size(total), size(free))
	}
	if opts.Vacuum && !opts.DryRun {
		avail, _, derr := DiskFree(filepath.Dir(store.Path()))
		if derr == nil && avail < uint64(total)*2 {
			r.warn("VACUUM no se hizo: necesita %s libres junto al catálogo y hay %s", size(total*2), size(int64(avail)))
		} else if err := store.Vacuum(ctx); err != nil {
			return r, fmt.Errorf("vacuum: %w", err)
		} else {
			after, _, _ := store.FileSizes(ctx)
			r.add("VACUUM: el catálogo pasó de %s a %s", size(total), size(after))
		}
	} else if total > 0 && free*4 > total {
		r.add("Sugerencia: más de una cuarta parte del catálogo está libre; -vacuum lo achica (con la rockola sin uso)")
	}

	// 6. Thumbnails.
	if opts.ThumbsDir != "" {
		res, err := CleanThumbs(ctx, store, opts.ThumbsDir, opts.DryRun)
		switch {
		case err != nil:
			r.warn("miniaturas: %v", err)
		case opts.DryRun:
			r.add("Miniaturas: %d de %d sin artista (%s) se borrarían", res.Removed, res.Files, size(res.Freed))
		default:
			r.add("Miniaturas: %d de %d sin artista borradas (%s)", res.Removed, res.Files, size(res.Freed))
		}
	}

	// 7. Logs.
	if opts.LogDir != "" {
		removed, freed, err := logx.Tidy(opts.LogDir, opts.LogMaxAge, 8<<20, opts.DryRun)
		if err != nil {
			r.warn("registros: %v", err)
		} else {
			r.add("Registros: %d archivos viejos o recortados (%s)", removed, size(freed))
		}
	}

	// 8. Old backups (importcatalog -respaldo auto).
	if n, freed, err := PruneBackups(BackupDir(store.Path()), KeepBackups, opts.DryRun); err != nil {
		r.warn("respaldos: %v", err)
	} else if n > 0 {
		r.add("Respaldos: %d viejos borrados (%s); quedan los %d más nuevos", n, size(freed), KeepBackups)
	}

	// 9. Free space.
	for _, d := range []struct{ name, dir string }{
		{"Disco del sistema (catálogo)", filepath.Dir(store.Path())},
		{"Disco de música", opts.MusicRoot},
	} {
		if d.dir == "" {
			continue
		}
		avail, tot, err := DiskFree(d.dir)
		if err != nil {
			r.warn("%s: %v", d.name, err)
			continue
		}
		r.add("%s: %s libres de %s", d.name, size(int64(avail)), size(int64(tot)))
		if avail < lowFreeBytes || (tot > 0 && avail*100/tot < lowFreePercent) {
			r.warn("%s casi lleno: %s libres", d.name, size(int64(avail)))
		}
	}
	return r, nil
}

// KeepBackups is how many catalog backups PruneBackups leaves.
const KeepBackups = 5

// BackupDir is where `importcatalog -respaldo auto` writes: data/respaldos.
func BackupDir(catalogDB string) string {
	return filepath.Join(filepath.Dir(catalogDB), "respaldos")
}

// PruneBackups deletes all but the newest keep "catalog-*.db" in dir.
func PruneBackups(dir string, keep int, dryRun bool) (n int, freed int64, err error) {
	matches, err := filepath.Glob(filepath.Join(dir, "catalog-*.db"))
	if err != nil || len(matches) <= keep {
		return 0, 0, err
	}
	sort.Strings(matches) // catalog-YYYYMMDD-HHMM.db: oldest first
	for _, p := range matches[:len(matches)-keep] {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !dryRun && os.Remove(p) != nil {
			continue
		}
		n++
		freed += st.Size()
	}
	return n, freed, nil
}

// ThumbsResult is what CleanThumbs found.
type ThumbsResult struct {
	Files   int   // thumbnails in the folder
	Removed int   // deleted (or that would be, with dryRun)
	Freed   int64 // bytes
}

// CleanThumbs deletes thumbnails of artists that are no longer in the
// catalog, and temporary files left by an interrupted thumbnail. If almost
// none of the thumbnails match an artist, it refuses (the naming may have
// changed in a newer version) instead of emptying the folder.
func CleanThumbs(ctx context.Context, store *catalog.Store, dir string, dryRun bool) (ThumbsResult, error) {
	var res ThumbsResult
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	want, err := store.ThumbNames(ctx)
	if err != nil {
		return res, err
	}
	var orphans []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir():
		case catalog.IsThumbName(name):
			res.Files++
			if !want[name] {
				orphans = append(orphans, e)
			}
		case strings.HasPrefix(name, ".thumb-"):
			// Left by a thumbnail interrupted by a power cut.
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
				orphans = append(orphans, e)
			}
		}
	}
	if res.Files >= 50 && (res.Files-len(orphans))*10 < res.Files {
		return res, fmt.Errorf("solo %d de %d miniaturas corresponden a un artista; no se borra nada (¿cambió el nombre de las miniaturas?)",
			res.Files-len(orphans), res.Files)
	}
	for _, e := range orphans {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !dryRun {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				continue
			}
		}
		res.Removed++
		res.Freed += info.Size()
	}
	return res, nil
}

func size(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%d KB", b>>10)
	}
	return fmt.Sprintf("%d B", b)
}
