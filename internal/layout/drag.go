package layout

// drag.go holds the pure (TUI-free) arithmetic that turns a mouse X column into
// a divider position, plus the divider hit-test. The model layer (internal/app)
// is a thin glue over these: it owns a DragState, calls HitDivider on a computed
// Layout to decide whether a press grabbed the divider, and on each drag motion
// feeds the mouse X through ResolveDrag/OffsetForDivider to get the new
// splitOffset. Keeping the math here means it can be unit-tested without a
// terminal and the model stays free of geometry.

// DragState is the divider-drag session state the model embeds. Active is true
// between the press that grabbed the divider and the release that ends it; while
// Active, each mouse-motion event reflows the panes to follow the cursor.
type DragState struct {
	Active bool
}

// DividerColumn returns the 0-based screen column the divider currently sits on
// for the given layout. With the standard column mapping (col 0 = left pane left
// border, cols 1..LeftInnerWidth = left inner, col LeftInnerWidth+1 = the left
// pane's right border == the divider), the divider is at LeftInnerWidth+1.
func DividerColumn(l Layout) int {
	return l.LeftInnerWidth + 1
}

// HitDivider reports whether screen column x is within tol columns of the
// divider in layout l, i.e. |x - dividerCol| <= tol. With tol=1 a click on the
// divider itself, the last left-inner column, or the right pane's left border
// all grab it, so the user does not have to land on a single 1-column target.
func HitDivider(l Layout, x, tol int) bool {
	d := DividerColumn(l)
	diff := x - d
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}

// OffsetForDivider returns the splitOffset that places the divider at screen
// column dividerCol for a terminal of width w with borders b, BEFORE the
// board-protection clamp. Because the divider lives at LeftInnerWidth+1, the
// target left inner width is dividerCol-1, and ComputeSplit derives the left
// inner width as evenLeft+splitOffset; so the offset is (dividerCol-1) - evenLeft
// where evenLeft = (content+1)/2. Feed the result to ComputeSplit, which
// re-applies the MinBoardWidth clamp, so callers never clamp themselves and the
// Tetris board always stays playable no matter how far the cursor is dragged.
func OffsetForDivider(w int, b Borders, dividerCol int) int {
	content := w - b.horizontalBordersTotal()
	if content < 0 {
		content = 0
	}
	evenLeft := (content + 1) / 2
	desiredLeft := dividerCol - 1
	return desiredLeft - evenLeft
}

// ResolveDrag converts a drag to mouse column x into the clamped Layout that the
// divider drag should produce for a w x h terminal with borders b. It is a
// convenience that composes OffsetForDivider with ComputeSplit (the single
// source of clamping), so a test can assert the post-clamp geometry directly
// from a target column. The model uses OffsetForDivider for the raw splitOffset
// it stores and then recomputes the layout itself; ResolveDrag exists so the
// pure round-trip (column -> offset -> clamped layout) reads naturally in tests.
func ResolveDrag(w, h int, b Borders, x int) Layout {
	off := OffsetForDivider(w, b, x)
	return ComputeSplit(w, h, b, off)
}

// ClampedOffsetForDivider returns the splitOffset that places the divider as
// close to screen column x as the board-protection clamp allows, ALREADY clamped
// to the valid range. It composes OffsetForDivider + ComputeSplit (the single
// source of clamping) and then back-derives the offset from the resulting
// LeftInnerWidth (LeftInnerWidth - evenLeft). The model stores THIS offset on a
// drag so the value it keeps matches the divider the user actually sees: feeding
// it back through ComputeSplit reproduces the same layout, and a keyboard nudge
// (C-b </>) steps from the clamped position by exactly one splitStep instead of
// burning through the dead travel hidden in a pre-clamp offset.
func ClampedOffsetForDivider(w, h int, b Borders, x int) int {
	l := ComputeSplit(w, h, b, OffsetForDivider(w, b, x))
	content := w - b.horizontalBordersTotal()
	if content < 0 {
		content = 0
	}
	evenLeft := (content + 1) / 2
	return l.LeftInnerWidth - evenLeft
}
