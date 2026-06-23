package layout

import "testing"

// TestDividerColumn pins the screen column the divider sits on: LeftInnerWidth+1.
func TestDividerColumn(t *testing.T) {
	cases := []struct {
		left int
		want int
	}{
		{0, 1}, {50, 51}, {80, 81},
	}
	for _, c := range cases {
		l := Layout{LeftInnerWidth: c.left}
		if got := DividerColumn(l); got != c.want {
			t.Errorf("DividerColumn(left=%d) = %d, want %d", c.left, got, c.want)
		}
	}
}

// TestOffsetForDividerRoundTrip verifies that the offset OffsetForDivider returns
// for a target divider column, fed back through ComputeSplit, reproduces a
// divider at exactly that column (LeftInnerWidth+1 == dividerCol) when unclamped.
func TestOffsetForDividerRoundTrip(t *testing.T) {
	b := DefaultBorders()
	const w = 104 // content = 100, evenLeft = 50
	cases := []struct {
		dividerCol int
		wantOffset int
	}{
		{51, 0},   // target left 50, offset 0
		{71, 20},  // target left 70, offset +20
		{31, -20}, // target left 30, offset -20
	}
	for _, c := range cases {
		off := OffsetForDivider(w, b, c.dividerCol)
		if off != c.wantOffset {
			t.Errorf("OffsetForDivider(div=%d) = %d, want %d", c.dividerCol, off, c.wantOffset)
		}
		l := ComputeSplit(w, 40, b, off)
		if got := DividerColumn(l); got != c.dividerCol {
			t.Errorf("round-trip div=%d: ComputeSplit divider = %d (left=%d)", c.dividerCol, got, l.LeftInnerWidth)
		}
		if got := l.LeftInnerWidth; got != c.dividerCol-1 {
			t.Errorf("round-trip div=%d: LeftInnerWidth = %d, want %d", c.dividerCol, got, c.dividerCol-1)
		}
	}
}

// TestOffsetForDividerClampPinsBoard checks that an out-of-range target column is
// pinned by the ComputeSplit board clamp, not by a second clamp here: a far-right
// column leaves the right pane at exactly MinBoardWidth (not TooSmall), a far-left
// column never produces negative widths, and every column is accounted for.
func TestOffsetForDividerClampPinsBoard(t *testing.T) {
	b := DefaultBorders()
	const w = 104 // content = 100

	// Far right: divider dragged past the right edge pins RightInnerWidth at the
	// board minimum and LeftInnerWidth at content-MinBoardWidth (= 80).
	right := ComputeSplit(w, 40, b, OffsetForDivider(w, b, 200))
	if right.RightInnerWidth != MinBoardWidth {
		t.Errorf("far-right: right inner = %d, want MinBoardWidth %d", right.RightInnerWidth, MinBoardWidth)
	}
	if right.LeftInnerWidth != 100-MinBoardWidth {
		t.Errorf("far-right: left inner = %d, want %d", right.LeftInnerWidth, 100-MinBoardWidth)
	}
	if right.TooSmall {
		t.Errorf("far-right: pinned at board minimum must not be TooSmall: %+v", right)
	}
	if got := TotalColumnsAccountedFor(right, b); got != w {
		t.Errorf("far-right: columns accounted = %d, want %d", got, w)
	}

	// Far left: divider dragged off the left edge never goes negative.
	for _, col := range []int{0, -50} {
		left := ComputeSplit(w, 40, b, OffsetForDivider(w, b, col))
		if left.LeftInnerWidth < 0 || left.RightInnerWidth < 0 {
			t.Errorf("far-left col=%d: negative widths %+v", col, left)
		}
		if got := TotalColumnsAccountedFor(left, b); got != w {
			t.Errorf("far-left col=%d: columns accounted = %d, want %d", col, got, w)
		}
	}
}

// TestHitDivider checks the ±tol band: with tol=1 the divider column and its two
// neighbours hit, while ±2 misses. tol=0 only hits the exact column.
func TestHitDivider(t *testing.T) {
	l := Layout{LeftInnerWidth: 50} // divider at column 51
	d := DividerColumn(l)

	// tol = 1: d-1, d, d+1 hit; d-2 and d+2 miss.
	for _, x := range []int{d - 1, d, d + 1} {
		if !HitDivider(l, x, 1) {
			t.Errorf("HitDivider tol=1 x=%d (div=%d) = false, want true", x, d)
		}
	}
	for _, x := range []int{d - 2, d + 2} {
		if HitDivider(l, x, 1) {
			t.Errorf("HitDivider tol=1 x=%d (div=%d) = true, want false", x, d)
		}
	}

	// tol = 0: exact column only.
	if !HitDivider(l, d, 0) {
		t.Errorf("HitDivider tol=0 at exact divider = false, want true")
	}
	for _, x := range []int{d - 1, d + 1} {
		if HitDivider(l, x, 0) {
			t.Errorf("HitDivider tol=0 x=%d = true, want false (exact only)", x)
		}
	}

	// Negative tolerance: the |x-d| <= tol test can never be satisfied by a
	// non-negative distance, so a negative tol never hits — not even the exact
	// divider column. Locks that a bad caller can't widen the band by going
	// negative.
	for _, x := range []int{d, d - 1, d + 1} {
		if HitDivider(l, x, -1) {
			t.Errorf("HitDivider tol=-1 x=%d = true, want false (negative tol never hits)", x)
		}
	}
}

// TestResolveDrag exercises the convenience composer: ResolveDrag(w,h,b,x) equals
// ComputeSplit with the OffsetForDivider(x) offset, and puts the divider at x
// (within the clamp).
func TestResolveDrag(t *testing.T) {
	b := DefaultBorders()
	const w, h = 104, 40
	for _, x := range []int{31, 51, 71} {
		got := ResolveDrag(w, h, b, x)
		want := ComputeSplit(w, h, b, OffsetForDivider(w, b, x))
		if got != want {
			t.Errorf("ResolveDrag x=%d = %+v, want %+v", x, got, want)
		}
		if DividerColumn(got) != x {
			t.Errorf("ResolveDrag x=%d: divider at %d, want %d", x, DividerColumn(got), x)
		}
	}
}

// TestDragRoundTripSweep sweeps several widths and in-range divider columns: each
// column round-trips to LeftInnerWidth == dividerCol-1 (subject to the clamp), and
// resolving the SAME column twice is idempotent.
func TestDragRoundTripSweep(t *testing.T) {
	b := DefaultBorders()
	for _, w := range []int{40, 60, 80, 81, 100, 104} {
		content := w - b.horizontalBordersTotal()
		// In-range columns keep the right pane >= MinBoardWidth so the clamp does
		// not bite: left target in [1, content-MinBoardWidth] => divider in
		// [2, content-MinBoardWidth+1].
		maxLeft := content - MinBoardWidth
		for left := 1; left <= maxLeft; left += 7 {
			dividerCol := left + 1
			l := ResolveDrag(w, 40, b, dividerCol)
			if l.LeftInnerWidth != dividerCol-1 {
				t.Errorf("w=%d div=%d: left = %d, want %d", w, dividerCol, l.LeftInnerWidth, dividerCol-1)
			}
			// Idempotent: resolving the produced divider column again is stable.
			l2 := ResolveDrag(w, 40, b, DividerColumn(l))
			if l2 != l {
				t.Errorf("w=%d div=%d: not idempotent %+v vs %+v", w, dividerCol, l2, l)
			}
			if got := TotalColumnsAccountedFor(l, b); got != w {
				t.Errorf("w=%d div=%d: columns accounted = %d, want %d", w, dividerCol, got, w)
			}
		}
	}
}

// TestClampedOffsetForDivider verifies the clamp round-trip: for an in-range
// target column the clamped offset equals the raw OffsetForDivider, but for an
// out-of-range (far-right) drag it returns the offset back-derived from the
// CLAMPED LeftInnerWidth — feeding it back through ComputeSplit reproduces the
// same divider, and re-feeding is stable (idempotent). This is the value the
// model stores so mouse and keyboard paths stay in sync.
func TestClampedOffsetForDivider(t *testing.T) {
	b := DefaultBorders()
	const w, h = 100, 40 // content 96, evenLeft 48, maxLeft 96-20 = 76

	// In range: same as the raw offset (no clamp bites).
	if got, want := ClampedOffsetForDivider(w, h, b, 60), OffsetForDivider(w, b, 60); got != want {
		t.Errorf("in-range col 60: clamped offset %d, want raw %d", got, want)
	}

	// Far right: the raw offset overshoots, the clamped one parks at the pinned
	// width. content 96, evenLeft 48, clamped left 76 => clamped offset 28.
	clamped := ClampedOffsetForDivider(w, h, b, w+50)
	if clamped != 76-48 {
		t.Errorf("far-right clamped offset = %d, want %d", clamped, 76-48)
	}
	raw := OffsetForDivider(w, b, w+50)
	if clamped >= raw {
		t.Errorf("far-right clamped offset %d should be smaller than the pre-clamp raw %d", clamped, raw)
	}

	// Round-trip: the clamped offset reproduces the pinned layout, and re-deriving
	// from the resulting divider column is idempotent.
	l := ComputeSplit(w, h, b, clamped)
	if l.LeftInnerWidth != 76 {
		t.Errorf("clamped offset re-fed: left = %d, want 76", l.LeftInnerWidth)
	}
	if again := ClampedOffsetForDivider(w, h, b, DividerColumn(l)); again != clamped {
		t.Errorf("clamped offset not idempotent under re-feed: %d -> %d", clamped, again)
	}

	// One keyboard step left (offset - 4) must move the divider exactly one step,
	// not sit in dead travel — the bug the clamp round-trip fixes.
	stepped := ComputeSplit(w, h, b, clamped-4)
	if got, want := DividerColumn(stepped), DividerColumn(l)-4; got != want {
		t.Errorf("one keyboard step from the pinned position: divider %d, want %d", got, want)
	}
}

// TestDragNarrowTerminal makes sure the offset math does not panic and never
// produces negative widths when content < MinBoardWidth (and even content == 0).
// For w >= the border total (4) every content column is still accounted for;
// below that content clamps to 0, so the accounted columns equal the border total.
func TestDragNarrowTerminal(t *testing.T) {
	b := DefaultBorders()
	borders := b.horizontalBordersTotal()
	for _, w := range []int{18, 4, 3, 0} { // content = 14, 0, <0, <0
		for _, col := range []int{-10, 0, 5, 50} {
			off := OffsetForDivider(w, b, col)
			l := ComputeSplit(w, 30, b, off)
			if l.LeftInnerWidth < 0 || l.RightInnerWidth < 0 {
				t.Errorf("w=%d col=%d: negative widths %+v", w, col, l)
			}
			wantCols := w
			if w < borders {
				wantCols = borders // content clamped to 0, only borders remain
			}
			if got := TotalColumnsAccountedFor(l, b); got != wantCols {
				t.Errorf("w=%d col=%d: columns accounted = %d, want %d", w, col, got, wantCols)
			}
		}
	}
}
