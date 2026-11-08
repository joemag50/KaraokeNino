package catalog

import (
	"bufio"
	"context"
	"html"
	"io"
	"strings"
	"time"
)

// ExportHTML writes a printable song book: genre → artist → "number  title".
// Guests dial the number on the keypad (0, number, Intro).
func ExportHTML(ctx context.Context, s *Store, w io.Writer, title string, genres []string) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	esc := html.EscapeString

	bw.WriteString(`<!doctype html><html lang="es"><head><meta charset="utf-8"><title>` + esc(title) + `</title>
<style>
body{font-family:Arial,Helvetica,sans-serif;font-size:10pt;margin:1cm;color:#111}
h1{font-size:20pt;margin:0 0 .2cm}
.meta{color:#555;margin-bottom:.6cm}
h2{font-size:15pt;background:#222;color:#fff;padding:.15cm .3cm;page-break-before:always;margin-top:0}
h2:first-of-type{page-break-before:auto}
.cols{column-count:3;column-gap:.8cm}
h3{font-size:10.5pt;margin:.3cm 0 .05cm;border-bottom:1px solid #999;break-after:avoid}
p{margin:0;break-inside:avoid}
b{display:inline-block;min-width:3.6em;font-variant-numeric:tabular-nums}
i{color:#555}
@media print{body{margin:0}}
</style></head><body>
<h1>` + esc(title) + `</h1>
<div class="meta">Para pedir una canción: <b>0</b>, el número y <b>Intro</b>. Generado ` + time.Now().Format("02/01/2006") + `</div>
`)

	want := map[string]bool{}
	for _, g := range genres {
		want[strings.ToUpper(g)] = true
	}
	all, err := s.Genres(ctx, KindAll)
	if err != nil {
		return err
	}
	for _, g := range all {
		if len(want) > 0 && !want[strings.ToUpper(g.Name)] {
			continue
		}
		bw.WriteString(`<h2>` + esc(g.Name) + `</h2><div class="cols">`)
		rows, err := s.q.QueryContext(ctx, `
SELECT a.name, t.id, t.title, t.track_artist FROM tracks t JOIN artists a ON a.id=t.artist_id
WHERE a.genre_id=? AND t.missing=0 ORDER BY a.sort_key, a.id, t.sort_key, t.id`, g.ID)
		if err != nil {
			return err
		}
		last := "\x00"
		for rows.Next() {
			var artist, title, trackArtist string
			var id int64
			if err := rows.Scan(&artist, &id, &title, &trackArtist); err != nil {
				rows.Close()
				return err
			}
			if artist != last {
				bw.WriteString(`<h3>` + esc(artist) + `</h3>`)
				last = artist
			}
			line := `<p><b>` + itoa(id) + `</b>` + esc(title)
			if trackArtist != "" {
				line += ` <i>— ` + esc(trackArtist) + `</i>`
			}
			bw.WriteString(line + "</p>\n")
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		bw.WriteString(`</div>`)
	}
	_, err = bw.WriteString(`</body></html>`)
	return err
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
