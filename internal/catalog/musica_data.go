package catalog

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Musica.data format (Windows-1252 / Latin-1 text, CRLF):
//
//	GENRE
//	\tArtist|D:\folder\|D:\folder\images.jpg
//	\t\tD:\folder\|File name without extension|TYPE
//
// The real file on disk is folder + name + "." + type. The name is kept verbatim:
// some files end with a space before the extension.

// EntryKind identifies what a Musica.data line describes.
type EntryKind int

const (
	EntryGenre EntryKind = iota
	EntryArtist
	EntryTrack
)

// Entry is one parsed line of Musica.data. Paths are still in Windows form.
type Entry struct {
	Kind   EntryKind
	Line   int
	Name   string // genre name, artist name or track title (trimmed, for display)
	Base   string // track file name without extension, exactly as on disk
	Folder string // artist / track folder (Windows path, trailing backslash)
	Image  string // artist cover (Windows path), may be empty
	Type   string // track media type, upper case (AVI, MP3...)
}

// File returns the Windows path of a track: folder + base name + "." + lowercase type.
func (e Entry) File() string {
	folder := e.Folder
	if folder != "" && !strings.HasSuffix(folder, `\`) && !strings.HasSuffix(folder, "/") {
		folder += `\`
	}
	return folder + e.Base + "." + strings.ToLower(e.Type)
}

// ParseMusicaData decodes r as Windows-1252 and calls fn for every genre,
// artist and track line. Malformed lines are reported through skip (may be nil).
func ParseMusicaData(r io.Reader, fn func(Entry) error, skip func(line int, text, reason string)) error {
	sc := bufio.NewScanner(charmap.Windows1252.NewDecoder().Reader(r))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	if skip == nil {
		skip = func(int, string, string) {}
	}

	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "\t\t"):
			parts := strings.Split(strings.TrimPrefix(line, "\t\t"), "|")
			if len(parts) < 3 {
				skip(n, line, "pista con menos de 3 campos")
				continue
			}
			e := Entry{
				Kind:   EntryTrack,
				Line:   n,
				Folder: strings.TrimSpace(parts[0]),
				Name:   strings.TrimSpace(parts[1]),
				Base:   parts[1], // file names may end with a space ("Tu Y Yo .mpg")
				Type:   strings.ToUpper(strings.TrimSpace(parts[2])),
			}
			if e.Name == "" || e.Type == "" {
				skip(n, line, "pista sin nombre o sin tipo")
				continue
			}
			if err := fn(e); err != nil {
				return err
			}
		case strings.HasPrefix(line, "\t"):
			parts := strings.Split(strings.TrimPrefix(line, "\t"), "|")
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				skip(n, line, "artista sin carpeta")
				continue
			}
			e := Entry{
				Kind:   EntryArtist,
				Line:   n,
				Name:   strings.TrimSpace(parts[0]),
				Folder: strings.TrimSpace(parts[1]),
			}
			if len(parts) >= 3 {
				e.Image = strings.TrimSpace(parts[2])
			}
			if err := fn(e); err != nil {
				return err
			}
		default:
			if err := fn(Entry{Kind: EntryGenre, Line: n, Name: strings.TrimSpace(line)}); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}
