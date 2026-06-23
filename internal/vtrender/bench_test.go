package vtrender

import "testing"

// benchGrid builds a w x h grid with frequent style changes (varied FG colors,
// occasional bold/reverse) to mimic a busy TUI like a coding agent's output —
// the worst case for the styled renderer's per-segment lipgloss work.
func benchGrid(w, h int) *fakeGrid {
	g := &fakeGrid{cols: w, rows: h}
	for y := 0; y < h; y++ {
		rr := make([]rune, w)
		st := make([]Style, w)
		for x := 0; x < w; x++ {
			rr[x] = rune('a' + (x+y)%26)
			s := DefaultStyle
			// Change color every few cells so runs don't fully coalesce.
			if (x/3+y)%4 != 0 {
				s.FG = 16 + (x*7+y*13)%216
			}
			if (x+y)%17 == 0 {
				s.Bold = true
			}
			st[x] = s
		}
		g.runes = append(g.runes, rr)
		g.styles = append(g.styles, st)
	}
	return g
}

func BenchmarkRenderStyled80x40(b *testing.B) {
	g := benchGrid(80, 40)
	opt := Options{Width: 80, Height: 40, Styled: true, ShowCursor: true}
	g.curX, g.curY, g.curVis = 10, 5, true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Render(g, opt)
	}
}

// benchGridTrueColor mimics a truecolor agent: every cell gets a distinct packed
// 24-bit RGB foreground (v >= 256), exercising the hexColor down-convert path and
// the SGR cache MISS path that the 256-palette benchmark above never touches.
func benchGridTrueColor(w, h int) *fakeGrid {
	g := &fakeGrid{cols: w, rows: h}
	for y := 0; y < h; y++ {
		rr := make([]rune, w)
		st := make([]Style, w)
		for x := 0; x < w; x++ {
			rr[x] = rune('a' + (x+y)%26)
			s := DefaultStyle
			r, gg, b := (x*9)&0xff, (y*7)&0xff, (x*y)&0xff
			s.FG = r<<16 | gg<<8 | b // packed RGB => truecolor path
			st[x] = s
		}
		g.runes = append(g.runes, rr)
		g.styles = append(g.styles, st)
	}
	return g
}

func BenchmarkRenderStyledTrueColor80x40(b *testing.B) {
	g := benchGridTrueColor(80, 40)
	opt := Options{Width: 80, Height: 40, Styled: true, ShowCursor: true}
	g.curX, g.curY, g.curVis = 10, 5, true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Render(g, opt)
	}
}
