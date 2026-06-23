package app

// windows.go holds the cmux-style command-window management glue: opening,
// cycling, selecting, and closing the parallel command windows. Each of these is
// a window transition, so it first ends any in-progress divider drag (endDrag, in
// view.go) — a transition must never leave the divider glued to the cursor with a
// lost release stranding drag.Active=true.

// newWindow opens a fresh command window running $SHELL (so you can launch any
// agent in it), sized to the current layout, and switches to it. Best-effort: a
// spawn failure or hitting maxWindows is silently ignored.
func (m *Model) newWindow() {
	m.endDrag() // a window transition ends any in-progress divider drag
	if len(m.windows) >= maxWindows {
		return
	}
	l := m.layout()
	cols, rows := maxInt(l.LeftInnerWidth, 1), maxInt(l.InnerHeight, 1)
	lp, err := newLeftPane(nil, cols, rows, m.notify()) // nil argv => $SHELL
	if err != nil {
		return
	}
	m.windows = append(m.windows, lp)
	m.active = len(m.windows) - 1
}

// cycleWindow moves the visible window by delta with wraparound.
func (m *Model) cycleWindow(delta int) {
	m.endDrag() // a window transition ends any in-progress divider drag
	n := len(m.windows)
	if n <= 1 {
		return
	}
	m.active = ((m.active+delta)%n + n) % n
	if w := m.cur(); w != nil {
		w.ConsumeDirty() // re-arm its notify; the redraw shows its latest output
	}
}

// selectWindow jumps to the 0-based window index, ignoring out-of-range values.
func (m *Model) selectWindow(i int) {
	m.endDrag() // a window transition ends any in-progress divider drag
	if i < 0 || i >= len(m.windows) {
		return
	}
	m.active = i
	if w := m.cur(); w != nil {
		w.ConsumeDirty()
	}
}

// closeWindow closes the current window, keeping at least one open.
func (m *Model) closeWindow() {
	m.endDrag() // a window transition ends any in-progress divider drag
	if len(m.windows) <= 1 {
		return
	}
	m.windows[m.active].Close()
	m.windows = append(m.windows[:m.active], m.windows[m.active+1:]...)
	if m.active >= len(m.windows) {
		m.active = len(m.windows) - 1
	}
	if w := m.cur(); w != nil {
		w.ConsumeDirty()
	}
}
