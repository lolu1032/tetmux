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

// TestWideGlyphSkipsReservedCell pins the width-aware row builder: a wide (CJK)
// glyph occupies two display columns, and the grid cell reserved right after it
// (a blank, mirroring the patched emulator's layout) is skipped rather than
// printed as a phantom space.
func TestWideGlyphSkipsReservedCell(t *testing.T) {
	// Grid: 클 [reserved] 로 [reserved] 드 — exactly what the width-aware
	// emulator produces (wide glyph + one reserved blank column).
	g := newFakeGrid([]string{"클 로 드"})
	row := Render(g, Options{Width: 6, Height: 1})[0]
	if row != "클로드" {
		t.Errorf("wide-glyph row = %q, want %q", row, "클로드")
	}
	if lipgloss.Width(row) != 6 {
		t.Errorf("visible width = %d, want 6", lipgloss.Width(row))
	}
}

// TestWideGlyphClippedAtBoundary ensures a wide glyph that would straddle the
// final column is replaced by a space so the row never exceeds its width.
func TestWideGlyphClippedAtBoundary(t *testing.T) {
	g := newFakeGrid([]string{"a클"}) // 'a' (1) + '클' (2) wants 3 columns
	row := Render(g, Options{Width: 2, Height: 1})[0]
	if lipgloss.Width(row) != 2 {
		t.Errorf("visible width = %d, want 2 (%q)", lipgloss.Width(row), row)
	}
	if row != "a " {
		t.Errorf("boundary row = %q, want %q (wide glyph must not overflow)", row, "a ")
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

// TestTruecolorEmitsValidSGR pins the fix for the corruption bug: a 24-bit RGB
// cell (stored by vt10x as a packed r<<16|g<<8|b value >= 256) must NOT leak its
// raw decimal into the escape (the old bug emitted "\x1b[38;5;16737280m"), and
// the visible width must stay 1.
func TestTruecolorEmitsValidSGR(t *testing.T) {
	const packed = 0xff6f00 // r=255,g=111,b=0 -> 16740096, a typical agent color
	g := &fakeGrid{cols: 1, rows: 1,
		runes:  [][]rune{{'X'}},
		styles: [][]Style{{{FG: packed, BG: -1}}},
	}
	row := Render(g, Options{Width: 1, Height: 1, Styled: true})[0]
	if strings.Contains(row, "16740096") {
		t.Errorf("raw packed RGB leaked into SGR (the corruption bug): %q", row)
	}
	if strings.Contains(row, "38;5;167") || strings.Contains(row, "38;5;1674") {
		t.Errorf("out-of-range 256-color index emitted: %q", row)
	}
	if lipgloss.Width(row) != 1 {
		t.Errorf("truecolor cell visible width = %d, want 1 (%q)", lipgloss.Width(row), row)
	}
	if !strings.ContainsRune(row, 'X') {
		t.Errorf("truecolor cell lost its rune: %q", row)
	}
	// buildSGR must produce a syntactically valid SGR for the packed value.
	seq := buildSGR(Style{FG: packed, BG: -1})
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		t.Errorf("buildSGR(truecolor) = %q, want a \\x1b[...m sequence", seq)
	}
}

// TestSGRCoalescing pins the run-coalescing correctness of the styled renderer
// (not just its visible width): a run of identical styles shares one prefix and
// one reset, a style change closes the prior run, the trailing run is always
// closed, and default cells emit no escapes at all.
func TestSGRCoalescing(t *testing.T) {
	red := Style{FG: 1, BG: -1}
	blue := Style{FG: 4, BG: -1}

	// Two identical red cells -> exactly one prefix + one reset.
	g := &fakeGrid{cols: 2, rows: 1, runes: [][]rune{{'a', 'b'}}, styles: [][]Style{{red, red}}}
	row := Render(g, Options{Width: 2, Height: 1, Styled: true})[0]
	if n := strings.Count(row, resetSeq); n != 1 {
		t.Errorf("identical run should emit one reset, got %d: %q", n, row)
	}
	if c := strings.Count(row, "\x1b["); c != 2 { // 1 color prefix + 1 reset are both "\x1b["
		t.Errorf("identical run should emit one prefix + one reset (2 escapes), got %d: %q", c, row)
	}

	// red,blue -> prior run closed before the next opens (two resets).
	g2 := &fakeGrid{cols: 2, rows: 1, runes: [][]rune{{'a', 'b'}}, styles: [][]Style{{red, blue}}}
	row2 := Render(g2, Options{Width: 2, Height: 1, Styled: true})[0]
	if n := strings.Count(row2, resetSeq); n != 2 {
		t.Errorf("style transition should close each run: want 2 resets, got %d: %q", n, row2)
	}
	if !strings.HasSuffix(row2, resetSeq) {
		t.Errorf("trailing run must be closed with a reset: %q", row2)
	}

	// Default cells emit no escapes.
	g3 := &fakeGrid{cols: 2, rows: 1, runes: [][]rune{{'a', 'b'}}, styles: [][]Style{{DefaultStyle, DefaultStyle}}}
	row3 := Render(g3, Options{Width: 2, Height: 1, Styled: true})[0]
	if strings.Contains(row3, "\x1b[") {
		t.Errorf("default cells should emit no escapes: %q", row3)
	}
	if row3 != "ab" {
		t.Errorf("default styled row = %q want \"ab\"", row3)
	}
}

func TestBuildSGRComposition(t *testing.T) {
	// Bold+reverse with no color -> "\x1b[1;7m"; ordering is bold, reverse.
	if got := buildSGR(Style{FG: -1, BG: -1, Bold: true, Reverse: true}); got != "\x1b[1;7m" {
		t.Errorf("bold+reverse SGR = %q want \\x1b[1;7m", got)
	}
	// Pure default -> empty.
	if got := buildSGR(DefaultStyle); got != "" {
		t.Errorf("default SGR = %q want empty", got)
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

// Control runes stored in a cell (e.g. a raw byte that slipped through gfx mode)
// must render as a space, never be re-emitted to the host terminal.
func TestControlRunesScrubbedToSpace(t *testing.T) {
	g := &fakeGrid{
		cols:  6,
		rows:  1,
		runes: [][]rune{{'a', 0x07, 0x1b, 'b', 0x9b, 0x7f}}, // BEL, ESC, C1 CSI, DEL
		styles: [][]Style{{
			DefaultStyle, DefaultStyle, DefaultStyle,
			DefaultStyle, DefaultStyle, DefaultStyle,
		}},
	}
	rows := Render(g, Options{Width: 6, Height: 1})
	if got, want := rows[0], "a  b  "; got != want {
		t.Errorf("control runes not scrubbed: %q want %q", got, want)
	}
}

// The SGR cache stays bounded: feeding far more distinct styles than the cap must
// not grow it without limit (a truecolor child would otherwise leak for the life
// of the process). After the flood the table is at most ~sgrCacheCap entries.
func TestSGRCacheBounded(t *testing.T) {
	SetColorProfile(termenv.TrueColor) // also resets the cache + counter
	for i := 0; i < sgrCacheCap*3; i++ {
		_ = sgrPrefix(Style{FG: i + 256, BG: -1}) // distinct packed-RGB styles
	}
	n := 0
	sgrCache.Range(func(_, _ any) bool { n++; return true })
	if n > sgrCacheCap+1 {
		t.Errorf("SGR cache grew to %d entries, want <= %d (cap not enforced)", n, sgrCacheCap+1)
	}
	SetColorProfile(termenv.ANSI) // restore the package default for other tests
}
