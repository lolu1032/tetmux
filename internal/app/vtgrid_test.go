package app

import (
	"bufio"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

// feed drains s through the vt10x parser, the same path the live reader uses.
func feed(term vt10x.Terminal, s string) {
	br := bufio.NewReader(strings.NewReader(s))
	for {
		if err := term.Parse(br); err != nil {
			break
		}
	}
}

// TestVTGridAttrsPinned pins the hardcoded vt10x attr-bit constants used in
// vtgrid.go (attrBold/attrReverse) against the real emulator. If a vt10x
// version bump changes those private iota values, this test fails instead of
// silently rendering the left pane with wrong styles.
func TestVTGridAttrsPinned(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(8, 2))
	// Bold "B", reset, then reverse "R".
	feed(term, "\x1b[1mB\x1b[0m\x1b[7mR")

	g := newVTGrid(term)

	if got := g.CellRune(0, 0); got != 'B' {
		t.Fatalf("cell(0,0) rune=%q want 'B'", got)
	}
	if st := g.CellStyle(0, 0); !st.Bold {
		t.Errorf("cell(0,0) expected Bold, got %+v", st)
	}
	if got := g.CellRune(1, 0); got != 'R' {
		t.Fatalf("cell(1,0) rune=%q want 'R'", got)
	}
	if st := g.CellStyle(1, 0); !st.Reverse {
		t.Errorf("cell(1,0) expected Reverse, got %+v", st)
	}
}

// TestVTGridSizeMatches sanity-checks the adapter reports the terminal size.
func TestVTGridSizeMatches(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(12, 5))
	g := newVTGrid(term)
	cols, rows := g.Size()
	if cols != 12 || rows != 5 {
		t.Errorf("Size()=%d,%d want 12,5", cols, rows)
	}
}
