package ui

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// A reused row must show the new artist's cover, not the one it showed
// before (rows are recycled while scrolling, and thumbnails arrive later).
func TestListRowShowsNewCover(t *testing.T) {
	test.NewTempApp(t)
	dir := t.TempDir()
	write := func(name string, c color.NRGBA) string {
		img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := jpeg.Encode(f, img, nil); err != nil {
			t.Fatal(err)
		}
		return p
	}
	red := write("rojo.jpg", color.NRGBA{255, 0, 0, 255})
	blue := write("azul.jpg", color.NRGBA{0, 0, 255, 255})

	pal := PaletteFor("moderno")
	lay := newScaledLayout(400, 100)
	l := newList(lay, &pal, Rect{0, 0, 400, 100}, 80, 16, 0, 64, 0)
	w := test.NewWindow(container.New(lay, l.objects()...))
	defer w.Close()
	w.Resize(fyne.NewSize(400, 100))

	icon := ""
	l.iconFn = func(int) string { return icon }
	sample := func() color.NRGBA {
		img := w.Canvas().Capture()
		// center of the icon square: x 6..70, row 0..80
		return color.NRGBAModel.Convert(img.At(img.Bounds().Dx()*38/400, img.Bounds().Dy()*40/100)).(color.NRGBA)
	}

	icon = red
	l.set([]rowItem{{Text: "Artista"}}, 0)
	if c := sample(); c.R < 180 || c.B > 80 {
		t.Fatalf("primera portada: esperaba rojo, salió %v", c)
	}
	icon = blue // same row, another artist (or the thumbnail just arrived)
	l.refresh()
	if c := sample(); c.B < 180 || c.R > 80 {
		t.Fatalf("la fila sigue con la portada anterior: esperaba azul, salió %v", c)
	}
}
