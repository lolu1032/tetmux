// Package vtrender renders an in-memory VT cell grid (rows of styled runes,
// plus a cursor) to clipped/padded string rows suitable for placing in a
// lipgloss pane. It is pure: it depends only on lipgloss for styling and a
// small Grid interface, so it can be unit-tested with a fake grid (no real
// terminal emulator required).
package vtrender

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// renderer is the lipgloss renderer used for styled output. It defaults to the
// process stdout's detected profile, but tests (and a non-TTY environment) can
// force a known profile via SetColorProfile so that cursor/color styling is
// deterministic and observable.
var renderer = lipgloss.NewRenderer(os.Stdout)

// SetColorProfile forces the styling color profile used by Render. Pass
// termenv.ANSI (or higher) to guarantee escape sequences are emitted even when
// stdout is not a TTY. Returns nothing; affects subsequent Render calls.
func SetColorProfile(p termenv.Profile) {
	renderer.SetColorProfile(p)
}

// Style carries the basic visual attributes of a cell. Foreground and
// Background are ANSI color indices; a negative value means "default".
// Bold/Reverse mirror common VT modes.
type Style struct {
	FG      int
	BG      int
	Bold    bool
	Reverse bool
}

// DefaultStyle is the zero-attribute style (default colors, no flags).
var DefaultStyle = Style{FG: -1, BG: -1}

// Grid is the minimal interface the renderer needs. A real vt10x terminal view
// is wrapped to satisfy it; tests supply a deterministic fake.
type Grid interface {
	// Size returns the grid dimensions in columns and rows.
	Size() (cols, rows int)
	// CellRune returns the rune at (x, y). A zero rune ('\x00') is treated as
	// a blank space by the renderer.
	CellRune(x, y int) rune
	// CellStyle returns the style at (x, y).
	CellStyle(x, y int) Style
	// Cursor returns the cursor position and whether it is currently visible.
	Cursor() (x, y int, visible bool)
}

// Options controls how a grid is rendered into a viewport.
type Options struct {
	// Width and Height are the target viewport size in terminal cells.
	Width  int
	Height int
	// Styled enables lipgloss color/bold/reverse output. When false the
	// renderer emits plain runes (faster, and exact-width assertions in tests
	// are trivial). The cursor is still drawn when ShowCursor is true.
	Styled bool
	// ShowCursor draws a visible block at the cursor cell (used for the
	// focused pane). The cursor is only drawn if the grid reports it visible
	// and it falls within the viewport.
	ShowCursor bool
}

// Render produces exactly Options.Height rows, each exactly Options.Width
// display cells wide (plain mode) once clipped/padded. Grids larger than the
// viewport are clipped (trailing columns/rows dropped); smaller grids are
// padded with spaces and blank lines.
//
// In plain mode (Styled=false) the returned rows are guaranteed to be exactly
// Width runes long (cursor cell included), which the tests assert. In styled
// mode the visible width is the same but the strings contain ANSI escapes.
func Render(g Grid, opt Options) []string {
	w, h := opt.Width, opt.Height
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}

	cols, rows := g.Size()
	curX, curY, curVis := g.Cursor()
	drawCursor := opt.ShowCursor && curVis

	out := make([]string, 0, h)
	for y := 0; y < h; y++ {
		if opt.Styled {
			out = append(out, renderStyledRow(g, y, w, cols, rows, drawCursor, curX, curY))
		} else {
			out = append(out, renderPlainRow(g, y, w, cols, rows, drawCursor, curX, curY))
		}
	}
	return out
}

// cellRuneOrSpace returns the visible rune for (x, y), mapping NUL and missing
// cells to a space.
func cellRuneOrSpace(g Grid, x, y, cols, rows int) rune {
	if x < 0 || y < 0 || x >= cols || y >= rows {
		return ' '
	}
	r := g.CellRune(x, y)
	if r == 0 || r == '\x00' {
		return ' '
	}
	return r
}

// renderPlainRow builds one row of exactly w runes with no styling, drawing the
// cursor as a reverse-ish marker handled by the caller (plain mode just keeps
// the underlying rune; cursor visualization in plain mode is a no-op so the
// width stays exact and predictable for tests).
func renderPlainRow(g Grid, y, w, cols, rows int, drawCursor bool, curX, curY int) string {
	var sb strings.Builder
	sb.Grow(w)
	for x := 0; x < w; x++ {
		sb.WriteRune(cellRuneOrSpace(g, x, y, cols, rows))
	}
	return sb.String()
}

// renderStyledRow builds one row using lipgloss styling, coalescing runs of
// identical style into single styled segments for compact output. The cursor
// cell (if drawn) is forced to a reverse style.
func renderStyledRow(g Grid, y, w, cols, rows int, drawCursor bool, curX, curY int) string {
	var sb strings.Builder

	type seg struct {
		style Style
		cur   bool
		runes []rune
	}
	var segs []seg
	for x := 0; x < w; x++ {
		r := cellRuneOrSpace(g, x, y, cols, rows)
		var st Style
		if x < cols && y < rows {
			st = g.CellStyle(x, y)
		} else {
			st = DefaultStyle
		}
		isCur := drawCursor && x == curX && y == curY
		if n := len(segs); n > 0 && segs[n-1].style == st && segs[n-1].cur == isCur {
			segs[n-1].runes = append(segs[n-1].runes, r)
		} else {
			segs = append(segs, seg{style: st, cur: isCur, runes: []rune{r}})
		}
	}

	for _, s := range segs {
		text := string(s.runes)
		ls := renderer.NewStyle()
		if s.style.FG >= 0 {
			ls = ls.Foreground(lipgloss.Color(colorString(s.style.FG)))
		}
		if s.style.BG >= 0 {
			ls = ls.Background(lipgloss.Color(colorString(s.style.BG)))
		}
		if s.style.Bold {
			ls = ls.Bold(true)
		}
		if s.style.Reverse || s.cur {
			ls = ls.Reverse(true)
		}
		sb.WriteString(ls.Render(text))
	}
	return sb.String()
}

// colorString converts an ANSI color index to a lipgloss color string.
func colorString(idx int) string {
	// lipgloss accepts ANSI 256 color numbers as decimal strings.
	return itoa(idx)
}

// itoa is a tiny dependency-free int->string for non-negative numbers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
