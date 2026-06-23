package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/layout"
	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

// mouse builds a left-button mouse event with the given action at (x,y).
func mouse(action tea.MouseAction, x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: action, Button: tea.MouseButtonLeft, X: x, Y: y}
}

// mouseRelease builds a button-RELEASE event the way an X10-encoded terminal
// actually reports it: Action=Release with Button=MouseButtonNone (NOT
// MouseButtonLeft, which only SGR terminals carry on release). The old mouse()
// helper hardcoded Button:Left on every action including release — an event real
// X10 terminals never send — so the release/cancel paths could rot while the
// suite stayed green. Release-path tests must use this helper.
func mouseRelease(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonNone, X: x, Y: y}
}

// dividerCol returns the current divider screen column for m's layout.
func dividerCol(m *Model) int { return layout.DividerColumn(m.layout()) }

// TestDragResizesDivider: a press near the divider then a motion drags the
// divider to follow the cursor, updating splitOffset (so the new LeftInnerWidth
// matches the target column) and clearing ratio mode.
func TestDragResizesDivider(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	if !m.drag.Active {
		t.Fatalf("press at the divider (col %d) should start a drag", d)
	}
	m.Update(mouse(tea.MouseActionMotion, d+15, 10))
	if m.ratioIdx != -1 {
		t.Errorf("drag should be in absolute-offset mode, ratioIdx=%d", m.ratioIdx)
	}
	if got, want := m.layout().LeftInnerWidth, (d+15)-1; got != want {
		t.Errorf("after motion to col %d: left inner = %d, want %d", d+15, got, want)
	}
}

// TestMotionWithoutPressDoesNothing: a stray motion with no active drag must not
// touch the split.
func TestMotionWithoutPressDoesNothing(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	before := m.splitOffset
	m.Update(mouse(tea.MouseActionMotion, 70, 10))
	if m.drag.Active {
		t.Errorf("motion alone must not start a drag")
	}
	if m.splitOffset != before {
		t.Errorf("stray motion changed splitOffset %d -> %d", before, m.splitOffset)
	}
}

// TestReleaseEndsDrag: after press+motion, a release ends the drag and a later
// motion at a new X no longer moves the divider.
func TestReleaseEndsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionMotion, d+10, 10))
	m.Update(mouse(tea.MouseActionRelease, d+10, 10))
	if m.drag.Active {
		t.Fatalf("release should end the drag")
	}
	frozen := m.splitOffset
	m.Update(mouse(tea.MouseActionMotion, d+30, 10))
	if m.splitOffset != frozen {
		t.Errorf("post-release motion changed splitOffset %d -> %d", frozen, m.splitOffset)
	}
}

// TestDragFarRightPinsBoard: dragging the divider way past the right edge leaves
// the Tetris pane at exactly MinBoardWidth (via the ComputeSplit clamp) and not
// TooSmall.
func TestDragFarRightPinsBoard(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionMotion, m.width+50, 10))
	l := m.layout()
	if l.RightInnerWidth != layout.MinBoardWidth {
		t.Errorf("far-right drag: right inner = %d, want MinBoardWidth %d", l.RightInnerWidth, layout.MinBoardWidth)
	}
	if l.TooSmall {
		t.Errorf("far-right drag pinned at board minimum must not be TooSmall: %+v", l)
	}
}

// TestDragTabBarRowDoesNotStart: a press on the tab-bar row (y==0) at the divider
// column does the window-select behavior, not a drag; a later motion is inert.
func TestDragTabBarRowDoesNotStart(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)
	before := m.splitOffset

	m.Update(mouse(tea.MouseActionPress, d, 0))
	if m.drag.Active {
		t.Fatalf("press on the tab bar must not start a drag")
	}
	m.Update(mouse(tea.MouseActionMotion, d+20, 0))
	if m.splitOffset != before {
		t.Errorf("tab-bar press+motion changed splitOffset %d -> %d", before, m.splitOffset)
	}
}

// TestDragStatusRowDoesNotStart: a press on the status row (y==height-1) at the
// divider column must not start a drag.
func TestDragStatusRowDoesNotStart(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)
	before := m.splitOffset

	m.Update(mouse(tea.MouseActionPress, d, m.height-1))
	if m.drag.Active {
		t.Fatalf("press on the status row must not start a drag")
	}
	m.Update(mouse(tea.MouseActionMotion, d+20, m.height-1))
	if m.splitOffset != before {
		t.Errorf("status-row press+motion changed splitOffset %d -> %d", before, m.splitOffset)
	}
}

// TestPaneBodyClickStillFocuses guards the existing focus behavior: a body press
// far from the divider focuses the clicked pane and does NOT start a drag.
func TestPaneBodyClickStillFocuses(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40

	m.Update(mouse(tea.MouseActionPress, m.width-3, 5)) // far right → game pane
	if m.drag.Active {
		t.Fatalf("a far-right body click must not start a drag")
	}
	if m.rstate.Focus != router.FocusRight {
		t.Errorf("far-right body click should focus the game pane, got %v", m.rstate.Focus)
	}
	m.Update(mouse(tea.MouseActionPress, 2, 5)) // far left → command pane
	if m.drag.Active {
		t.Fatalf("a far-left body click must not start a drag")
	}
	if m.rstate.Focus != router.FocusLeft {
		t.Errorf("far-left body click should focus the command pane, got %v", m.rstate.Focus)
	}
}

// TestDragFromPresetConvertsToAbsolute: starting a drag while a ratio preset is
// active switches to absolute-offset mode (ratioIdx=-1) and the divider follows
// smoothly from the preset position with no jump back to even.
func TestDragFromPresetConvertsToAbsolute(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 104, 40 // content 100, even left 50
	m.ratioIdx = 0              // 2:1 preset → left inner 66, divider at 67
	d := dividerCol(m)
	if d != 67 {
		t.Fatalf("2:1 preset divider = %d, want 67", d)
	}

	m.Update(mouse(tea.MouseActionPress, d, 10))
	if !m.drag.Active {
		t.Fatalf("press at the preset divider (col %d) should start a drag", d)
	}
	m.Update(mouse(tea.MouseActionMotion, d+10, 10))
	if m.ratioIdx != -1 {
		t.Errorf("drag should convert preset to absolute mode, ratioIdx=%d", m.ratioIdx)
	}
	if got, want := m.layout().LeftInnerWidth, (d+10)-1; got != want {
		t.Errorf("drag from preset to col %d: left inner = %d, want %d (jumped?)", d+10, got, want)
	}
}

// TestDragReflowsWindow drives a full press+motion drag with a real window
// present so the per-motion reflow() (which Resizes every PTY) runs on a live
// pane. It asserts the drag mutated the layout's left width and that nothing
// panicked. Skipped when a PTY cannot be allocated.
func TestDragReflowsWindow(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // creates window 0
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()

	d := dividerCol(m)
	before := m.layout().LeftInnerWidth
	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionMotion, d-12, 10)) // shrink the left pane

	if got, want := m.layout().LeftInnerWidth, (d-12)-1; got != want {
		t.Errorf("after drag with a live window: left inner = %d, want %d", got, want)
	}
	if m.layout().LeftInnerWidth >= before {
		t.Errorf("dragging the divider left should shrink the left pane (%d -> %d)", before, m.layout().LeftInnerWidth)
	}
}

// TestDividerClickNoMotionIsNoOp: a pure click on the divider (press + release
// with no motion) must leave the split untouched AND preserve an active ratio
// preset. The press only grabs the divider; the conversion to absolute-offset
// mode happens on the first motion, so a click that never moves is a strict
// no-op (it must not silently drop ratio-tracking or nudge the divider).
func TestDividerClickNoMotionIsNoOp(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 104, 40 // content 100, even left 50
	m.ratioIdx = 0              // 2:1 preset active
	d := dividerCol(m)
	beforeOffset := m.splitOffset

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionRelease, d, 10))

	if m.drag.Active {
		t.Errorf("release must end the drag")
	}
	if m.ratioIdx != 0 {
		t.Errorf("a click without motion must preserve the ratio preset, ratioIdx=%d", m.ratioIdx)
	}
	if m.splitOffset != beforeOffset {
		t.Errorf("a click without motion must not change splitOffset %d -> %d", beforeOffset, m.splitOffset)
	}
	if got := dividerCol(m); got != d {
		t.Errorf("a click without motion must not move the divider %d -> %d", d, got)
	}
}

// TestX10ReleaseEndsDrag (D1): an X10-encoded terminal reports a button release
// as Action=Release, Button=MouseButtonNone. The release must still end the drag
// — the button guard must not swallow it first — or the divider stays glued to
// the cursor forever, reflowing every PTY on each motion.
func TestX10ReleaseEndsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	if !m.drag.Active {
		t.Fatalf("press at the divider (col %d) should start a drag", d)
	}
	m.Update(mouse(tea.MouseActionMotion, d+10, 10))
	m.Update(mouseRelease(d+10, 10)) // X10-style release: Button=None
	if m.drag.Active {
		t.Fatalf("an X10 release (Button=None) must end the drag")
	}
	frozen := m.splitOffset
	m.Update(mouse(tea.MouseActionMotion, d+30, 10))
	if m.splitOffset != frozen {
		t.Errorf("post-release motion changed splitOffset %d -> %d", frozen, m.splitOffset)
	}
}

// TestResizeDuringDragClearsDrag (D2): a terminal resize mid-drag re-bases the
// split onto the new width, so it must end the drag; a later motion must not move
// the divider as if the drag were still live.
func TestResizeDuringDragClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	if !m.drag.Active {
		t.Fatalf("press at the divider should start a drag")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.drag.Active {
		t.Fatalf("a resize mid-drag must end the drag")
	}
	frozen := m.splitOffset
	m.Update(mouse(tea.MouseActionMotion, dividerCol(m)+20, 10))
	if m.splitOffset != frozen {
		t.Errorf("motion after a resize-ended drag changed splitOffset %d -> %d", frozen, m.splitOffset)
	}
}

// TestCloseWindowClearsDrag (D2): closing a window mid-drag ends the drag.
func TestCloseWindowClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.closeWindow() // window transition mid-drag
	if m.drag.Active {
		t.Fatalf("closeWindow mid-drag must end the drag")
	}
}

// TestCycleWindowClearsDrag (D2): cycling windows mid-drag ends the drag.
func TestCycleWindowClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.cycleWindow(1) // window transition mid-drag
	if m.drag.Active {
		t.Fatalf("cycleWindow mid-drag must end the drag")
	}
}

// TestSelectWindowClearsDrag (D2): selecting a window mid-drag ends the drag.
func TestSelectWindowClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.selectWindow(0) // window transition mid-drag
	if m.drag.Active {
		t.Fatalf("selectWindow mid-drag must end the drag")
	}
}

// TestNewWindowClearsDrag (D2): opening a window mid-drag ends the drag.
func TestNewWindowClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.newWindow() // window transition mid-drag (spawn may fail; endDrag runs first)
	if m.drag.Active {
		t.Fatalf("newWindow mid-drag must end the drag")
	}
}

// TestGameOverClearsDrag (D2): a game-over transition mid-drag ends the drag. The
// top rows are blocked (but left non-clearable so no line clear rescues the board)
// so the next gravity lock-and-spawn collides, driving the GameOver transition
// through the model (handleGravity), which is the path that must call endDrag.
func TestGameOverClearsDrag(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	// Block the top spawn area but leave column 0 empty so these rows never form a
	// full (clearable) line — otherwise a clear would empty the board and the
	// spawn would succeed instead of topping out.
	for y := 0; y < 4; y++ {
		for x := 1; x < tetris.Width; x++ {
			m.game.SetFilled(x, y, true, tetris.I)
		}
	}
	d := dividerCol(m)
	m.Update(mouse(tea.MouseActionPress, d, 10))
	if !m.drag.Active {
		t.Fatalf("press at the divider should start a drag")
	}
	// Step gravity through the model until the board tops out. Drive it via Update
	// so the model's game-over transition (which calls endDrag) is under test.
	for i := 0; i < tetris.Height+4 && m.game.State() == tetris.Playing; i++ {
		m.Update(gravityTickMsg{seq: m.gravitySeq})
	}
	if m.game.State() != tetris.GameOver {
		t.Fatalf("expected GameOver after gravity into a blocked board, got %v", m.game.State())
	}
	if m.drag.Active {
		t.Fatalf("a game-over transition mid-drag must end the drag")
	}
}

// TestStalePressSelfHeals (D2): if a release was lost and drag.Active is still
// true, a fresh press FAR from the divider on a body row must not act as a
// continued drag — the press self-heals the stale state and just focuses a pane.
func TestStalePressSelfHeals(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	m.drag.Active = true // simulate a lost release leaving a stale drag

	m.Update(mouse(tea.MouseActionPress, m.width-3, 5)) // far-right body press
	if m.drag.Active {
		t.Fatalf("a fresh body press far from the divider must self-heal a stale drag")
	}
	if m.rstate.Focus != router.FocusRight {
		t.Errorf("the self-healed press should still focus the clicked pane, got %v", m.rstate.Focus)
	}
}

// TestDragFarRightThenKeyboardStep (D3): after dragging the divider past the right
// edge (which pins the board at MinBoardWidth), a single keyboard shrink-left
// (C-b <) must move the divider exactly one splitStep left. Before the fix the
// stored splitOffset held the huge PRE-clamp value, so the first ~24 presses were
// dead travel and the divider did not move at all.
func TestDragFarRightThenKeyboardStep(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionMotion, m.width+50, 10)) // drag far past the right edge
	col0 := dividerCol(m)

	m.resizeSplit(-splitStep, false) // the C-b < path
	col1 := dividerCol(m)
	if col1 != col0-splitStep {
		t.Errorf("after a far-right drag, one C-b < should move the divider exactly %d left: %d -> %d (want %d)",
			splitStep, col0, col1, col0-splitStep)
	}
}

// TestShortTerminalNoDragStart (D7): on a terminal too short to draw the board
// (TooSmall), a divider press on the first body row (y==TabRows, just under the
// tab bar) must NOT start a drag — the board is invisible, so there is nothing to
// resize. Today such a press still arms a drag.
func TestShortTerminalNoDragStart(t *testing.T) {
	for _, h := range []int{1, 2, 3} {
		m := New(nil, 1)
		m.width, m.height = 100, h
		l := m.layout()
		if !l.TooSmall {
			t.Fatalf("h=%d expected TooSmall layout, got %+v", h, l)
		}
		d := dividerCol(m)
		m.Update(mouse(tea.MouseActionPress, d, layout.TabRows)) // first body row
		if m.drag.Active {
			t.Errorf("h=%d: a press on a too-short terminal must not start a drag", h)
		}
	}
}

// TestDragCoalescesReflow (D5): a fast motion sweep across many distinct columns
// must NOT issue one PTY Setsize per motion. The per-motion reflow is coalesced to
// a frame tick (whose tea.Cmd a test does not auto-run) and only the visible
// window is touched, so across N motions + the release flush the real ioctl count
// stays a small constant far below N. Skipped when a PTY cannot be allocated.
func TestDragCoalescesReflow(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // creates window 0
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()
	w := m.windows[0]

	d := dividerCol(m)
	m.Update(mouse(tea.MouseActionPress, d, 10))
	base := w.ResizeCalls()

	const n = 30
	for i := 0; i < n; i++ {
		m.Update(mouse(tea.MouseActionMotion, d-i-1, 10)) // distinct shrinking columns
	}
	m.Update(mouseRelease(d-n, 10)) // release flushes all windows once

	got := w.ResizeCalls() - base
	if got > 3 {
		t.Errorf("drag of %d motions issued %d pty Setsize calls, want a small constant (<=3)", n, got)
	}
	if got == 0 {
		t.Errorf("the drag should still apply the final size at least once on release")
	}
}

// TestResizeSkipsUnchanged (D6): Resize to the same dimensions twice must issue
// the pty.Setsize ioctl only once — an identical SIGWINCH is pure waste and at the
// clamp boundary many mouse columns map to the same width. Skipped without a PTY.
func TestResizeSkipsUnchanged(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()
	w := m.windows[0]

	base := w.ResizeCalls()
	w.Resize(40, 20)
	afterFirst := w.ResizeCalls()
	if afterFirst != base+1 {
		t.Fatalf("first Resize to a new size should Setsize once: %d -> %d", base, afterFirst)
	}
	w.Resize(40, 20) // identical dims
	if got := w.ResizeCalls(); got != afterFirst {
		t.Errorf("a Resize to the same dims must skip Setsize: %d -> %d", afterFirst, got)
	}
}

// TestBackgroundWindowReflowedAfterDragTick locks the endDrag flush fix: during a
// drag only the VISIBLE window is resized (reflowVisible on each frame tick), so a
// background window is never touched until the release flushes all windows. If a
// frame tick fires just before the release it clears dragDirty while resizing only
// the visible pane — so endDrag must reflow ALL windows on any wasActive end, NOT
// gate the flush on dragDirty, or the background pane keeps a stale (wrong) width.
// Skipped when PTYs cannot be allocated.
func TestBackgroundWindowReflowedAfterDragTick(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // creates window 0
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	m.newWindow() // window 1 becomes visible; window 0 is now a background window
	if len(m.windows) < 2 {
		t.Skip("could not open a second window")
	}
	defer m.Close()
	bg := m.windows[0] // background (not the active/visible) window
	d := dividerCol(m)

	m.Update(mouse(tea.MouseActionPress, d, 10))
	m.Update(mouse(tea.MouseActionMotion, d-15, 10)) // shrink; marks dragDirty, arms tick
	// Fire the frame tick: it resizes ONLY the visible window and clears dragDirty,
	// so right here the background window is still at its pre-drag width.
	m.Update(dragTickMsg{})
	beforeRelease := bg.ResizeCalls()

	// Release AFTER the tick (dragDirty already cleared): endDrag must still flush
	// the background window to the final width.
	m.Update(mouseRelease(d-15, 10))
	if got := bg.ResizeCalls(); got <= beforeRelease {
		t.Errorf("background window not reflowed on release after a frame tick: ResizeCalls %d -> %d (stale background width)", beforeRelease, got)
	}
}
