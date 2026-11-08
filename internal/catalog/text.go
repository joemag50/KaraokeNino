package catalog

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fold lowercases s and removes accents: "Ángel Zoé" -> "angel zoe".
func Fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

// SortKey orders names like a person would: accents and case ignored, and
// leading symbols used by playlist folders ("# Seguro cantas una",
// "$ Maluma") skipped.
func SortKey(s string) string {
	f := strings.TrimLeftFunc(Fold(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(strings.Fields(f), " ")
}

// LetterOf returns the ABC bucket of a sort key: "a".."z", or "#" for digits/symbols.
func LetterOf(sortKey string) string {
	for _, r := range sortKey {
		if r >= 'a' && r <= 'z' {
			return string(r)
		}
		if r == 'ñ' {
			return "n"
		}
		return "#"
	}
	return "#"
}

// ParseTitle splits a file name into title and singer.
//
// Files in artist folders are usually just the title ("Amor Eterno"), but
// playlist/compilation folders hold "Title - Artist" and some artists'
// folders hold "Artist - Title". The returned artist is "" when the song is
// by the folder's artist.
func ParseTitle(folderArtist, name string) (title, artist string) {
	name = strings.Join(strings.Fields(strings.ReplaceAll(name, "_", " ")), " ")
	left, right, ok := strings.Cut(name, " - ")
	if !ok {
		return name, ""
	}
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return name, ""
	}
	fa := SortKey(folderArtist)
	switch {
	case fa != "" && SortKey(left) == fa:
		return right, ""
	case fa != "" && SortKey(right) == fa:
		return left, ""
	case isCompilation(folderArtist):
		return left, right
	}
	return name, ""
}

// isCompilation reports folders that mix singers: playlist folders start
// with a symbol ("# Seguro cantas una") and mixes say so in the name.
func isCompilation(folder string) bool {
	f := strings.TrimSpace(folder)
	if f == "" {
		return false
	}
	if r := []rune(f)[0]; !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return true
	}
	k := Fold(f)
	for _, w := range []string{"varios", "mix", "exitos", "hits", "top", "lo mejor", "coleccion", "seguro cantas", "karaoke"} {
		if strings.Contains(k, w) {
			return true
		}
	}
	return false
}

// DupKey identifies the same song stored twice ("Amor Del Bueno - Reyli" in two folders).
func DupKey(title, artist string) string {
	return SortKey(title) + "|" + SortKey(artist)
}
