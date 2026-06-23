package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

func keyType(tp tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: tp} }

func countFilled(g *tetris.Game) int {
	n := 0
	for y := 0; y < tetris.Height; y++ {
		for x := 0; x < tetris.Width; x++ {
			if g.Filled[y][x] {
				n++
			}
		}
	}
	return n
}

func TestHandleGameKeyMovesPiece(t *testing.T) {
	m := New(nil, 1)
	x0 := m.game.Current.X
	m.handleGameKey(keyType(tea.KeyLeft))
	if m.game.Current.X != x0-1 {
		t.Errorf("left: X=%d want %d", m.game.Current.X, x0-1)
	}
	m.handleGameKey(keyType(tea.KeyRight))
	if m.game.Current.X != x0 {
		t.Errorf("right: X=%d want %d", m.game.Current.X, x0)
	}
}

func TestHandleGameKeyHardDropLocks(t *testing.T) {
	m := New(nil, 1)
	before := countFilled(m.game)
	m.handleGameKey(keyType(tea.KeySpace))
	if after := countFilled(m.game); after <= before {
		t.Errorf("hard drop should lock 4 cells: before=%d after=%d", before, after)
	}
}

func TestHandleGameKeyPauseToggles(t *testing.T) {
	m := New(nil, 1)
	m.handleGameKey(runeKey('p'))
	if m.game.State() != tetris.Paused {
		t.Errorf("p should pause, got %v", m.game.State())
	}
	m.handleGameKey(runeKey('p'))
	if m.game.State() != tetris.Playing {
		t.Errorf("second p should resume, got %v", m.game.State())
	}
}

func TestHandleGameKeySoftDropAndRotate(t *testing.T) {
	m := New(nil, 1)
	y0 := m.game.Current.Y
	m.handleGameKey(keyType(tea.KeyDown))
	if m.game.Current.Y != y0+1 {
		t.Errorf("soft drop: Y=%d want %d", m.game.Current.Y, y0+1)
	}
	// Rotations must keep the piece in a legal position (no panic, stays valid).
	m.handleGameKey(runeKey('x')) // CW
	m.handleGameKey(runeKey('z')) // CCW
	if !m.game.CanPlace(m.game.Current.X, m.game.Current.Y, m.game.Current.Rotation) {
		t.Errorf("piece left in an illegal position after rotate")
	}
}

func TestHandleGameKeyHoldKey(t *testing.T) {
	m := New(nil, 1)
	first := m.game.Current.Kind
	m.handleGameKey(runeKey('c'))
	if !m.game.HasHeld || m.game.Held != first {
		t.Errorf("'c' should hold the current piece: held=%v hasHeld=%v", m.game.Held, m.game.HasHeld)
	}
}

func TestHandleGameKeyRestartsAfterGameOver(t *testing.T) {
	m := New(nil, 1)
	for y := 0; y < 2; y++ {
		for x := 0; x < tetris.Width; x++ {
			m.game.SetFilled(x, y, true, tetris.I)
		}
	}
	m.game.ForceSpawn()
	if m.game.State() != tetris.GameOver {
		t.Fatalf("expected GameOver, got %v", m.game.State())
	}
	m.handleGameKey(runeKey('r'))
	if m.game.State() != tetris.Playing {
		t.Errorf("'r' should restart into a fresh game, state=%v", m.game.State())
	}
}

func TestHandleGravityAdvancesAndReschedules(t *testing.T) {
	m := New(nil, 1)
	y0 := m.game.Current.Y
	_, cmd := m.handleGravity(gravityTickMsg{seq: m.gravitySeq})
	if m.game.Current.Y != y0+1 {
		t.Errorf("live gravity tick should drop the piece: Y=%d want %d", m.game.Current.Y, y0+1)
	}
	if cmd == nil {
		t.Errorf("gravity should reschedule while still playing")
	}
}

func TestHandleGravityIgnoresStaleTick(t *testing.T) {
	m := New(nil, 1)
	m.gravitySeq = 5
	y0 := m.game.Current.Y
	_, cmd := m.handleGravity(gravityTickMsg{seq: 3})
	if cmd != nil {
		t.Errorf("stale gravity tick should not reschedule")
	}
	if m.game.Current.Y != y0 {
		t.Errorf("stale gravity tick must not advance the piece")
	}
}

func TestStartGravityNilWhenNotPlaying(t *testing.T) {
	m := New(nil, 1)
	m.game.SetState(tetris.Paused)
	if cmd := m.startGravity(); cmd != nil {
		t.Errorf("startGravity should return nil when not playing")
	}
}

// ctrlB and rune helpers keep the resize-key driving terse.
func ctrlB() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlB} }
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// drive sends a sequence of key messages through Update, mutating m in place.
func drive(m *Model, keys ...tea.KeyMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func TestSplitResizeKeys(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	base := m.layout().LeftInnerWidth

	// C-b > grows the left pane by one step.
	drive(m, ctrlB(), runeKey('>'))
	if m.splitOffset != splitStep {
		t.Fatalf("after C-b >: splitOffset=%d want %d", m.splitOffset, splitStep)
	}
	if got := m.layout().LeftInnerWidth; got != base+splitStep {
		t.Errorf("left inner=%d want %d", got, base+splitStep)
	}

	// C-b < shrinks it back, then below the midpoint.
	drive(m, ctrlB(), runeKey('<'), ctrlB(), runeKey('<'))
	if m.splitOffset != -splitStep {
		t.Errorf("after two C-b <: splitOffset=%d want %d", m.splitOffset, -splitStep)
	}

	// C-b = resets to the even split.
	drive(m, ctrlB(), runeKey('='))
	if m.splitOffset != 0 {
		t.Errorf("after C-b =: splitOffset=%d want 0", m.splitOffset)
	}
	if got := m.layout().LeftInnerWidth; got != base {
		t.Errorf("reset left inner=%d want %d", got, base)
	}
}

// Tab toggles focus between the panes, and once the board is focused the game
// keys work directly with no prefix.
func TestTabFocusAndDirectGameKeys(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40

	// Default focus is the left pane; Tab jumps to the game.
	tab := tea.KeyMsg{Type: tea.KeyTab}
	drive(m, tab)
	if m.rstate.Focus != router.FocusRight {
		t.Fatalf("Tab should focus the game, got %v", m.rstate.Focus)
	}
	// 'p' now stops the game directly.
	drive(m, runeKey('p'))
	if m.game.State() != tetris.Paused {
		t.Errorf("p (board focused) should stop the game, state=%v", m.game.State())
	}
	drive(m, runeKey('p'))
	if m.game.State() != tetris.Playing {
		t.Errorf("p again should start the game, state=%v", m.game.State())
	}
	// Tab returns to the command pane.
	drive(m, tab)
	if m.rstate.Focus != router.FocusLeft {
		t.Errorf("Tab should return focus to the command pane, got %v", m.rstate.Focus)
	}
}

// Esc pauses/resumes the game when the board is focused (and the pane then shows
// the pause overlay via renderRight).
func TestEscPausesWhenGameFocused(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	m.rstate.Focus = router.FocusRight

	drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.game.State() != tetris.Paused {
		t.Errorf("Esc (game focused) should pause, state=%v", m.game.State())
	}
	drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.game.State() != tetris.Playing {
		t.Errorf("Esc again should resume, state=%v", m.game.State())
	}
}

// 'r' restarts the game at any time (not only after game over), clearing the
// board into a fresh playing game.
func TestRestartAnytime(t *testing.T) {
	m := New(nil, 1)
	m.rstate.Focus = router.FocusRight
	m.game.SetFilled(0, tetris.Height-1, true, tetris.I)
	drive(m, runeKey('r'))
	if m.game.State() != tetris.Playing {
		t.Errorf("r should yield a playing game, state=%v", m.game.State())
	}
	if m.game.Filled[tetris.Height-1][0] {
		t.Errorf("r should clear the board")
	}
}

// While paused, ↑/↓ move the menu highlight and Enter activates it: the top
// item (계속하기) resumes, the second (재시작) starts a fresh game.
func TestPauseMenuEnterResumesAndRestarts(t *testing.T) {
	m := New(nil, 1)
	m.rstate.Focus = router.FocusRight

	// Pause, then Enter on the default item resumes.
	drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.game.State() != tetris.Paused || m.pauseSel != pauseResume {
		t.Fatalf("Esc should pause on the resume item, state=%v sel=%d", m.game.State(), m.pauseSel)
	}
	drive(m, keyType(tea.KeyEnter))
	if m.game.State() != tetris.Playing {
		t.Errorf("Enter on 계속하기 should resume, state=%v", m.game.State())
	}

	// Pause again, move to 재시작, dirty the board, then Enter restarts it clean.
	m.game.SetFilled(0, tetris.Height-1, true, tetris.I)
	drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	drive(m, keyType(tea.KeyDown))
	if m.pauseSel != pauseRestart {
		t.Fatalf("Down should highlight 재시작, sel=%d", m.pauseSel)
	}
	drive(m, keyType(tea.KeyEnter))
	if m.game.State() != tetris.Playing {
		t.Errorf("Enter on 재시작 should start a playing game, state=%v", m.game.State())
	}
	if m.game.Filled[tetris.Height-1][0] {
		t.Errorf("restart from the pause menu should clear the board")
	}
}

// Pause-menu navigation wraps around the two items in both directions.
func TestPauseMenuNavigationWraps(t *testing.T) {
	m := New(nil, 1)
	m.rstate.Focus = router.FocusRight
	drive(m, tea.KeyMsg{Type: tea.KeyEsc}) // sel = pauseResume (0)

	drive(m, keyType(tea.KeyUp)) // wrap up from 0 to the last item
	if m.pauseSel != pauseRestart {
		t.Errorf("Up from the top item should wrap to 재시작, sel=%d", m.pauseSel)
	}
	drive(m, keyType(tea.KeyDown)) // wrap back down to 0
	if m.pauseSel != pauseResume {
		t.Errorf("Down from the last item should wrap to 계속하기, sel=%d", m.pauseSel)
	}
}

// After a loss, Enter restarts the game from the game-over overlay (in addition
// to the existing 'r' shortcut), clearing the board into a fresh playing game.
func TestGameOverEnterRestarts(t *testing.T) {
	m := New(nil, 1)
	m.rstate.Focus = router.FocusRight
	// Force a game over: fill the top rows so the next spawn collides.
	for y := 0; y < 2; y++ {
		for x := 0; x < tetris.Width; x++ {
			m.game.SetFilled(x, y, true, tetris.I)
		}
	}
	m.game.ForceSpawn()
	if m.game.State() != tetris.GameOver {
		t.Fatalf("expected GameOver, got %v", m.game.State())
	}

	cmd := m.handleGameKey(keyType(tea.KeyEnter))
	if m.game.State() != tetris.Playing {
		t.Errorf("Enter on the game-over overlay should restart, state=%v", m.game.State())
	}
	if cmd == nil {
		t.Errorf("restart should resume the gravity loop")
	}
	if countFilled(m.game) > 4 {
		t.Errorf("restart should clear the board, filled=%d", countFilled(m.game))
	}
}

// Ctrl-C quits tetmux when the game pane is focused (returns a tea.Quit cmd).
func TestCtrlCQuitsWhenGameFocused(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	m.rstate.Focus = router.FocusRight

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("Ctrl-C (game focused) should return a quit command")
	}
	if msg := cmd(); msg == nil {
		t.Errorf("expected the quit command to produce a tea.QuitMsg, got nil")
	}
}

// Ctrl-C with the command pane focused is forwarded to the child (does NOT
// quit tetmux), so a running command can still be interrupted.
func TestCtrlCForwardedWhenCommandFocused(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // creates window 0
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()

	if m.rstate.Focus != router.FocusLeft {
		t.Fatalf("default focus should be the command pane, got %v", m.rstate.Focus)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Errorf("Ctrl-C (command focused) should not quit tetmux")
	}
}

// cmux-style windows: C-b c creates, C-b 1-9 / n / p switch, C-b x closes, and
// the last window can't be closed.
func TestMultiWindowLifecycle(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // creates window 0
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()

	if m.active != 0 {
		t.Fatalf("first window should be active, active=%d", m.active)
	}
	// C-b c → new window becomes active.
	drive(m, ctrlB(), runeKey('c'))
	if len(m.windows) != 2 || m.active != 1 {
		t.Fatalf("C-b c: windows=%d active=%d want 2/1", len(m.windows), m.active)
	}
	// C-b 1 → jump to window 1 (0-based 0).
	drive(m, ctrlB(), runeKey('1'))
	if m.active != 0 {
		t.Errorf("C-b 1: active=%d want 0", m.active)
	}
	// C-b n / C-b p cycle with wraparound.
	drive(m, ctrlB(), runeKey('n'))
	if m.active != 1 {
		t.Errorf("C-b n: active=%d want 1", m.active)
	}
	drive(m, ctrlB(), runeKey('n')) // wrap to 0
	if m.active != 0 {
		t.Errorf("C-b n wrap: active=%d want 0", m.active)
	}
	// C-b x closes the current window; the last one cannot be closed.
	drive(m, ctrlB(), runeKey('x'))
	if len(m.windows) != 1 {
		t.Errorf("C-b x: windows=%d want 1", len(m.windows))
	}
	drive(m, ctrlB(), runeKey('x'))
	if len(m.windows) != 1 {
		t.Errorf("closing the last window must be a no-op, windows=%d", len(m.windows))
	}
}

func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}
}

// Clicking the top tab bar selects windows; clicking [+] creates one.
func TestMouseTabBarSelectsAndCreates(t *testing.T) {
	m := New([]string{"sleep", "60"}, 1)
	m.width, m.height = 100, 40
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if len(m.windows) == 0 {
		t.Skip("pty unavailable")
	}
	defer m.Close()

	// Click [+] to create and activate a second window.
	_, hits := m.tabBar()
	var plusX int
	for _, h := range hits {
		if h.isNew {
			plusX = h.start
		}
	}
	m.Update(leftClick(plusX, 0))
	if len(m.windows) != 2 || m.active != 1 {
		t.Fatalf("clicking + should create+activate a window, windows=%d active=%d", len(m.windows), m.active)
	}
	// Click the first tab to select window 0.
	_, hits = m.tabBar()
	m.Update(leftClick(hits[0].start, 0))
	if m.active != 0 {
		t.Errorf("clicking tab 1 should select window 0, active=%d", m.active)
	}
}

// Clicking inside a pane focuses it.
func TestMousePaneClickFocuses(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	m.Update(leftClick(90, 5)) // far right → game pane
	if m.rstate.Focus != router.FocusRight {
		t.Errorf("click on right pane should focus it, got %v", m.rstate.Focus)
	}
	m.Update(leftClick(2, 5)) // far left → command pane
	if m.rstate.Focus != router.FocusLeft {
		t.Errorf("click on left pane should focus it, got %v", m.rstate.Focus)
	}
}

// A plain '>' without the prefix must not resize — it is an ordinary key.
func TestUnprefixedRuneDoesNotResize(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	drive(m, runeKey('>'))
	if m.splitOffset != 0 {
		t.Errorf("unprefixed '>' changed splitOffset to %d", m.splitOffset)
	}
}

// PrimaryExitCode reports nothing until the first window exists and has exited,
// then returns its real exit status (so main can propagate `tetmux -- cmd`'s code).
func TestPrimaryExitCode(t *testing.T) {
	m := New(nil, 1)
	if _, ok := m.PrimaryExitCode(); ok {
		t.Error("no windows => PrimaryExitCode should report ok=false")
	}

	lp, err := newLeftPane([]string{"sh", "-c", "exit 7"}, 10, 4, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	m.windows = append(m.windows, lp)
	defer m.Close()

	if !waitExited(lp, 3*time.Second) {
		t.Fatal("child did not exit in time")
	}
	code, ok := m.PrimaryExitCode()
	if !ok || code != 7 {
		t.Errorf("PrimaryExitCode = %d, %v; want 7, true", code, ok)
	}
}

// cycleRatio walks the preset ratios and the resulting left inner width tracks
// each ratio; the chosen ratio also survives a terminal resize (recomputed for
// the new width), and a nudge drops back to absolute-offset mode.
func TestCycleRatioAndResizeStability(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 104, 40 // content = 100, even left = 50

	if got := m.layout().LeftInnerWidth; got != 50 {
		t.Fatalf("baseline even left = %d, want 50", got)
	}

	// 2:1 -> 1:1 -> 1:2 -> wrap to 2:1.
	wants := []int{66, 50, 33, 66}
	for i, want := range wants {
		m.cycleRatio()
		if got := m.layout().LeftInnerWidth; got != want {
			t.Errorf("cycle %d: left = %d, want %d (ratioIdx=%d)", i, got, want, m.ratioIdx)
		}
	}

	// Now at 2:1 (ratioIdx 0). Widen the terminal: the ratio must hold, so the
	// left width is recomputed (content 200 * 2/3 = 133), not frozen at 66.
	m.width = 204
	if got := m.layout().LeftInnerWidth; got != 133 {
		t.Errorf("after resize, 2:1 left = %d, want 133 (ratio not resize-stable)", got)
	}

	// A nudge leaves ratio mode and continues from the current divider position.
	before := m.layout().LeftInnerWidth
	m.resizeSplit(splitStep, false)
	if m.ratioIdx != -1 {
		t.Errorf("nudge should clear ratio mode, ratioIdx=%d", m.ratioIdx)
	}
	if got := m.layout().LeftInnerWidth; got != before+splitStep {
		t.Errorf("nudge: left = %d, want %d", got, before+splitStep)
	}

	// Reset returns to the even split and stays out of ratio mode.
	m.resizeSplit(0, true)
	if m.ratioIdx != -1 || m.splitOffset != 0 {
		t.Errorf("reset: ratioIdx=%d splitOffset=%d, want -1, 0", m.ratioIdx, m.splitOffset)
	}
	if got, want := m.layout().LeftInnerWidth, (204-4+1)/2; got != want {
		t.Errorf("reset even left = %d, want %d", got, want)
	}
}

// Shrinking the terminal below the board size auto-pauses the game so gravity
// can't drop pieces onto an invisible board (you'd resize back to a lost game).
func TestTooSmallAutoPauses(t *testing.T) {
	m := New(nil, 1)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40}) // comfortably fits
	if m.game.State() != tetris.Playing {
		t.Fatalf("should start playing, state=%v", m.game.State())
	}
	// Shrink below the board: layout reports TooSmall.
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	if !m.layout().TooSmall {
		t.Fatalf("20x10 should be too small: %+v", m.layout())
	}
	if m.game.State() != tetris.Paused {
		t.Errorf("too-small terminal should auto-pause, state=%v", m.game.State())
	}
}
