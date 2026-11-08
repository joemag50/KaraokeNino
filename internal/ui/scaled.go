package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// scaledLayout places objects at design coordinates and scales the whole
// design uniformly to the window (letterboxed), so a 1024×768 or 1280×720
// design fills any TV — 4:3 or 16:9, 720p or 1080p — without distortion.
// Text sizes scale too.
type scaledLayout struct {
	dw, dh float32
	items  map[fyne.CanvasObject]placement

	scale        float32
	offX, offY   float32
	onResize     func()
	lastW, lastH float32
}

type placement struct {
	r        Rect
	textSize float32 // design text size for *canvas.Text (0 = not text)
	full     bool    // stretch over the whole window (backgrounds)
}

func newScaledLayout(designW, designH int) *scaledLayout {
	return &scaledLayout{dw: float32(designW), dh: float32(designH), items: map[fyne.CanvasObject]placement{}, scale: 1}
}

// place registers obj at design rect r.
func (l *scaledLayout) place(obj fyne.CanvasObject, r Rect) fyne.CanvasObject {
	l.items[obj] = placement{r: r}
	return obj
}

// placeText registers a text at design rect r with design text size.
func (l *scaledLayout) placeText(t *canvas.Text, r Rect, size float32) *canvas.Text {
	l.items[t] = placement{r: r, textSize: size}
	return t
}

// placeFull stretches obj over the whole window (background gradients).
func (l *scaledLayout) placeFull(obj fyne.CanvasObject) fyne.CanvasObject {
	l.items[obj] = placement{full: true}
	return obj
}

// move changes the design rect of an already placed object.
func (l *scaledLayout) move(obj fyne.CanvasObject, r Rect) {
	p := l.items[obj]
	p.r = r
	p.full = false
	l.items[obj] = p
	l.apply(obj, p)
}

// setFull stretches an already placed object over the whole window.
func (l *scaledLayout) setFull(obj fyne.CanvasObject) {
	p := l.items[obj]
	p.full = true
	l.items[obj] = p
	if l.lastW > 0 {
		obj.Move(fyne.NewPos(0, 0))
		obj.Resize(fyne.NewSize(l.lastW, l.lastH))
	}
}

func (l *scaledLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.scale = min(size.Width/l.dw, size.Height/l.dh)
	l.offX = (size.Width - l.dw*l.scale) / 2
	l.offY = (size.Height - l.dh*l.scale) / 2
	for _, o := range objects {
		p, ok := l.items[o]
		if !ok {
			continue
		}
		if p.full {
			o.Move(fyne.NewPos(0, 0))
			o.Resize(size)
			continue
		}
		l.apply(o, p)
	}
	if (size.Width != l.lastW || size.Height != l.lastH) && l.onResize != nil {
		l.lastW, l.lastH = size.Width, size.Height
		l.onResize()
	}
}

func (l *scaledLayout) apply(o fyne.CanvasObject, p placement) {
	if p.full {
		return
	}
	o.Move(l.pos(p.r))
	o.Resize(l.size(p.r))
	if t, ok := o.(*canvas.Text); ok && p.textSize > 0 {
		t.TextSize = p.textSize * l.scale
	}
}

func (l *scaledLayout) pos(r Rect) fyne.Position {
	return fyne.NewPos(l.offX+float32(r.Left)*l.scale, l.offY+float32(r.Top)*l.scale)
}

func (l *scaledLayout) size(r Rect) fyne.Size {
	return fyne.NewSize(float32(r.Width)*l.scale, float32(r.Height)*l.scale)
}

func (l *scaledLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(320, 240)
}

// screenRect converts a design rect to window coordinates.
func (l *scaledLayout) screenRect(r Rect) (fyne.Position, fyne.Size) {
	return l.pos(r), l.size(r)
}
