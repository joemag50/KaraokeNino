package player

import "github.com/josegamboa/karaokenino/internal/library"

// ResolvePath finds a catalog file on disk ignoring case (see library.Resolve).
func ResolvePath(p string) (string, error) { return library.Resolve(p) }
