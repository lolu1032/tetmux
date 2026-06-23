package app

import (
	"os"
	"sync"

	"tetmux/internal/layout"
	"tetmux/internal/router"
)

// hwCursor is the shared, mutex-guarded "where should the real terminal cursor
// sit" state. The Model writes it from View() (it alone knows the layout and the
// focused command pane's vt cursor); the output writer reads it around each frame
// and drives the hardware cursor's visibility + position.
//
// Why this exists: an IME/CJK composition *preedit* (the half-formed 조합 중 글자
// the OS shows before you commit it) is anchored by the terminal to the VISIBLE
// hardware cursor. bubbletea hides the cursor and parks it bottom-left, so the OS
// draws the preedit there (or at a window default) instead of inline — the
// one-keystroke "lag". Simply moving a HIDDEN cursor isn't enough: macOS anchors
// the preedit to the cursor only when it is shown. So we SHOW the cursor on the
// command pane's own input cell (hiding it again only while a frame is being
// painted, to avoid flicker as bubbletea redraws line by line). ASCII input never
// composes, so it was never affected.
type hwCursor struct {
	mu      sync.Mutex
	managed bool // false => emit nothing; let bubbletea own the cursor
	show    bool // when managed: show+position the cursor (vs keep it hidden)
	row     int  // 1-based terminal row
	col     int  // 1-based terminal column
}

// anchor shows the cursor at (row, col) (1-based) after each frame so the IME
// composes inline there.
func (c *hwCursor) anchor(row, col int) {
	c.mu.Lock()
	c.managed, c.show, c.row, c.col = true, true, row, col
	c.mu.Unlock()
}

// hideCursor keeps the cursor hidden (e.g. the game pane is focused — no text
// input, so no cursor should show in the command pane).
func (c *hwCursor) hideCursor() {
	c.mu.Lock()
	c.managed, c.show = true, false
	c.mu.Unlock()
}

// disable relinquishes cursor control entirely (TETMUX_NO_IME_CURSOR), leaving
// the cursor wherever bubbletea parks it — the pre-fix behavior.
func (c *hwCursor) disable() {
	c.mu.Lock()
	c.managed = false
	c.mu.Unlock()
}

// framePre is emitted BEFORE a frame: hide the cursor so it doesn't streak across
// the screen while bubbletea repaints line by line.
func (c *hwCursor) framePre() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.managed {
		return ""
	}
	return "\x1b[?25l"
}

// framePost is emitted AFTER a frame: re-show the cursor on the anchored input
// cell (so the OS IME composes inline), or leave it hidden (framePre already did).
func (c *hwCursor) framePost() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.managed {
		return ""
	}
	if c.show && c.row >= 1 && c.col >= 1 {
		return "\x1b[?25h\x1b[" + itoa(c.row) + ";" + itoa(c.col) + "H"
	}
	return ""
}

// cursorWriter wraps the program's tty output. bubbletea writes each frame as a
// single buffer; this brackets that write with a cursor hide (before, so the
// repaint doesn't flicker the cursor) and a show+position (after, onto the
// focused pane's input cell) so an IME preedit composes inline. The bracketing
// bytes are not counted in the returned n, so the io.Writer contract
// (n == len(p) on success) holds.
//
// It embeds *os.File rather than wrapping an io.Writer so it still satisfies
// charmbracelet/x/term.File (via the promoted Fd/Read/Close): bubbletea only
// detects the terminal size — and thus ever emits a WindowSizeMsg — when its
// output is a real tty File. A plain io.Writer wrapper would make the app hang
// at startup, never sized, never spawning the PTY.
type cursorWriter struct {
	*os.File
	cs *hwCursor
}

// Write brackets the frame with the cursor hide/show+position escapes.
func (cw *cursorWriter) Write(p []byte) (int, error) {
	if pre := cw.cs.framePre(); pre != "" {
		_, _ = cw.File.Write([]byte(pre))
	}
	n, err := cw.File.Write(p)
	if err != nil {
		return n, err
	}
	if post := cw.cs.framePost(); post != "" {
		_, _ = cw.File.Write([]byte(post))
	}
	return n, err
}

// WrapOutput wraps the program's tty output so the hardware cursor is re-homed
// onto the focused command pane's cursor cell after every frame. Pass the result
// to tea.WithOutput; this is what makes Korean/CJK IME composition appear inline
// instead of lagging by one keystroke in the status row. f must be the real
// stdout *os.File so bubbletea still sees a tty (and thus still detects the
// window size and ever emits a WindowSizeMsg).
func (m *Model) WrapOutput(f *os.File) *cursorWriter {
	return &cursorWriter{File: f, cs: m.hw}
}

// updateHWCursor points the shared hardware-cursor target at the focused command
// pane's vt cursor cell, translated to absolute (1-based) screen coordinates, so
// an IME/CJK composition preedit renders inline there. It deactivates (leaving
// the cursor at bubbletea's park spot) when the game pane is focused, no pane
// exists, the child hid its cursor, or the cursor sits outside the visible pane.
func (m *Model) updateHWCursor(l layout.Layout) {
	if m.hw == nil {
		return
	}
	if !m.imeCursor { // disabled via TETMUX_NO_IME_CURSOR: leave the cursor parked
		m.hw.disable()
		return
	}
	cur := m.cur()
	if m.rstate.Focus != router.FocusLeft || cur == nil {
		m.hw.hideCursor()
		return
	}
	cx, cy, vis := cur.CursorPos()
	if !vis {
		m.hw.hideCursor()
		return
	}
	row, col, active := absCursor(l, cx, cy)
	if !active {
		m.hw.hideCursor()
		return
	}
	m.hw.anchor(row, col)
}

// absCursor maps a command pane vt cursor cell (cx, cy, grid index = display
// column) to absolute 1-based screen coordinates for the hardware-cursor escape,
// or active=false when the cell lies outside the visible pane. Screen geometry
// (0-based): row 0 is the tab bar, row 1 the left pane's top border, so content
// row cy is at screen row TabRows+Top+cy; column 0 is the left border, so content
// col cx is at screen col Left+cx. The returned coordinates are 1-based.
func absCursor(l layout.Layout, cx, cy int) (row, col int, active bool) {
	w, h := maxInt(l.LeftInnerWidth, 1), maxInt(l.InnerHeight, 1)
	if cx < 0 || cy < 0 || cx >= w || cy >= h {
		return 0, 0, false
	}
	b := layout.DefaultBorders()
	return layout.TabRows + b.Top + cy + 1, b.Left + cx + 1, true
}
