package layout

import "testing"

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func TestEvenWidthSplit(t *testing.T) {
	b := DefaultBorders()
	l := Compute(80, 40, b)
	if absInt(l.LeftInnerWidth-l.RightInnerWidth) > 1 {
		t.Errorf("halves differ by more than 1: %d vs %d", l.LeftInnerWidth, l.RightInnerWidth)
	}
	if l.LeftInnerWidth <= 0 || l.RightInnerWidth <= 0 {
		t.Errorf("inner widths must be > 0 for a normal terminal: %d, %d", l.LeftInnerWidth, l.RightInnerWidth)
	}
}

func TestOddWidthSplit(t *testing.T) {
	b := DefaultBorders()
	l := Compute(81, 40, b)
	if absInt(l.LeftInnerWidth-l.RightInnerWidth) > 1 {
		t.Errorf("odd-width halves differ by more than 1: %d vs %d", l.LeftInnerWidth, l.RightInnerWidth)
	}
	if l.LeftInnerWidth < 0 || l.RightInnerWidth < 0 {
		t.Errorf("inner widths must never be negative")
	}
}

func TestTooSmallWidth(t *testing.T) {
	b := DefaultBorders()
	// Very narrow terminal cannot fit the board.
	l := Compute(10, 40, b)
	if !l.TooSmall {
		t.Errorf("expected TooSmall for narrow terminal")
	}
	if l.LeftInnerWidth < 0 || l.RightInnerWidth < 0 || l.InnerHeight < 0 {
		t.Errorf("dims must clamp to >= 0, got %+v", l)
	}
}

func TestTooSmallExtreme(t *testing.T) {
	b := DefaultBorders()
	// Width smaller than the borders themselves.
	l := Compute(2, 2, b)
	if !l.TooSmall {
		t.Errorf("expected TooSmall")
	}
	if l.LeftInnerWidth < 0 || l.RightInnerWidth < 0 || l.InnerHeight < 0 {
		t.Errorf("dims must never be negative: %+v", l)
	}
}

func TestHeightCalc(t *testing.T) {
	b := DefaultBorders()
	l := Compute(80, 40, b)
	// inner height = H - status(1) - top(1) - bottom(1) = 40 - 3 = 37
	want := 40 - StatusRows - b.Top - b.Bottom
	if l.InnerHeight != want {
		t.Errorf("inner height = %d, want %d", l.InnerHeight, want)
	}
	if l.InnerHeight <= 0 {
		t.Errorf("inner height must be > 0 for normal terminal")
	}
}

func TestHeightTooSmall(t *testing.T) {
	b := DefaultBorders()
	// Height below what the 20-row board needs.
	l := Compute(80, 10, b)
	if !l.TooSmall {
		t.Errorf("expected TooSmall for short terminal")
	}
	if l.InnerHeight < 0 {
		t.Errorf("inner height must never be negative")
	}
}

func TestHeightNeverNegative(t *testing.T) {
	b := DefaultBorders()
	l := Compute(80, 1, b)
	if l.InnerHeight < 0 {
		t.Errorf("inner height negative: %d", l.InnerHeight)
	}
	l0 := Compute(80, 0, b)
	if l0.InnerHeight < 0 {
		t.Errorf("inner height negative for H=0: %d", l0.InnerHeight)
	}
}

func TestBorderAccounting(t *testing.T) {
	b := DefaultBorders()
	for _, w := range []int{80, 81, 100, 60, 40} {
		l := Compute(w, 40, b)
		total := TotalColumnsAccountedFor(l, b)
		if total != w {
			t.Errorf("W=%d: columns accounted = %d, want %d (no lost/overcounted cols)", w, total, w)
		}
	}
}

func TestBorderAccountingCustom(t *testing.T) {
	b := Borders{Left: 1, Right: 1, Top: 2, Bottom: 1}
	l := Compute(100, 50, b)
	total := TotalColumnsAccountedFor(l, b)
	if total != 100 {
		t.Errorf("custom borders: columns accounted = %d, want 100", total)
	}
}

func TestNormalTerminalNotTooSmall(t *testing.T) {
	b := DefaultBorders()
	// A comfortable terminal: right inner must be >= MinBoardWidth and inner
	// height >= MinBoardHeight.
	l := Compute(100, 40, b)
	if l.TooSmall {
		t.Errorf("100x40 should not be too small: %+v", l)
	}
	if l.RightInnerWidth < MinBoardWidth {
		t.Errorf("right inner %d < min %d", l.RightInnerWidth, MinBoardWidth)
	}
	if l.InnerHeight < MinBoardHeight {
		t.Errorf("inner height %d < min %d", l.InnerHeight, MinBoardHeight)
	}
}
