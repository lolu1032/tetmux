package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/layout"
	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

// View composes the two panes and a status bar.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "initializing tetmux..."
	}
	l := m.layout()

	m.updateHWCursor(l)

	tabs, _ := m.tabBar()
	leftBox := m.renderLeft(l)
	rightBox := m.renderRight(l)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	status := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left, tabs, body, status)
}

func (m *Model) renderLeft(l layout.Layout) string {
	w := l.LeftInnerWidth
	h := l.InnerHeight
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	focused := m.rstate.Focus == router.FocusLeft
	cur := m.cur()
	var content string
	switch {
	case m.spawnErr != nil:
		content = leftErrorContent(m.argv, m.spawnErr, w, h)
	case cur == nil:
		content = "starting..."
	default:
		// When the IME cursor fix is on, the real (hardware) terminal cursor is
		// shown on the input cell, so drawing the synthetic reverse block too would
		// double it; let the real cursor be the one indicator. With the fix off
		// (TETMUX_NO_IME_CURSOR) fall back to the synthetic block.
		showSynthetic := focused && !m.imeCursor
		rows := cur.RenderRows(w, h, showSynthetic)
		if cur.Exited() && len(rows) > 0 {
			rows[0] = clipPad(exitBanner(cur.ExitCode()), w)
		}
		content = joinRows(rows)
	}

	border := lipgloss.RoundedBorder()
	style := m.renderer.NewStyle().Border(border).Width(w).Height(h)
	if focused {
		style = style.BorderForeground(lipgloss.Color("51"))
	} else {
		style = style.BorderForeground(lipgloss.Color("240"))
	}
	return style.Render(content)
}

func (m *Model) renderRight(l layout.Layout) string {
	w := l.RightInnerWidth
	h := l.InnerHeight
	focused := m.rstate.Focus == router.FocusRight

	border := lipgloss.RoundedBorder()
	style := m.renderer.NewStyle().Border(border).Width(maxInt(w, 1)).Height(maxInt(h, 1))
	if focused {
		style = style.BorderForeground(lipgloss.Color("226"))
	} else {
		style = style.BorderForeground(lipgloss.Color("240"))
	}

	if l.TooSmall {
		return style.Render("window too small")
	}

	// The Paused / GameOver overlays are INTERACTIVE modals (their Resume /
	// Restart keys only work when the game pane has focus). Show them only while
	// the game is focused: when the command pane is focused those keys route to
	// the child instead, so an overlay shown then would be an unreachable,
	// undismissable modal contradicting the status bar's focus:LEFT. With the
	// command pane focused we fall through to the plain board (a non-modal,
	// background view) so focus and what's drawn never disagree.
	if focused {
		// While stopped, show a centered pause menu over the whole pane instead of
		// the board — the "일시정지 / 계속하기" overlay.
		if m.game.State() == tetris.Paused {
			menu := renderPauseMenu(m.renderer, m.pauseSel)
			body := lipgloss.Place(maxInt(w, 1), maxInt(h, 1), lipgloss.Center, lipgloss.Center, menu)
			return style.Render(body)
		}
		// After a loss, replace the board with the game-over overlay (final score +
		// the 재시작 prompt), mirroring the pause overlay.
		if m.game.State() == tetris.GameOver {
			menu := renderGameOverMenu(m.renderer, m.game.Score, m.best)
			body := lipgloss.Place(maxInt(w, 1), maxInt(h, 1), lipgloss.Center, lipgloss.Center, menu)
			return style.Render(body)
		}
	}

	board := joinRows(renderTetris(m.game, m.renderer))
	// Show the HOLD/NEXT side panel beside the board when the pane is wide
	// enough for the 20-col board plus a gap and the preview column.
	if w >= tetris.Width*2+previewGap+previewCols {
		side := renderSidePanel(m.game, m.renderer)
		gap := m.renderer.NewStyle().Width(previewGap).Render("")
		board = lipgloss.JoinHorizontal(lipgloss.Top, board, gap, side)
	}
	return style.Render(board)
}

// tabHit records the column span [start,end) of one clickable tab-bar segment.
type tabHit struct {
	start, end int
	win        int  // window index to select
	isNew      bool // the [+] new-window button
}

// tabBar renders the clickable cmux-style window tab bar across the top row,
// returning the styled string and the hit map for mouse clicks. Tab labels are
// ASCII so rune offsets equal display columns.
func (m *Model) tabBar() (string, []tabHit) {
	var b strings.Builder
	var hits []tabHit
	x := 0
	add := func(label, styled string, win int, isNew bool) {
		n := len([]rune(label))
		hits = append(hits, tabHit{start: x, end: x + n, win: win, isNew: isNew})
		b.WriteString(styled)
		x += n
	}
	for i, w := range m.windows {
		// Mark an exited window with a trailing ✗ (a width-1 rune, so the hit-map
		// column math still holds) so a dead background job is visible in the tab
		// bar without switching to it.
		mark := ""
		if w.Exited() {
			mark = "✗"
		}
		label := " " + itoa(i+1) + ":" + w.Name() + mark + " "
		st := m.renderer.NewStyle()
		switch {
		case i == m.active:
			st = st.Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("51"))
		case w.Exited():
			st = st.Faint(true).Foreground(lipgloss.Color("203")) // dim red for a dead window
		default:
			st = st.Faint(true)
		}
		add(label, st.Render(label), i, false)
	}
	plus := " + "
	add(plus, m.renderer.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(plus), -1, true)
	bar := m.renderer.NewStyle().Width(maxInt(m.width, 1)).Render(b.String())
	return bar, hits
}

// dividerHitTol is how many columns away from the divider a press still grabs it
// for a drag. With 1 the divider column, the last left-inner column, and the
// right pane's left border all start a drag, so the user need not land on a
// single 1-column target. The same tolerance is applied symmetrically by
// layout.HitDivider.
const dividerHitTol = 1

// handleMouse routes left-button mouse events. A press grabs the divider for a
// live drag when it lands near the divider on a pane-body row; otherwise it does
// the usual tab-bar (window select / [+]) or pane-focus action. While a drag is
// active, each motion event moves the divider to follow the cursor and reflows
// every command PTY; the release ends the drag. Non-left buttons and stray
// motions outside a drag are ignored.
func (m *Model) handleMouse(e tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Release ends any drag REGARDLESS of which button it carries: an X10-encoded
	// terminal reports a button-up as Action=Release, Button=MouseButtonNone (only
	// SGR terminals carry Button=Left on release). Handling it above the left-only
	// button guard means the release is never dropped, so a drag can't get stuck
	// Active forever (the divider glued to the cursor, every motion reflowing
	// every PTY). endDrag also flushes the background windows to the final width.
	if e.Action == tea.MouseActionRelease {
		m.endDrag()
		return m, nil
	}
	if e.Button != tea.MouseButtonLeft {
		return m, nil
	}
	switch e.Action {
	case tea.MouseActionMotion:
		// Only meaningful mid-drag; a stray motion with no active drag is ignored.
		if m.drag.Active {
			m.dragDivider(e.X)
			return m, m.scheduleDragReflow()
		}
		return m, nil
	case tea.MouseActionPress:
		// fall through to the press handling below
	default:
		return m, nil
	}

	l := m.layout()
	switch {
	case e.Y < layout.TabRows: // top tab bar
		_, hits := m.tabBar()
		for _, h := range hits {
			if e.X >= h.start && e.X < h.end {
				if h.isNew {
					m.newWindow()
				} else {
					m.selectWindow(h.win)
				}
				break
			}
		}
	case e.Y >= l.StatusRow: // status row (derived from the Layout, not raw height-1)
		// nothing clickable here (yet)
	default: // pane body
		// A fresh press first self-heals any stale drag (e.g. a release that the
		// terminal never delivered) so the hit-test below decides cleanly whether
		// THIS press grabs the divider — a stale Active must never let an unrelated
		// body press act as a continued drag.
		m.endDrag()
		// A press near the divider grabs it for a drag instead of just focusing a
		// pane, so the user can drag the boundary to rebalance the split live. The
		// press itself does NOT move the divider: a pure click (press+release with
		// no motion) leaves the split — and any active ratio preset — byte-for-byte
		// untouched. The first motion event (dragDivider) is what converts to
		// absolute-offset mode and starts tracking the cursor, so the divider never
		// jumps on the grab. Dragging is gated on a drawable board (!TooSmall and a
		// positive body-row count) so a press on a too-short terminal — where the
		// board is invisible — never starts a phantom drag.
		if !l.TooSmall && l.InnerHeight > 0 && layout.HitDivider(l, e.X, dividerHitTol) {
			m.drag.Active = true
			break
		}
		leftBox := maxInt(l.LeftInnerWidth, 1) + 2 // + left/right border
		if e.X < leftBox {
			m.rstate.Focus = router.FocusLeft
		} else {
			m.rstate.Focus = router.FocusRight
		}
		// A click that changes focus drives the game lifecycle the same way Tab
		// does: clicking away from a Playing board auto-pauses it (so gravity
		// stops), clicking back onto an auto-paused board resumes it. Keeping this
		// here means a mouse focus change can't strand the game in a state that
		// contradicts the focus shown in the status bar.
		if cmd := m.reconcileFocus(); cmd != nil {
			return m, cmd
		}
	}
	return m, nil
}

// dragDivider moves the split divider so it tracks mouse column x: it converts x
// into the CLAMPED absolute splitOffset that puts the divider there (leaving
// ratio-preset mode so the divider follows smoothly from wherever it sat). The
// stored offset is back-derived from the post-clamp layout (ClampedOffsetForDivider)
// so the value the model keeps matches the divider the user sees: dragging too far
// right pins the Tetris pane at MinBoardWidth AND parks splitOffset at the clamped
// value, so a later keyboard nudge (C-b </>) steps by exactly one splitStep instead
// of burning through the dead travel a pre-clamp offset would hide. The render
// update is cheap (offset only); the PTY reflow is coalesced to once per frame by
// the dragTickMsg loop, so a fast motion sweep does not Setsize every window per
// event.
func (m *Model) dragDivider(x int) {
	m.splitOffset = layout.ClampedOffsetForDivider(m.width, m.height, layout.DefaultBorders(), x)
	m.ratioIdx = -1
	m.dragDirty = true
}

// dragTickMsg fires one frame after a drag motion scheduled a reflow, flushing
// the coalesced PTY resize of the visible window to the latest divider position.
type dragTickMsg struct{}

// scheduleDragReflow arms a single frame-gated dragTickMsg (reusing the
// frameInterval cadence) so a burst of motions within one frame collapses into
// one PTY reflow. Returns nil when a tick is already pending this frame.
func (m *Model) scheduleDragReflow() tea.Cmd {
	if m.dragTick {
		return nil
	}
	m.dragTick = true
	return tea.Tick(frameInterval, func(time.Time) tea.Msg { return dragTickMsg{} })
}

// handleDragTick flushes the frame-coalesced drag reflow: it resizes the visible
// window to the latest divider position and, if more motion arrived during the
// frame, schedules another tick. A tick that lands after the drag already ended
// is ignored (endDrag cleared dragTick and already flushed all windows).
func (m *Model) handleDragTick() (tea.Model, tea.Cmd) {
	if !m.drag.Active || !m.dragTick {
		return m, nil
	}
	m.dragTick = false
	if m.dragDirty {
		m.dragDirty = false
		m.reflowVisible()
	}
	return m, nil
}

// reflowVisible resizes only the currently visible window's PTY to the current
// layout's left inner dimensions. Used on the drag hot path so a single motion
// touches one PTY instead of every window; the background windows are caught up
// by reflow() on release (see endDrag).
func (m *Model) reflowVisible() {
	l := m.layout()
	cols, rows := maxInt(l.LeftInnerWidth, 1), maxInt(l.InnerHeight, 1)
	if w := m.cur(); w != nil {
		w.Resize(cols, rows)
	}
}

// endDrag is the single chokepoint that clears an in-progress divider drag. It
// is called on release and from every state transition that could otherwise
// strand drag.Active=true (resize, window add/remove/select/cycle, game-over),
// so a single lost release (e.g. an X10 terminal that sends Button=None) can
// never leave the divider permanently glued to the cursor. If a drag was live it
// flushes ALL windows once so the BACKGROUND sessions catch up to the final
// divider width: during the drag only the visible window is resized (reflowVisible
// on each frame tick), so the background PTYs are never touched until here — and
// the flush must NOT be gated on a pending dirty flag, because a frame tick that
// fired just before the release already cleared dragDirty while having resized
// only the visible window, leaving the background windows stale.
func (m *Model) endDrag() {
	wasActive := m.drag.Active
	m.drag = layout.DragState{}
	m.dragDirty = false
	m.dragTick = false
	if wasActive {
		m.reflow() // catch up ALL windows (incl. background) to the final width
	}
}

func (m *Model) renderStatus() string {
	focus := "LEFT"
	if m.rstate.Focus == router.FocusRight {
		focus = "RIGHT"
	}
	prefix := ""
	if m.rstate.PrefixArmed {
		prefix = " [C-b]"
	}
	best := ""
	if m.best > 0 {
		best = " best:" + itoa(m.best)
	}
	// Context hint: Tab toggles between the two. When the board is focused the
	// game keys work directly; otherwise you are typing into the command pane.
	hint := "Tab/click:play | C-b z:ratio C-b q:quit"
	if m.rstate.Focus == router.FocusRight {
		hint = "Tab:back Esc:pause C-c:quit | ←→move x/z:rot space:drop c:hold r:restart"
	}
	if m.ctrlCArmed {
		hint = "press C-c again to quit tetmux"
	}
	// Show the active ratio preset so cycling with C-b z gives visible feedback.
	ratio := ""
	if m.ratioIdx >= 0 && m.ratioIdx < len(ratioPresets) {
		p := ratioPresets[m.ratioIdx]
		ratio = " ratio:" + itoa(p[0]) + ":" + itoa(p[1])
	}
	text := " focus:" + focus + prefix + ratio +
		" | score:" + itoa(m.game.Score) + best +
		" lv:" + itoa(m.game.Level()) +
		" | " + gameStatusLine(m.game) +
		" | " + hint
	return m.renderer.NewStyle().
		Reverse(true).
		Width(maxInt(m.width, 1)).
		Render(clipPad(text, m.width))
}
