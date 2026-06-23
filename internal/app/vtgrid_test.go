package app

import (
	"strings"
	"testing"

	"github.com/hinshun/vt10x"

	"tetmux/internal/vtrender"
)

// feed drives s through the vt10x emulator via Write — the SAME call the live
// reader goroutine uses (leftpane.readLoop -> term.Write). It intentionally does
// NOT use term.Parse: Parse blocks on its reader and is banned from the read
// loop, so tests must exercise the Write path that production actually runs.
func feed(term vt10x.Terminal, s string) {
	_, _ = term.Write([]byte(s))
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

// TestVTGridColors pins how the adapter maps vt10x colors into vtrender.Style:
// 256-palette indices pass through, the default-FG sentinel becomes -1 (must not
// leak as a giant index), and 24-bit truecolor is kept as its packed RGB value
// (which vtrender then down-converts to the terminal profile).
func TestVTGridColors(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(8, 1))
	// 256-color FG, then reset FG to default, then a truecolor FG.
	feed(term, "\x1b[38;5;51mA\x1b[39mB\x1b[38;2;255;111;0mC")
	g := newVTGrid(term)

	if st := g.CellStyle(0, 0); st.FG != 51 {
		t.Errorf("256-color cell FG=%d want 51", st.FG)
	}
	if st := g.CellStyle(1, 0); st.FG != -1 {
		t.Errorf("default-FG cell FG=%d want -1 (sentinel must not leak)", st.FG)
	}
	const wantRGB = 255<<16 | 111<<8 | 0
	if st := g.CellStyle(2, 0); st.FG != wantRGB {
		t.Errorf("truecolor cell FG=%d want %d", st.FG, wantRGB)
	}
}

// TestCJKWidthNoPhantomSpaces is the end-to-end regression for the left-pane
// Hangul bug: a width-aware client (e.g. Claude Code via bubbletea) emits a wide
// glyph then repositions the cursor to the NEXT display column. Upstream vt10x
// advanced one column per rune, so the reposition skipped a cell and left a
// blank "phantom" between every glyph ("클 로 드"). The patched emulator advances
// by the glyph width, so the grid stays compact and the row renders cleanly.
func TestCJKWidthNoPhantomSpaces(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 1))
	// "클" then CSI to display column 3 (1-based) = the next cell after the wide
	// glyph, "로", then column 5, "드" — mirrors a cellbuf redraw.
	feed(term, "클\x1b[3G로\x1b[5G드")

	row := vtrender.Render(newVTGrid(term), vtrender.Options{Width: 20, Height: 1})[0]
	got := strings.TrimRight(row, " ")
	if got != "클로드" {
		t.Errorf("CJK row = %q, want %q (phantom spaces between glyphs?)", got, "클로드")
	}
}

// TestCJKRealSpacePreserved guards the other direction: a genuine space the
// program wrote between Hangul words must survive. A grid-level "drop the blank
// after a wide glyph" hack would have eaten it; the width-aware emulator does
// not, because the real space lands in its own (non-reserved) cell.
func TestCJKRealSpacePreserved(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 1))
	feed(term, "수정 완료") // raw consecutive runes incl. one real space

	row := vtrender.Render(newVTGrid(term), vtrender.Options{Width: 20, Height: 1})[0]
	got := strings.TrimRight(row, " ")
	if got != "수정 완료" {
		t.Errorf("CJK row = %q, want %q (real space lost or doubled?)", got, "수정 완료")
	}
}

// TestAmbiguousWidthBoxDrawingStaysNarrow guards the left-pane framing bug: in
// an East Asian locale go-runewidth defaults ambiguous glyphs (box-drawing,
// bullets, arrows) to two columns, which overflowed and wrapped a width-aware
// client's borders (Claude Code's rounded box). The emulator and renderer pin
// EastAsianWidth off, so these glyphs stay one column and a 7-glyph box fits on
// one row instead of wrapping.
func TestAmbiguousWidthBoxDrawingStaysNarrow(t *testing.T) {
	term := vt10x.New(vt10x.WithSize(20, 2))
	feed(term, "╭─────╮") // 7 box-drawing glyphs, all single-width

	rows := vtrender.Render(newVTGrid(term), vtrender.Options{Width: 20, Height: 2})
	if got := strings.TrimRight(rows[0], " "); got != "╭─────╮" {
		t.Errorf("box-drawing row = %q, want %q (ambiguous glyphs sized as 2 cols?)", got, "╭─────╮")
	}
	if strings.TrimSpace(rows[1]) != "" {
		t.Errorf("box-drawing line wrapped onto row 1: %q", rows[1])
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
