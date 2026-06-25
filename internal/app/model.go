// Package app is the thin TUI glue that wires bubbletea + PTY + vt10x over the
// tested pure packages (tetris, router, layout, vtrender). It intentionally
// keeps no game/routing/layout arithmetic of its own — that all lives in the
// unit-tested packages — but the key-byte mapping, lifecycle, and scheduling
// decisions here are exercised by internal/app's own tests.
package app

import (
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/layout"
	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

// splitStep is how many content columns one resize keystroke moves the divider.
const splitStep = 4

// ctrlCQuitWindow is how long the "press Ctrl-C again to quit" prompt stays live
// after a first Ctrl-C on the command pane.
const ctrlCQuitWindow = 600 * time.Millisecond

// maxPendingInput caps the pre-PTY keystroke buffer so a never-spawning child
// can't grow it without bound.
const maxPendingInput = 4096

// gravityTickMsg fires a gravity step. seq guards against stale ticks after the
// game stops so we never schedule more than one live gravity loop.
type gravityTickMsg struct{ seq int }

// ptyMsg is pushed by a command window's reader goroutine whenever new output
// has arrived (coalesced via the pane's dirty bit). Receiving it triggers
// exactly one redraw; when every child is idle no ptyMsg is sent, so idle CPU
// is ~0.
type ptyMsg struct{}

// maxWindows caps how many command windows can exist at once.
const maxWindows = 9

// Model is the bubbletea model holding the command windows, the game, focus
// state, and terminal dimensions. The left side is a cmux-style stack of
// command windows (each its own PTY, all running in parallel); exactly one is
// shown at a time and the rest keep running in the background.
type Model struct {
	windows  []*leftPane // command windows; all run in parallel
	active   int         // index of the visible window
	game     *tetris.Game
	rstate   router.State
	width    int
	height   int
	renderer *lipgloss.Renderer

	// splitOffset shifts the pane divider away from the even midpoint, in
	// content columns: positive grows the left (command) pane, negative grows
	// the right (Tetris) pane. Adjusted live with C-b >/< and reset with C-b =.
	splitOffset int

	// ratioIdx selects a preset left:right divider ratio (see ratioPresets),
	// cycled with C-b z. -1 means "no preset" — the absolute splitOffset is used
	// instead. A preset is recomputed for the current width on every layout, so
	// the ratio holds across terminal resizes; nudging or resetting clears it.
	ratioIdx int

	// drag tracks an in-progress mouse divider drag: a left-button press near the
	// divider arms it, each motion event reflows the panes to follow the cursor,
	// and the button release ends it. The pure X→offset math and the divider
	// hit-test live in internal/layout so this stays thin glue.
	drag layout.DragState

	// dragDirty/dragTick coalesce the PTY reflow during a drag: each motion only
	// updates splitOffset (cheap) and marks dragDirty, while the costly pty.Setsize
	// runs at most once per frame (dragTickMsg) on the visible window; background
	// windows are batch-reflowed once on release/endDrag. Without this, every motion
	// Resized up to maxWindows PTYs — a SIGWINCH + full-redraw + realloc storm. See
	// scheduleDragReflow/handleDragTick/endDrag in view.go.
	dragDirty bool
	dragTick  bool // a frame-gated drag reflow is already scheduled

	best     int       // best score seen, persisted across runs (best-effort)
	bestOnce sync.Once // guards the single persist on shutdown

	// pauseSel is the highlighted item in the pause overlay's selectable menu
	// (see pauseMenu* consts). It is reset to the top item each time the game
	// is paused so the menu always opens on "계속하기".
	pauseSel int

	// autoPaused records that the CURRENT Paused state was forced by the glue
	// layer (focus left the game while it was Playing, or the terminal shrank
	// below the board) rather than by a user Esc/p. Only an auto-pause is
	// auto-resumed: when focus returns to the game (and the board fits) an
	// auto-paused game resumes and restarts gravity, while a user pause stays
	// paused until the user resumes it. It is the single flag that keeps the
	// input-focus state machine and the game lifecycle from drifting into
	// contradictory states. See reconcileFocus.
	autoPaused bool

	// ctrlCArmed/ctrlCAt implement "press Ctrl-C twice to quit" from the command
	// pane: a single Ctrl-C is forwarded to the child (so you can cancel e.g. a
	// claude turn), and a second one within ctrlCQuitWindow quits tetmux. Any
	// other key disarms it. See handleCommandCtrlC.
	ctrlCArmed bool
	ctrlCAt    time.Time

	// pendingInput buffers keystrokes routed to the left pane before the PTY
	// exists (between startup and the first WindowSizeMsg); flushed on create.
	pendingInput []byte

	gravitySeq int // increments each time we (re)start gravity

	argv     []string
	seed     int64
	send     func(tea.Msg) // injected from main after the program is built
	spawnErr error         // non-nil if the left command failed to start

	// hw is the shared hardware-cursor target. View() points it at the focused
	// command pane's cursor cell so an IME/CJK preedit appears inline; the
	// output writer (see cursor.go / WrapOutput) re-homes the real cursor there
	// after every frame.
	hw *hwCursor
	// imeCursor enables the hardware-cursor re-homing above. It is on by default
	// and disabled by TETMUX_NO_IME_CURSOR, an escape hatch for terminals/TUIs
	// where following the child's reported cursor misplaces the OS IME preedit.
	imeCursor bool
}

// New constructs a Model. The left pane is created lazily on the first
// WindowSizeMsg so the PTY is sized correctly.
func New(argv []string, seed int64) *Model {
	return &Model{
		game:      tetris.NewGame(seed),
		rstate:    router.State{Focus: router.FocusLeft},
		renderer:  lipgloss.NewRenderer(os.Stdout),
		argv:      argv,
		seed:      seed,
		best:      loadBestScore(),
		hw:        &hwCursor{},
		ratioIdx:  -1, // start in absolute-offset (even) mode, no preset selected
		imeCursor: os.Getenv("TETMUX_NO_IME_CURSOR") == "",
	}
}

// ratioPresets are the left:right divider ratios cycled by C-b z. 2:1 gives the
// command pane twice the Tetris pane's width; 1:1 is even (matching C-b =); 1:2
// favors the game. Recomputed per width so a chosen ratio survives resizes.
var ratioPresets = [][2]int{{2, 1}, {1, 1}, {1, 2}}

// trackBest bumps the in-memory best score to the current game score. The value
// is persisted on game-over (persistBest) and once on shutdown (Close); see
// score.go.
func (m *Model) trackBest() {
	if m.game.Score > m.best {
		m.best = m.game.Score
	}
}

// persistBest eagerly writes the best score. It is idempotent — saveBestScore
// no-ops unless the value beats what's stored — so calling it at game-over means
// a high score survives even if the process is killed before the final Close().
func (m *Model) persistBest() { saveBestScore(m.best) }

// cur returns the currently visible command window, or nil if none exists yet.
func (m *Model) cur() *leftPane {
	if m.active >= 0 && m.active < len(m.windows) {
		return m.windows[m.active]
	}
	return nil
}

// SetSend injects the program's thread-safe Send func so the left pane's reader
// goroutine can push a redraw when PTY output arrives. Must be called before
// Run() (i.e. before the reader goroutine could exist), so there is no race.
func (m *Model) SetSend(fn func(tea.Msg)) { m.send = fn }

// SpawnErr returns the error from a failed left-command spawn, if any, so the
// caller can exit non-zero after the program ends.
func (m *Model) SpawnErr() error { return m.spawnErr }

// PrimaryExitCode reports the exit status of the originally-launched command
// (the first window) once it has exited, so the process can propagate it as its
// own exit status. ok is false when that window never existed or is still
// running. Read it before Close() so a natural exit isn't masked by teardown.
func (m *Model) PrimaryExitCode() (code int, ok bool) {
	if len(m.windows) == 0 || m.windows[0] == nil {
		return 0, false
	}
	w := m.windows[0]
	if !w.Exited() {
		return 0, false
	}
	return w.ExitCode(), true
}

// Close terminates every command window's child and PTY. Safe to call more than
// once and when no window was ever created.
func (m *Model) Close() {
	// Persist the best score exactly once, even though Close may be called from
	// both the signal handler and the main goroutine.
	m.bestOnce.Do(func() { saveBestScore(m.best) })
	for _, w := range m.windows {
		if w != nil {
			w.Close()
		}
	}
}

// Init starts the program. Gravity ticks while the game is playing; the left
// pane pushes its own redraws, so no polling timer is needed.
func (m *Model) Init() tea.Cmd {
	return m.startGravity()
}

// startGravity schedules a gravity tick only when the game is actively
// playing. When stopped/idle it returns nil so no tick runs.
func (m *Model) startGravity() tea.Cmd {
	if m.game.State() != tetris.Playing {
		return nil
	}
	m.gravitySeq++
	seq := m.gravitySeq
	return tea.Tick(tetris.GravityInterval(m.game.Level()), func(time.Time) tea.Msg {
		return gravityTickMsg{seq: seq}
	})
}

// reconcileFocus makes the game lifecycle a pure function of the current input
// focus, recomputed lazily wherever focus or the game is observed (the Update
// key path, gravity ticks, mouse focus changes). It is the single point that
// keeps the two state machines coherent:
//
//   - Focus left the game while it was Playing -> auto-pause it and flag the
//     pause as automatic. The live gravity loop then stops on its next tick
//     (handleGravity no-ops once the game is not Playing), so a backgrounded
//     board never tops out silently.
//   - Focus returned to the game while it is AUTO-paused and the board fits ->
//     auto-resume (Playing) and clear the flag; the returned Cmd restarts the
//     gravity loop. A USER pause (autoPaused==false) is left untouched, so it is
//     never auto-resumed by a focus change.
//
// It returns the gravity Cmd to schedule (non-nil only on an auto-resume) so the
// caller in the Update path can keep the gravity loop alive; callers that cannot
// thread a Cmd (gravity/mouse) rely on the next observation to restart it.
func (m *Model) reconcileFocus() tea.Cmd {
	if m.rstate.Focus == router.FocusRight {
		// Returned to the game: resume an auto-pause once the board fits again.
		if m.autoPaused && m.game.State() == tetris.Paused && !m.layout().TooSmall {
			m.game.SetState(tetris.Playing)
			m.autoPaused = false
			return m.startGravity()
		}
		return nil
	}
	// Left the game: auto-pause a Playing board so gravity stops.
	if m.game.State() == tetris.Playing {
		m.game.SetState(tetris.Paused)
		m.pauseSel = pauseResume
		m.autoPaused = true
	}
	return nil
}

// notify builds the callback the left pane's reader goroutine invokes when new
// output arrives. It is a no-op when no Send func was injected (e.g. in tests).
func (m *Model) notify() func() {
	send := m.send
	if send == nil {
		return func() {}
	}
	return func() { send(ptyMsg{}) }
}

// Update is the central event router.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	case tea.KeyMsg:
		model, cmd := m.handleKey(msg)
		// A focus change (Tab / C-b h|l|arrows) drives the game lifecycle: leaving
		// the game auto-pauses it, returning auto-resumes an auto-pause. Reconcile
		// after the key is routed so the focus the user just set is the source of
		// truth, and prefer the auto-resume gravity Cmd so the loop restarts on the
		// same keystroke that re-focuses the board.
		if rc := m.reconcileFocus(); rc != nil {
			cmd = rc
		}
		return model, cmd
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case gravityTickMsg:
		return m.handleGravity(msg)
	case dragTickMsg:
		return m.handleDragTick()
	case ptyMsg:
		// New PTY output arrived. Clear the visible window's dirty bit so its
		// next output re-notifies, then fall through to a redraw. Background
		// windows keep their dirty bit set (they notified once) and are
		// re-consumed when switched to, so they don't drive redundant redraws.
		if w := m.cur(); w != nil {
			w.ConsumeDirty()
		}
		return m, nil
	}
	return m, nil
}

// layout returns the current geometry, honoring the effective divider offset.
func (m *Model) layout() layout.Layout {
	return layout.ComputeSplit(m.width, m.height, layout.DefaultBorders(), m.effectiveOffset())
}

// effectiveOffset resolves the divider offset: a selected ratio preset wins and
// is recomputed for the current width (so the ratio survives terminal resizes);
// otherwise the absolute splitOffset from C-b >/< nudging is used.
func (m *Model) effectiveOffset() int {
	if m.ratioIdx >= 0 && m.ratioIdx < len(ratioPresets) {
		p := ratioPresets[m.ratioIdx]
		return layout.OffsetForRatio(m.width, layout.DefaultBorders(), p[0], p[1])
	}
	return m.splitOffset
}

// reflow resizes every command window's PTY + grid to the current layout's left
// inner dimensions. Called after any change to the divider (nudge, reset, ratio).
func (m *Model) reflow() {
	l := m.layout()
	cols, rows := maxInt(l.LeftInnerWidth, 1), maxInt(l.InnerHeight, 1)
	for _, w := range m.windows {
		w.Resize(cols, rows)
	}
}

// cycleRatio advances to the next preset divider ratio (entering ratio mode from
// nudge mode if needed) and reflows the panes to the new width.
func (m *Model) cycleRatio() {
	m.ratioIdx = (m.ratioIdx + 1) % len(ratioPresets)
	m.reflow()
}

func (m *Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	// A terminal resize re-bases the split geometry onto the new width, so any
	// in-progress divider drag must end here rather than silently re-anchor mid
	// drag (and a lost release would otherwise strand it Active forever).
	m.endDrag()
	m.width = msg.Width
	m.height = msg.Height
	l := m.layout()

	// Auto-pause when the board no longer fits: otherwise gravity keeps dropping
	// pieces onto an invisible board and you could resize back to find the game
	// already lost. Resuming is one keypress once the board fits again.
	if l.TooSmall && m.game.State() == tetris.Playing {
		m.game.SetState(tetris.Paused)
		m.pauseSel = pauseResume
		m.autoPaused = true // shrinking too small is an auto-pause; a regrow + focus resumes it
	}

	cols := maxInt(l.LeftInnerWidth, 1)
	rows := maxInt(l.InnerHeight, 1)

	if len(m.windows) == 0 {
		// Lazily create the first window sized to its inner dimensions. Capture
		// any spawn error so the View can show it instead of hanging.
		lp, err := newLeftPane(m.argv, cols, rows, m.notify())
		if err != nil {
			m.spawnErr = err
		} else {
			m.windows = append(m.windows, lp)
			m.active = 0
			// Flush any keystrokes typed before the PTY existed (the window
			// between program start and the first WindowSizeMsg).
			if len(m.pendingInput) > 0 {
				lp.Write(m.pendingInput)
				m.pendingInput = nil
			}
		}
		return m, nil
	}

	// Resize every window so background sessions stay correctly sized.
	for _, w := range m.windows {
		w.Resize(cols, rows)
	}
	return m, nil
}

// resizeSplit moves the divider by delta content columns (or resets to even
// when reset is true) and reflows every window's PTY to the new width. Either
// action leaves ratio-preset mode: a nudge continues from wherever the current
// (possibly preset-derived) divider sits, and reset returns to the even split.
func (m *Model) resizeSplit(delta int, reset bool) {
	if reset {
		m.splitOffset = 0
	} else {
		m.splitOffset = m.effectiveOffset() + delta
	}
	m.ratioIdx = -1
	m.reflow()
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := toRouterKey(msg)
	act := router.Route(m.rstate, key)
	m.rstate = act.NewState

	// Ctrl-C on the command pane: "press twice to quit" (the router routes it
	// left so a running child still gets the first interrupt). Game-pane Ctrl-C
	// is RouteNone+CmdQuit and falls through to the quit case below.
	if msg.Type == tea.KeyCtrlC && act.RouteTo == router.RouteLeft {
		return m.handleCommandCtrlC(act)
	}
	// Any other key cancels a pending "press again to quit".
	m.ctrlCArmed = false

	switch act.Command {
	case router.CmdQuit:
		m.Close()
		return m, tea.Quit
	case router.CmdSplitGrowLeft:
		m.resizeSplit(splitStep, false)
		return m, nil
	case router.CmdSplitShrinkLeft:
		m.resizeSplit(-splitStep, false)
		return m, nil
	case router.CmdSplitReset:
		m.resizeSplit(0, true)
		return m, nil
	case router.CmdSplitRatioCycle:
		m.cycleRatio()
		return m, nil
	case router.CmdNewWindow:
		m.newWindow()
		return m, nil
	case router.CmdNextWindow:
		m.cycleWindow(1)
		return m, nil
	case router.CmdPrevWindow:
		m.cycleWindow(-1)
		return m, nil
	case router.CmdCloseWindow:
		m.closeWindow()
		return m, nil
	case router.CmdSelectWindow:
		m.selectWindow(act.Arg - 1) // Arg is 1-based from the keypress
		return m, nil
	}

	switch act.RouteTo {
	case router.RouteLeft:
		if len(act.Bytes) > 0 {
			if w := m.cur(); w != nil {
				out := act.Bytes
				// A bubbletea paste (dragged-in image path, multi-line block)
				// arrives as one KeyRunes msg with Paste set; re-frame it in the
				// bracketed-paste markers so a child that enabled mode 2004 (e.g.
				// claude) recognizes it as a paste instead of typed text — that is
				// what turns a pasted image path into an attachment rather than
				// literal characters.
				if msg.Paste && w.BracketedPaste() {
					out = wrapBracketedPaste(out)
				}
				w.Write(out)
			} else if len(m.pendingInput) < maxPendingInput {
				// PTY not created yet (pre first WindowSizeMsg): buffer so the
				// keystrokes aren't lost, capped so a stuck spawn can't grow it
				// unbounded.
				m.pendingInput = append(m.pendingInput, act.Bytes...)
			}
		}
	case router.RouteRight:
		return m, m.handleGameKey(msg)
	}

	return m, nil
}

// handleCommandCtrlC implements the command pane's "press Ctrl-C twice to quit"
// rule. A single Ctrl-C is forwarded to the child so a running command (e.g. a
// claude turn) can be interrupted as usual; a second Ctrl-C within
// ctrlCQuitWindow quits tetmux. If the child has already exited there is nothing
// to interrupt, so a single Ctrl-C quits immediately.
func (m *Model) handleCommandCtrlC(act router.Action) (tea.Model, tea.Cmd) {
	w := m.cur()
	if w == nil || w.Exited() {
		m.Close()
		return m, tea.Quit
	}
	if m.ctrlCArmed && time.Since(m.ctrlCAt) < ctrlCQuitWindow {
		m.Close()
		return m, tea.Quit
	}
	w.Write(act.Bytes) // forward the first interrupt to the child
	m.ctrlCArmed = true
	m.ctrlCAt = time.Now()
	return m, nil
}

// pauseMenu items: the selectable choices shown in the pause overlay. They are
// indexed by pauseSel; pauseMenuItems is the count for wrap-around navigation.
const (
	pauseResume = iota
	pauseRestart
	pauseMenuItems
)

// handleGameKey applies a key to the Tetris game when the right pane is
// focused, restarting gravity if a previously-stopped game becomes active.
func (m *Model) handleGameKey(msg tea.KeyMsg) tea.Cmd {
	// While paused, the right pane shows a selectable menu (계속하기 / 재시작)
	// instead of accepting gameplay keys; route the keys there.
	if m.game.State() == tetris.Paused {
		return m.handlePauseMenuKey(msg)
	}
	// After a loss the pane shows the game-over overlay; only restart keys act.
	if m.game.State() == tetris.GameOver {
		return m.handleGameOverKey(msg)
	}

	wasPlaying := m.game.State() == tetris.Playing
	// Movement keys go through the shared tetris mapper; only the lifecycle keys
	// (pause/restart) are handled here.
	if !m.game.ApplyKey(msg.String()) {
		switch msg.String() {
		case "p", "esc":
			// Esc and p pause the game; the pause overlay (renderRight) then shows
			// the selectable Resume/Restart menu, opening on "계속하기". This is a
			// USER pause, so clear the auto-pause flag: a later focus change must
			// NOT auto-resume a pause the user asked for.
			m.game.SetState(tetris.Paused)
			m.pauseSel = pauseResume
			m.autoPaused = false
		case "r":
			m.restart()
		}
	}
	m.trackBest()
	if wasPlaying && m.game.State() == tetris.GameOver {
		m.persistBest() // lock in the high score the moment the board dies
		m.endDrag()     // the game-over transition ends any in-progress divider drag
	}
	// If the game just became (or resumed) playing and no gravity loop is
	// running, start one.
	if !wasPlaying && m.game.State() == tetris.Playing {
		return m.startGravity()
	}
	return nil
}

// handlePauseMenuKey drives the selectable pause overlay: ↑/↓ (or h/j/k/l) move
// the highlight, Enter activates the highlighted item, and Esc/p/r keep their
// shortcut meaning (resume, resume, restart) so muscle memory still works.
func (m *Model) handlePauseMenuKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k", "left", "h":
		m.pauseSel = (m.pauseSel - 1 + pauseMenuItems) % pauseMenuItems
		return nil
	case "down", "j", "right", "l":
		m.pauseSel = (m.pauseSel + 1) % pauseMenuItems
		return nil
	case "esc", "p":
		// Quick resume shortcut, independent of the highlighted item.
		m.game.SetState(tetris.Playing)
		return m.startGravity()
	case "r":
		m.restart()
		return m.startGravity()
	case "enter":
		if m.pauseSel == pauseRestart {
			m.restart()
		} else {
			m.game.SetState(tetris.Playing)
		}
		return m.startGravity()
	}
	return nil
}

// handleGameOverKey drives the game-over overlay: Enter or r starts a fresh
// game (the only meaningful action once the board is dead). Other keys are
// swallowed so a dead board can't be "played".
func (m *Model) handleGameOverKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter", "r":
		m.restart()
		return m.startGravity()
	}
	return nil
}

// restart begins a fresh game. It re-seeds so each replay differs; the
// deterministic seed path is kept only for tests (which call tetris.NewGame
// directly).
func (m *Model) restart() {
	m.game = tetris.NewGame(time.Now().UnixNano())
	m.pauseSel = pauseResume
	m.trackBest()
}

func (m *Model) handleGravity(msg gravityTickMsg) (tea.Model, tea.Cmd) {
	// Ignore stale ticks from a previous gravity loop.
	if msg.seq != m.gravitySeq {
		return m, nil
	}
	if m.game.State() != tetris.Playing {
		// Stopped: do NOT reschedule => idle, near-zero CPU.
		return m, nil
	}
	m.game.Step()
	m.trackBest()
	if m.game.State() == tetris.GameOver {
		m.persistBest() // a gravity step can top out the board
		m.endDrag()     // the game-over transition ends any in-progress divider drag
	}
	// Continue the loop only while still playing.
	return m, m.startGravity()
}
