// Package layout computes the split-pane geometry for tetmux. It is pure (no
// TUI deps) so the width/height arithmetic can be tested directly.
//
// The terminal is split into two vertical halves with borders, plus a single
// status-bar row reserved at the bottom (focus indicator + score). Pane inner
// dimensions exclude border columns/rows.
package layout

const (
	// MinBoardWidth is the minimum inner width the right pane needs to draw the
	// 10-wide Tetris board (2 terminal columns per block => 20 columns).
	MinBoardWidth = 20
	// MinBoardHeight is the minimum inner height the right pane needs for the
	// 20-row board.
	MinBoardHeight = 20
	// StatusRows is the number of rows reserved for the status bar.
	StatusRows = 1
)

// Borders describes the per-pane border thickness. For a standard lipgloss
// rounded/normal border each pane has a 1-column border on the left and right
// and a 1-row border on top and bottom.
type Borders struct {
	Left   int // columns consumed by each pane's left border
	Right  int // columns consumed by each pane's right border
	Top    int // rows consumed by each pane's top border
	Bottom int // rows consumed by each pane's bottom border
}

// DefaultBorders returns the standard single-line border on all sides.
func DefaultBorders() Borders {
	return Borders{Left: 1, Right: 1, Top: 1, Bottom: 1}
}

// horizontalBorders returns the total border columns consumed across BOTH
// panes (each pane has its own left+right border).
func (b Borders) horizontalBordersTotal() int {
	return 2 * (b.Left + b.Right)
}

// verticalBorders returns the rows consumed by one pane's top+bottom borders.
func (b Borders) verticalBorders() int {
	return b.Top + b.Bottom
}

// Layout is the computed geometry.
type Layout struct {
	LeftInnerWidth  int
	RightInnerWidth int
	InnerHeight     int
	StatusRow       int  // y position (row index) of the status bar
	TooSmall        bool // true if the terminal cannot fit the board
}

// Compute splits a W x H terminal into two bordered panes plus a status row.
//
// Width: total border columns are subtracted; the remaining content width is
// split into two inner widths that differ by at most 1 (left gets the extra
// odd column). Neither inner width is ever negative.
//
// Height: the status row and each pane's top+bottom borders are subtracted to
// give the pane inner height; never negative.
//
// TooSmall is set when either pane inner width or the inner height falls below
// what the Tetris board needs, OR when subtracting borders/status would make a
// dimension negative.
func Compute(w, h int, b Borders) Layout {
	var l Layout

	// --- width ---
	content := w - b.horizontalBordersTotal()
	if content < 0 {
		content = 0
	}
	left := content / 2
	right := content - left
	// Give the extra odd column to the left pane for a stable, testable rule.
	if content%2 == 1 {
		left = (content + 1) / 2
		right = content - left
	}
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	l.LeftInnerWidth = left
	l.RightInnerWidth = right

	// --- height ---
	usableH := h - StatusRows
	if usableH < 0 {
		usableH = 0
	}
	inner := usableH - b.verticalBorders()
	if inner < 0 {
		inner = 0
	}
	l.InnerHeight = inner

	// Status row sits on the very last terminal row.
	if h > 0 {
		l.StatusRow = h - 1
	} else {
		l.StatusRow = 0
	}

	// --- too-small flag ---
	// The right pane (Tetris) needs MinBoardWidth inner cols and MinBoardHeight
	// inner rows. If the original terminal couldn't even afford the borders,
	// the clamped-to-zero dims already reflect that.
	if l.RightInnerWidth < MinBoardWidth || l.InnerHeight < MinBoardHeight {
		l.TooSmall = true
	}

	return l
}

// TotalColumnsAccountedFor returns leftInner + rightInner + all border columns
// for the given layout/borders. It is used by tests to assert no columns are
// lost or double-counted in the split.
func TotalColumnsAccountedFor(l Layout, b Borders) int {
	return l.LeftInnerWidth + l.RightInnerWidth + b.horizontalBordersTotal()
}
