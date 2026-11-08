package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// noCursor is an invisible layer over the whole window that hides the mouse
// pointer: the rockola is used only with the keypad, and a pointer left in
// the middle of the TV would cover the lyrics. It is hoverable so the
// window looks no further down for another cursor; it takes no clicks or keys.
type noCursor struct{ widget.BaseWidget }

func newNoCursor() *noCursor {
	n := &noCursor{}
	n.ExtendBaseWidget(n)
	return n
}

func (*noCursor) Cursor() desktop.Cursor         { return desktop.HiddenCursor }
func (*noCursor) MouseIn(*desktop.MouseEvent)    {}
func (*noCursor) MouseMoved(*desktop.MouseEvent) {}
func (*noCursor) MouseOut()                      {}
func (n *noCursor) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(&fyne.Container{})
}
