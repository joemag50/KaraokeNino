// importcatalog builds and maintains the catalog from the command line:
//
//	importcatalog -musica Musica.data            # import a Musica.data song list
//	importcatalog -scan /media/usb/master.1000   # add new files on the disk
//	importcatalog -dupes                         # list duplicated songs
//	importcatalog -export catalogo.html          # printable song book
//	importcatalog -mantenimiento                 # check and tidy (docs/MANTENIMIENTO.md)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josegamboa/karaokenino/internal/catalog"
	"github.com/josegamboa/karaokenino/internal/config"
	"github.com/josegamboa/karaokenino/internal/library"
	"github.com/josegamboa/karaokenino/internal/maint"
)

func main() {
	cfgPath := flag.String("config", "config/config.yaml", "archivo de configuración")
	musica := flag.String("musica", "", "importar una lista Musica.data")
	karaokeOnly := flag.Bool("karaoke-only", false, "con -musica: solo géneros con KARAOKE en el nombre")
	verbose := flag.Bool("v", false, "mostrar cada línea omitida")
	scan := flag.String("scan", "", "escanear el disco (ruta raíz, o 'auto' para buscarlo)")
	dupes := flag.Bool("dupes", false, "listar canciones duplicadas")
	export := flag.String("export", "", "escribir catálogo imprimible (HTML) en este archivo")
	exportGenres := flag.String("genres", "", "con -export: géneros separados por coma (vacío = todos)")
	title := flag.String("title", "Catálogo de canciones", "con -export: título del catálogo")
	gentle := flag.Bool("suave", false, "con -scan: escanear despacio (si la rockola está tocando del mismo disco)")

	mant := flag.Bool("mantenimiento", false, "revisar y ordenar: catálogo, búsqueda, miniaturas, registros y espacio libre (no borra canciones)")
	full := flag.Bool("revision-completa", false, "con -mantenimiento: revisión completa del catálogo (más lenta)")
	pruneMonths := flag.Int("podar-historial", 0, "con -mantenimiento: juntar en el total las reproducciones de hace más de N meses")
	missingDays := flag.Int("dias-faltantes", 30, "días que una canción debe faltar del disco para reportarla o borrarla")
	purge := flag.Bool("purgar-faltantes", false, "con -mantenimiento: borrar las canciones que faltan hace más de -dias-faltantes")
	vacuum := flag.Bool("vacuum", false, "con -mantenimiento: compactar el archivo del catálogo (con la rockola sin uso)")
	rebuild := flag.Bool("reconstruir-busqueda", false, "con -mantenimiento: reconstruir el índice de búsqueda")
	dryRun := flag.Bool("simular", false, "con -mantenimiento: solo decir qué se haría")
	listMissing := flag.Bool("faltantes", false, "listar las canciones que faltan del disco hace más de -dias-faltantes")
	music := flag.String("disco", "", "con -mantenimiento: ruta del disco de música para ver su espacio libre ('auto' = buscarlo)")
	backup := flag.String("respaldo", "", "copiar el catálogo a este archivo antes de todo ('auto' = data/respaldos/catalog-FECHA.db)")
	flag.Parse()

	if *musica == "" && *scan == "" && !*dupes && *export == "" && !*mant && !*listMissing && *backup == "" {
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	store, err := catalog.Open(cfg.CatalogDB)
	if err != nil {
		log.Fatalf("catálogo: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	exitCode := 0

	if *backup != "" {
		dst := *backup
		if dst == "auto" {
			dir := filepath.Join(filepath.Dir(cfg.CatalogDB), "respaldos")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				log.Fatal(err)
			}
			dst = filepath.Join(dir, "catalog-"+time.Now().Format("20060102-1504")+".db")
		}
		if err := store.Backup(ctx, dst); err != nil {
			log.Fatalf("respaldo: %v", err)
		}
		log.Printf("Respaldo del catálogo en %s", dst)
	}

	if *musica != "" {
		start := time.Now()
		res, err := catalog.ImportMusicaData(ctx, cfg, *musica, store, catalog.ImportOptions{
			KaraokeOnly: *karaokeOnly,
			OnSkip: func(line int, text, reason string) {
				if *verbose {
					log.Printf("omitida línea %d (%s): %q", line, reason, text)
				}
			},
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Importado en %s: %s", time.Since(start).Round(time.Millisecond), res)
	}

	if *scan != "" {
		root := *scan
		if root == "auto" {
			root, err = library.New(cfg.Library).Detect()
			if err != nil {
				log.Fatal(err)
			}
		}
		log.Printf("Escaneando %s…", root)
		res, err := catalog.ScanWith(ctx, store, root, catalog.ScanOptions{
			Fast: !*gentle,
			Progress: func(p catalog.ScanProgress) {
				log.Printf("  %d archivos…", p.Files)
			},
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Escaneo en %s: %d archivos, %d nuevas, %d faltantes, %d recuperadas, %d corregidas",
			res.Took.Round(time.Second), res.Files, res.Added, res.Missing, res.Restored, res.Renamed)
		if res.MissingSkipped {
			log.Printf("Aviso: casi nada del catálogo está en %s; no se marcaron faltantes", root)
		}
	}

	if *dupes {
		groups, err := store.Duplicates(ctx, 100000)
		if err != nil {
			log.Fatal(err)
		}
		for _, g := range groups {
			fmt.Printf("%s — %s\n", g[0].Title, g[0].Artist)
			for _, t := range g {
				fmt.Printf("    #%d  %s\n", t.ID, t.File)
			}
		}
		log.Printf("%d canciones duplicadas", len(groups))
	}

	if *export != "" {
		f, err := os.Create(*export)
		if err != nil {
			log.Fatal(err)
		}
		var genres []string
		if *exportGenres != "" {
			genres = strings.Split(*exportGenres, ",")
		}
		if err := catalog.ExportHTML(ctx, store, f, *title, genres); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Catálogo escrito en %s (ábrelo en el navegador e imprime)", *export)
	}

	if *listMissing {
		cut := time.Now().AddDate(0, 0, -*missingDays)
		ms, err := store.MissingTracks(ctx, cut, 100000)
		if err != nil {
			log.Fatal(err)
		}
		for _, m := range ms {
			since := "?"
			if !m.Since.IsZero() {
				since = m.Since.Format("2006-01-02")
			}
			fmt.Printf("#%d  falta desde %s  %s\n", m.ID, since, m.File)
		}
		log.Printf("%d canciones faltan del disco hace más de %d días", len(ms), *missingDays)
	}

	if *mant {
		root := *music
		if root == "auto" {
			root, _ = library.New(cfg.Library).Detect()
		}
		opts := maint.Options{
			ThumbsDir:        filepath.Join(filepath.Dir(cfg.CatalogDB), "thumbs"),
			MusicRoot:        root,
			Full:             *full,
			PrunePlaysMonths: *pruneMonths,
			MissingDays:      *missingDays,
			PurgeMissing:     *purge,
			Vacuum:           *vacuum,
			RebuildSearch:    *rebuild,
			DryRun:           *dryRun,
		}
		if cfg.LogFile != "" {
			opts.LogDir = filepath.Dir(cfg.LogFile)
		}
		start := time.Now()
		rep, err := maint.Run(ctx, store, opts)
		for _, l := range rep.Lines {
			fmt.Println("  ✓ " + l)
		}
		for _, p := range rep.Problems {
			fmt.Println("  ! " + p)
		}
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Mantenimiento en %s", time.Since(start).Round(time.Millisecond))
		if len(rep.Problems) > 0 {
			exitCode = 1 // scripts can tell something needs a look
		}
	}

	st, _ := store.Stats(ctx)
	log.Printf("Catálogo %s: %s", cfg.CatalogDB, st)
	if exitCode != 0 {
		store.Close()
		os.Exit(exitCode)
	}
}
