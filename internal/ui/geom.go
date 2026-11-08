package ui

// The screen is designed at 1280×720 and scaled to any TV (scaledLayout).
const (
	designW = 1280
	designH = 720
)

// Rect is a rectangle in design pixels.
type Rect struct {
	Left, Top, Width, Height int
}

func (r Rect) X2() int     { return r.Left + r.Width }
func (r Rect) Y2() int     { return r.Top + r.Height }
func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

func grow(r Rect, d int) Rect {
	return Rect{Left: r.Left - d, Top: r.Top - d, Width: r.Width + 2*d, Height: r.Height + 2*d}
}
