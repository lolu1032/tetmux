package vtrender

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces a known color profile so styled-output and cursor tests are
// deterministic regardless of whether the test runs under a TTY.
func TestMain(m *testing.M) {
	SetColorProfile(termenv.ANSI)
	os.Exit(m.Run())
}

// fakeGrid is a deterministic in-memory Grid for testing.
type fakeGrid struct {
	cols, rows int
	runes      [][]rune
	styles     [][]Style
	curX, curY int
	curVis     bool
}

func newFakeGrid(rows []string) *fakeGrid {
	g := &fakeGrid{rows: len(rows)}
	for _, r := range rows {
		rr := []rune(r)
		if len(rr) > g.cols {
			g.cols = len(rr)
		}
		g.runes = append(g.runes, rr)
		st := make([]Style, len(rr))
		for i := range st {
			st[i] = DefaultStyle
		}
		g.styles = append(g.styles, st)
	}
	return g
}

func (g *fakeGrid) Size() (int, int) { return g.cols, g.rows }

func (g *fakeGrid) CellRune(x, y int) rune {
	if y < 0 || y >= len(g.runes) || x < 0 || x >= len(g.runes[y]) {
		return 0
	}
	return g.runes[y][x]
}

func (g *fakeGrid) CellStyle(x, y int) Style {
	if y < 0 || y >= len(g.styles) || x < 0 || x >= len(g.styles[y]) {
		return DefaultStyle
	}
	return g.styles[y][x]
}

func (g *fakeGrid) Cursor() (int, int, bool) { return g.curX, g.curY, g.curVis }

// --- exact fit ---

func TestExactFit(t *testing.T) {
	g := newFakeGrid([]string{"abc", "def"})
	rows := Render(g, Options{Width: 3, Height: 2})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0] != "abc" || rows[1] != "def" {
		t.Errorf("exact fit mismatch: %q", rows)
	}
	for i, r := range rows {
		if len([]rune(r)) != 3 {
			t.Errorf("row %d width = %d, want 3", i, len([]rune(r)))
		}
	}
}

// --- clip wider ---

func TestClipWider(t *testing.T) {
	g := newFakeGrid([]string{"abcdef", "ghijkl"})
	rows := Render(g, Options{Width: 3, Height: 2})
	if rows[0] != "abc" || rows[1] != "ghi" {
		t.Errorf("clip wider mismatch: %q", rows)
	}
	for _, r := range rows {
		if len([]rune(r)) > 3 {
			t.Errorf("row longer than width: %q", r)
		}
	}
}

// --- clip taller ---

func TestClipTaller(t *testing.T) {
	g := newFakeGrid([]string{"a", "b", "c", "d"})
	rows := Render(g, Options{Width: 1, Height: 2})
	if len(rows) != 2 {
		t.Fatalf("expected exactly 2 rows, got %d", len(rows))
	}
	if rows[0] != "a" || rows[1] != "b" {
		t.Errorf("clip taller mismatch: %q", rows)
	}
}

// --- pad smaller ---

func TestPadSmaller(t *testing.T) {
	g := newFakeGrid([]string{"ab"})
	rows := Render(g, Options{Width: 5, Height: 3})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (padded), got %d", len(rows))
	}
	if rows[0] != "ab   " {
		t.Errorf("row 0 not padded with spaces: %q", rows[0])
	}
	if rows[1] != strings.Repeat(" ", 5) || rows[2] != strings.Repeat(" ", 5) {
		t.Errorf("blank rows not full-width spaces: %q", rows[1:])
	}
	for i, r := range rows {
		if len([]rune(r)) != 5 {
			t.Errorf("row %d width = %d, want 5", i, len([]rune(r)))
		}
	}
}

// --- empty grid ---

func TestEmptyGrid(t *testing.T) {
	g := newFakeGrid(nil)
	rows := Render(g, Options{Width: 4, Height: 3})
	if len(rows) != 3 {
		t.Fatalf("expected 3 blank rows, got %d", len(rows))
	}
	for i, r := range rows {
		if r != strings.Repeat(" ", 4) {
			t.Errorf("row %d not 4 blank spaces: %q", i, r)
		}
	}
}

// --- fake grid / NUL handling ---

func TestNullRuneRendersAsSpace(t *testing.T) {
	g := &fakeGrid{cols: 3, rows: 1,
		runes:  [][]rune{{'a', 0, 'c'}},
		styles: [][]Style{{DefaultStyle, DefaultStyle, DefaultStyle}},
	}
	rows := Render(g, Options{Width: 3, Height: 1})
	if rows[0] != "a c" {
		t.Errorf("NUL cell should render as space, got %q", rows[0])
	}
	// Ensure no literal NUL byte leaks through.
	if strings.ContainsRune(rows[0], '\x00') {
		t.Errorf("NUL byte leaked into output: %q", rows[0])
	}
}

func TestZeroSizeNoPanic(t *testing.T) {
	g := newFakeGrid([]string{"abc"})
	rows := Render(g, Options{Width: 0, Height: 0})
	if len(rows) != 0 {
		t.Errorf("zero height should give no rows, got %d", len(rows))
	}
	// Negative clamps to zero.
	rows = Render(g, Options{Width: -5, Height: -5})
	if len(rows) != 0 {
		t.Errorf("negative dims should give no rows, got %d", len(rows))
	}
}

// --- styled / color / cursor dimension ---

func TestStyledOutputContainsColor(t *testing.T) {
	g := &fakeGrid{cols: 1, rows: 1,
		runes:  [][]rune{{'X'}},
		styles: [][]Style{{{FG: 1, BG: -1}}},
	}
	rows := Render(g, Options{Width: 1, Height: 1, Styled: true})
	// The styled output should contain ANSI escape codes when color profile
	// supports it. We at least assert the visible content width is preserved.
	if lipgloss.Width(rows[0]) != 1 {
		t.Errorf("styled row visible width = %d, want 1", lipgloss.Width(rows[0]))
	}
	if !strings.ContainsRune(rows[0], 'X') {
		t.Errorf("styled row lost its rune: %q", rows[0])
	}
}

func TestCursorDrawnWhenVisibleAndShown(t *testing.T) {
	g := &fakeGrid{cols: 3, rows: 1,
		runes:  [][]rune{{'a', 'b', 'c'}},
		styles: [][]Style{{DefaultStyle, DefaultStyle, DefaultStyle}},
		curX:   1, curY: 0, curVis: true,
	}
	// With styling on and ShowCursor, the cursor cell gets reverse styling so
	// the rendered string differs from the non-cursor styled render.
	withCur := Render(g, Options{Width: 3, Height: 1, Styled: true, ShowCursor: true})
	g.curVis = false
	noCur := Render(g, Options{Width: 3, Height: 1, Styled: true, ShowCursor: true})
	if withCur[0] == noCur[0] {
		t.Errorf("cursor cell should render differently when visible")
	}
	// Visible width unchanged.
	if lipgloss.Width(withCur[0]) != 3 {
		t.Errorf("cursor render changed visible width: %d", lipgloss.Width(withCur[0]))
	}
}

func TestCursorNotDrawnWhenHidden(t *testing.T) {
	g := &fakeGrid{cols: 3, rows: 1,
		runes:  [][]rune{{'a', 'b', 'c'}},
		styles: [][]Style{{DefaultStyle, DefaultStyle, DefaultStyle}},
		curX:   1, curY: 0, curVis: false,
	}
	withShow := Render(g, Options{Width: 3, Height: 1, Styled: true, ShowCursor: true})
	noShow := Render(g, Options{Width: 3, Height: 1, Styled: true, ShowCursor: false})
	if withShow[0] != noShow[0] {
		t.Errorf("hidden cursor should not change rendering")
	}
}

func TestStyledTableDimension(t *testing.T) {
	cases := []struct {
		name  string
		style Style
	}{
		{"default", DefaultStyle},
		{"fg-red", Style{FG: 1, BG: -1}},
		{"bg-blue", Style{FG: -1, BG: 4}},
		{"bold", Style{FG: -1, BG: -1, Bold: true}},
		{"reverse", Style{FG: -1, BG: -1, Reverse: true}},
		{"fg-bg-bold", Style{FG: 2, BG: 5, Bold: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGrid{cols: 2, rows: 1,
				runes:  [][]rune{{'Z', 'Z'}},
				styles: [][]Style{{tc.style, tc.style}},
			}
			rows := Render(g, Options{Width: 2, Height: 1, Styled: true})
			if lipgloss.Width(rows[0]) != 2 {
				t.Errorf("%s: visible width = %d, want 2", tc.name, lipgloss.Width(rows[0]))
			}
		})
	}
}
