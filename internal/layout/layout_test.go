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
	// inner height = H - status(1) - tab(1) - top(1) - bottom(1) = 40 - 4 = 36
	want := 40 - StatusRows - TabRows - b.Top - b.Bottom
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

func TestComputeSplitZeroEqualsCompute(t *testing.T) {
	b := DefaultBorders()
	for _, w := range []int{80, 81, 100, 60, 40, 10, 2} {
		a := Compute(w, 40, b)
		c := ComputeSplit(w, 40, b, 0)
		if a != c {
			t.Errorf("W=%d: ComputeSplit offset 0 (%+v) must equal Compute (%+v)", w, c, a)
		}
	}
}

func TestComputeSplitShiftsDivider(t *testing.T) {
	b := DefaultBorders()
	base := Compute(100, 40, b)
	grow := ComputeSplit(100, 40, b, 8)
	if grow.LeftInnerWidth != base.LeftInnerWidth+8 {
		t.Errorf("offset +8: left = %d, want %d", grow.LeftInnerWidth, base.LeftInnerWidth+8)
	}
	if grow.RightInnerWidth != base.RightInnerWidth-8 {
		t.Errorf("offset +8: right = %d, want %d", grow.RightInnerWidth, base.RightInnerWidth-8)
	}
	// No columns lost regardless of offset.
	if got := TotalColumnsAccountedFor(grow, b); got != 100 {
		t.Errorf("offset +8: columns accounted = %d, want 100", got)
	}
}

func TestComputeSplitKeepsBoardPlayable(t *testing.T) {
	b := DefaultBorders()
	// A huge positive offset would zero the right pane, but the clamp keeps it
	// at least MinBoardWidth so Tetris stays playable.
	l := ComputeSplit(100, 40, b, 1000)
	if l.RightInnerWidth < MinBoardWidth {
		t.Errorf("right inner %d clamped below MinBoardWidth %d", l.RightInnerWidth, MinBoardWidth)
	}
	if l.TooSmall {
		t.Errorf("right pinned at the board minimum should not be TooSmall: %+v", l)
	}
	if got := TotalColumnsAccountedFor(l, b); got != 100 {
		t.Errorf("clamped offset: columns accounted = %d, want 100", got)
	}
}

func TestComputeSplitNegativeShrinksLeft(t *testing.T) {
	b := DefaultBorders()
	// A large negative offset shrinks the left pane toward zero (user's choice)
	// without ever going negative.
	l := ComputeSplit(100, 40, b, -1000)
	if l.LeftInnerWidth < 0 || l.RightInnerWidth < 0 {
		t.Errorf("widths must never be negative: %+v", l)
	}
	if got := TotalColumnsAccountedFor(l, b); got != 100 {
		t.Errorf("negative offset: columns accounted = %d, want 100", got)
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

func TestOffsetForRatio(t *testing.T) {
	b := DefaultBorders()
	// Width 104 => content = 104 - 2*(1+1) = 100, evenLeft = 50.
	const w = 104
	cases := []struct {
		l, r     int
		wantLeft int // desired left inner width before ComputeSplit clamping
	}{
		{1, 1, 50}, // even
		{2, 1, 66}, // 100*2/3 = 66
		{1, 2, 33}, // 100*1/3 = 33
		{2, 2, 50}, // 2:2 reduces to 1:1
	}
	for _, c := range cases {
		off := OffsetForRatio(w, b, c.l, c.r)
		// Feeding the offset to ComputeSplit must yield ~the desired left width
		// (subject to the board-protection clamp, which doesn't bite at 104 wide).
		got := ComputeSplit(w, 40, b, off).LeftInnerWidth
		if got != c.wantLeft {
			t.Errorf("ratio %d:%d -> left=%d, want %d (offset=%d)", c.l, c.r, got, c.wantLeft, off)
		}
		// Every column must still be accounted for (no leaks).
		if total := TotalColumnsAccountedFor(ComputeSplit(w, 40, b, off), b); total != w {
			t.Errorf("ratio %d:%d columns accounted = %d, want %d", c.l, c.r, total, w)
		}
	}

	// Non-positive ratios fall back to the even split (offset 0).
	if off := OffsetForRatio(w, b, 0, 1); off != 0 {
		t.Errorf("zero ratio should yield offset 0, got %d", off)
	}
}
