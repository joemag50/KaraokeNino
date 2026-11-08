package catalog

import "testing"

func TestSortKeyAndLetter(t *testing.T) {
	cases := map[string]string{
		"Ángel Xan":            "angel xan",
		"# Seguro cantas una":  "seguro cantas una",
		"$ Maluma":             "maluma",
		"  ZOÉ  ":              "zoe",
		"1000 Karaokes":        "1000 karaokes",
		"Ñengo Flow":           "nengo flow",
		"juan  gabriel  MARIA": "juan gabriel maria",
	}
	for in, want := range cases {
		if got := SortKey(in); got != want {
			t.Errorf("SortKey(%q)=%q, quería %q", in, got, want)
		}
	}
	if LetterOf("angel") != "a" || LetterOf("1000") != "#" || LetterOf("") != "#" {
		t.Error("LetterOf")
	}
}

func TestParseTitle(t *testing.T) {
	cases := []struct{ folder, name, title, artist string }{
		{"Juan Gabriel", "Amor Eterno", "Amor Eterno", ""},
		{"# Seguro cantas una", "25 rosas - Joan Sebastian", "25 rosas", "Joan Sebastian"},
		{"$ Maluma", "Corazón Ft Nego Do Borel", "Corazón Ft Nego Do Borel", ""},
		{"Shakira", "Shakira - Antologia", "Antologia", ""},
		{"SHAKIRA", "Antologia - shakira", "Antologia", ""},
		{"Medley", "I will survive, Funky town", "I will survive, Funky town", ""},
		{"Los Tigres", "Jefe de jefes - en vivo", "Jefe de jefes - en vivo", ""},
		{"Exitos Banda", "El Sinaloense - Banda El Recodo", "El Sinaloense", "Banda El Recodo"},
	}
	for _, c := range cases {
		title, artist := ParseTitle(c.folder, c.name)
		if title != c.title || artist != c.artist {
			t.Errorf("ParseTitle(%q,%q)=(%q,%q), quería (%q,%q)", c.folder, c.name, title, artist, c.title, c.artist)
		}
	}
}
