package app

import (
	"github.com/hinshun/vt10x"

	"tetmux/internal/vtrender"
)

// vt10x glyph mode bits (mirrored from the library's internal attr constants:
// attrReverse = 1<<0, attrUnderline = 1<<1, attrBold = 1<<2).
const (
	attrReverse int16 = 1 << 0
	attrBold    int16 = 1 << 2
)

// vt10x default color sentinels (Color = 1<<24 + iota for DefaultFG/BG/Cursor).
const (
	defaultFG vt10x.Color = 1<<24 + iota
	defaultBG
)

// vtGridAdapter wraps a vt10x.Terminal view so it satisfies the
// vtrender.Grid interface. Callers MUST hold the terminal lock around Render;
// the adapter itself does not lock so the caller can take one consistent
// snapshot under a single Lock()/Unlock().
type vtGridAdapter struct {
	term vt10x.Terminal
}

func newVTGrid(term vt10x.Terminal) *vtGridAdapter {
	return &vtGridAdapter{term: term}
}

func (a *vtGridAdapter) Size() (int, int) { return a.term.Size() }

func (a *vtGridAdapter) CellRune(x, y int) rune {
	g := a.term.Cell(x, y)
	return g.Char
}

func (a *vtGridAdapter) CellStyle(x, y int) vtrender.Style {
	g := a.term.Cell(x, y)
	st := vtrender.Style{FG: -1, BG: -1}
	if g.FG != defaultFG {
		st.FG = int(g.FG)
	}
	if g.BG != defaultBG {
		st.BG = int(g.BG)
	}
	if g.Mode&attrBold != 0 {
		st.Bold = true
	}
	if g.Mode&attrReverse != 0 {
		st.Reverse = true
	}
	return st
}

func (a *vtGridAdapter) Cursor() (int, int, bool) {
	c := a.term.Cursor()
	return c.X, c.Y, a.term.CursorVisible()
}
