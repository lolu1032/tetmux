// Package vtrender renders an in-memory VT cell grid (rows of styled runes,
// plus a cursor) to clipped/padded string rows suitable for placing in a
// lipgloss pane. It is pure: it depends only on lipgloss for styling and a
// small Grid interface, so it can be unit-tested with a fake grid (no real
// terminal emulator required).
package vtrender

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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
	// Cached SGR prefixes were computed against the old profile; drop them so
	// subsequent renders re-derive sequences for the new profile.
	clearSGRCache()
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

// cellRuneOrSpace returns the visible rune for (x, y), mapping NUL, missing
// cells, and any stray control rune to a space. Control runes (C0 < 0x20, DEL,
// and C1 0x80–0x9f) are scrubbed so untrusted child output — e.g. a raw control
// byte that fell through the emulator's line-drawing (gfx) mode into a cell —
// can never be re-emitted to the host terminal as a corruption/injection vector.
func cellRuneOrSpace(g Grid, x, y, cols, rows int) rune {
	if x < 0 || y < 0 || x >= cols || y >= rows {
		return ' '
	}
	r := g.CellRune(x, y)
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return ' '
	}
	return r
}

// widthCond measures glyph width with East Asian *ambiguous* characters pinned
// to one column, so it agrees exactly with the patched vt10x emulator (see the
// matching widthCond in third_party/vt10x). go-runewidth auto-enables
// EastAsianWidth from the locale, which would size box-drawing/bullets/arrows at
// two columns and desync the renderer from the grid; pinning it off keeps only
// truly wide glyphs (Hangul, CJK, emoji) at two columns.
var widthCond = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return c
}()

// cellWidth reports how many terminal columns a rune occupies: 2 for East Asian
// wide / fullwidth glyphs (CJK, Hangul...), 1 otherwise. It must agree with the
// width logic in the patched vt10x emulator (third_party/vt10x), which advances
// the cursor — and thus reserves a trailing grid column — for each wide glyph.
// The renderer therefore steps the column cursor by this width too, so it emits
// the wide rune once and skips the reserved trailing cell instead of printing a
// blank "phantom" column after it.
func cellWidth(r rune) int {
	if widthCond.RuneWidth(r) == 2 {
		return 2
	}
	return 1
}

// renderPlainRow builds one row of exactly w display columns with no styling.
// Wide (CJK) glyphs consume two columns and the grid cell reserved after them
// is skipped; a wide glyph that would straddle the final column is replaced by
// a space so the row never overflows its width. Cursor visualization in plain
// mode is a no-op so the width stays exact and predictable for tests.
func renderPlainRow(g Grid, y, w, cols, rows int, drawCursor bool, curX, curY int) string {
	var sb strings.Builder
	sb.Grow(w)
	for col := 0; col < w; {
		r := cellRuneOrSpace(g, col, y, cols, rows)
		if cellWidth(r) == 2 {
			if col+2 > w {
				sb.WriteByte(' ') // wide glyph won't fit the last column
				col++
				continue
			}
			sb.WriteRune(r)
			col += 2
			continue
		}
		sb.WriteRune(r)
		col++
	}
	return sb.String()
}

// resetSeq is the SGR reset that closes every styled run.
const resetSeq = "\x1b[0m"

// renderStyledRow builds one row by emitting raw ANSI SGR sequences directly,
// coalescing runs of identical style so a run shares a single escape prefix and
// reset. The cursor cell (if drawn) is forced to reverse video. This replaces a
// per-segment lipgloss.NewStyle().Render() path that allocated ~7KB and cost
// ~0.7ms PER ROW on busy output (e.g. a coding agent's colored text), which is
// what made typing into the left pane feel laggy. The per-style escape prefix
// is computed once and cached (see sgrPrefix), so a frame is now just map
// lookups plus string writes.
func renderStyledRow(g Grid, y, w, cols, rows int, drawCursor bool, curX, curY int) string {
	var sb strings.Builder
	sb.Grow(w + 16)

	open := false // is an SGR run currently open (needs a reset)?
	var cur Style // the style of the currently open run
	first := true
	for col := 0; col < w; {
		st := DefaultStyle
		if col < cols && y < rows {
			st = g.CellStyle(col, y)
		}
		if drawCursor && col == curX && y == curY {
			st.Reverse = true
		}
		if first || st != cur {
			if open {
				sb.WriteString(resetSeq)
				open = false
			}
			if prefix := sgrPrefix(st); prefix != "" {
				sb.WriteString(prefix)
				open = true
			}
			cur = st
			first = false
		}
		r := cellRuneOrSpace(g, col, y, cols, rows)
		if cellWidth(r) == 2 {
			if col+2 > w {
				sb.WriteByte(' ') // wide glyph won't fit the last column
				col++
				continue
			}
			sb.WriteRune(r)
			col += 2
			continue
		}
		sb.WriteRune(r)
		col++
	}
	if open {
		sb.WriteString(resetSeq)
	}
	return sb.String()
}

// sgrCache memoizes the SGR escape prefix for each distinct Style so the
// profile-aware color conversion runs once, not once per cell per frame. Style
// is a small comparable struct, so it is a valid key. The cache is cleared when
// the color profile changes (SetColorProfile).
var sgrCache sync.Map // Style -> string

// sgrCacheLen approximately counts cached entries so the cache can be bounded.
// A truecolor child (syntax-highlighted diffs, gradients) emits unboundedly many
// distinct RGB styles; without a cap the sync.Map would grow for the whole
// process lifetime — a slow leak proportional to colors seen.
var sgrCacheLen int64

// sgrCacheCap bounds the cache. Past it the cache is dropped wholesale and
// repopulated, trading an occasional rebuild for bounded memory.
const sgrCacheCap = 4096

// sgrPrefix returns the cached ANSI SGR prefix (e.g. "\x1b[1;38;5;51m") for a
// style, or "" when the style needs no styling at all.
func sgrPrefix(st Style) string {
	if v, ok := sgrCache.Load(st); ok {
		return v.(string)
	}
	p := buildSGR(st)
	if atomic.AddInt64(&sgrCacheLen, 1) > sgrCacheCap {
		clearSGRCache()
	}
	sgrCache.Store(st, p)
	return p
}

// clearSGRCache empties the memoization table and resets its counter. Called
// when the cache exceeds its cap or the color profile changes.
func clearSGRCache() {
	sgrCache.Range(func(k, _ any) bool { sgrCache.Delete(k); return true })
	atomic.StoreInt64(&sgrCacheLen, 0)
}

// buildSGR composes the SGR escape for a style, converting color indices
// through the active color profile so low-color terminals still get sensible
// (downsampled) sequences. Returns "" for the unstyled default.
func buildSGR(st Style) string {
	if st.FG < 0 && st.BG < 0 && !st.Bold && !st.Reverse {
		return ""
	}
	prof := renderer.ColorProfile()
	var parts []string
	if st.Bold {
		parts = append(parts, "1")
	}
	if st.Reverse {
		parts = append(parts, "7")
	}
	if seq := colorSeq(prof, st.FG, false); seq != "" {
		parts = append(parts, seq)
	}
	if seq := colorSeq(prof, st.BG, true); seq != "" {
		parts = append(parts, seq)
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// colorSeq returns the SGR parameter(s) for a color value, profile-converted.
// Values 0..255 are palette indices; values >= 256 are a 24-bit truecolor
// packed as r<<16|g<<8|b (how vt10x stores `38;2;r;g;b`). Passing the packed
// int straight through as a palette index produced invalid sequences like
// "38;5;16737280" that corrupt the line — so RGB values are handed to termenv
// as a hex color, which down-converts to the terminal's actual profile (e.g.
// nearest 256-color on Terminal.app, real truecolor on iTerm). Returns "" for
// the default color (v < 0).
func colorSeq(prof termenv.Profile, v int, bg bool) string {
	if v < 0 {
		return ""
	}
	var c termenv.Color
	if v >= 256 {
		c = prof.Color(hexColor(v))
	} else {
		c = prof.Color(itoa(v))
	}
	if c == nil {
		return ""
	}
	return c.Sequence(bg)
}

// hexColor renders a packed 24-bit RGB value (r<<16|g<<8|b) as "#rrggbb"
// without pulling in fmt, keeping the render hot path allocation-light.
func hexColor(v int) string {
	const hex = "0123456789abcdef"
	r, g, b := (v>>16)&0xff, (v>>8)&0xff, v&0xff
	return string([]byte{
		'#',
		hex[r>>4], hex[r&0xf],
		hex[g>>4], hex[g&0xf],
		hex[b>>4], hex[b&0xf],
	})
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
