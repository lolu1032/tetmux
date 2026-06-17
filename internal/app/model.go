// Package app is the thin TUI glue that wires bubbletea + PTY + vt10x over the
// tested pure packages (tetris, router, layout, vtrender). It intentionally
// keeps no game/routing/layout arithmetic of its own — that all lives in the
// unit-tested packages — but the key-byte mapping, lifecycle, and scheduling
// decisions here are exercised by internal/app's own tests.
package app

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/layout"
	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

// gravityInterval is how often a falling piece descends one row.
const gravityInterval = 600 * time.Millisecond

// gravityTickMsg fires a gravity step. seq guards against stale ticks after the
// game stops so we never schedule more than one live gravity loop.
type gravityTickMsg struct{ seq int }

// ptyMsg is pushed by the left pane's reader goroutine whenever new output has
// arrived (coalesced via the pane's dirty bit). Receiving it triggers exactly
// one redraw; when the child is idle no ptyMsg is sent, so idle CPU is ~0.
type ptyMsg struct{}

// Model is the bubbletea model holding the left pane, the game, focus state,
// and terminal dimensions.
type Model struct {
	left     *leftPane
	game     *tetris.Game
	rstate   router.State
	width    int
	height   int
	renderer *lipgloss.Renderer

	gravitySeq int // increments each time we (re)start gravity

	argv     []string
	seed     int64
	send     func(tea.Msg) // injected from main after the program is built
	spawnErr error         // non-nil if the left command failed to start
}

// New constructs a Model. The left pane is created lazily on the first
// WindowSizeMsg so the PTY is sized correctly.
func New(argv []string, seed int64) *Model {
	return &Model{
		game:     tetris.NewGame(seed),
		rstate:   router.State{Focus: router.FocusLeft},
		renderer: lipgloss.NewRenderer(os.Stdout),
		argv:     argv,
		seed:     seed,
	}
}

// SetSend injects the program's thread-safe Send func so the left pane's reader
// goroutine can push a redraw when PTY output arrives. Must be called before
// Run() (i.e. before the reader goroutine could exist), so there is no race.
func (m *Model) SetSend(fn func(tea.Msg)) { m.send = fn }

// SpawnErr returns the error from a failed left-command spawn, if any, so the
// caller can exit non-zero after the program ends.
func (m *Model) SpawnErr() error { return m.spawnErr }

// Close terminates the left child and PTY. Safe to call more than once and
// when no pane was ever created.
func (m *Model) Close() {
	if m.left != nil {
		m.left.Close()
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
	return tea.Tick(gravityInterval, func(time.Time) tea.Msg {
		return gravityTickMsg{seq: seq}
	})
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
		return m.handleKey(msg)
	case gravityTickMsg:
		return m.handleGravity(msg)
	case ptyMsg:
		// New PTY output arrived. Clear the dirty bit so the next output
		// re-notifies, then fall through to a redraw (bubbletea calls View
		// after Update returns).
		if m.left != nil {
			m.left.ConsumeDirty()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	l := layout.Compute(m.width, m.height, layout.DefaultBorders())

	cols := l.LeftInnerWidth
	rows := l.InnerHeight
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}

	if m.left == nil {
		// Lazily create the left pane sized to its inner dimensions. Capture
		// any spawn error so the View can show it instead of hanging.
		lp, err := newLeftPane(m.argv, cols, rows, m.notify())
		if err != nil {
			m.spawnErr = err
		} else {
			m.left = lp
		}
		return m, nil
	}

	m.left.Resize(cols, rows)
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := toRouterKey(msg)
	act := router.Route(m.rstate, key)
	m.rstate = act.NewState

	switch act.Command {
	case router.CmdQuit:
		m.Close()
		return m, tea.Quit
	}

	switch act.RouteTo {
	case router.RouteLeft:
		if m.left != nil && len(act.Bytes) > 0 {
			m.left.Write(act.Bytes)
		}
	case router.RouteRight:
		return m, m.handleGameKey(msg)
	}

	return m, nil
}

// handleGameKey applies a key to the Tetris game when the right pane is
// focused, restarting gravity if a previously-stopped game becomes active.
func (m *Model) handleGameKey(msg tea.KeyMsg) tea.Cmd {
	wasPlaying := m.game.State() == tetris.Playing
	switch msg.String() {
	case "left", "h":
		m.game.MoveLeft()
	case "right", "l":
		m.game.MoveRight()
	case "up", "x":
		m.game.RotateCW()
	case "z":
		m.game.RotateCCW()
	case "down", "j":
		m.game.SoftDrop()
	case " ", "spacebar", "space":
		m.game.HardDrop()
	case "p":
		if m.game.State() == tetris.Paused {
			m.game.SetState(tetris.Playing)
		} else {
			m.game.SetState(tetris.Paused)
		}
	case "r":
		if m.game.State() == tetris.GameOver {
			// Re-seed so each replay differs; the deterministic seed path is
			// kept only for tests (which call tetris.NewGame directly).
			m.game = tetris.NewGame(time.Now().UnixNano())
		}
	}
	// If the game just became (or resumed) playing and no gravity loop is
	// running, start one.
	if !wasPlaying && m.game.State() == tetris.Playing {
		return m.startGravity()
	}
	return nil
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
	// Continue the loop only while still playing.
	return m, m.startGravity()
}

// View composes the two panes and a status bar.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "initializing tetmux..."
	}
	l := layout.Compute(m.width, m.height, layout.DefaultBorders())

	leftBox := m.renderLeft(l)
	rightBox := m.renderRight(l)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	status := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
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
	var content string
	switch {
	case m.spawnErr != nil:
		content = leftErrorContent(m.argv, m.spawnErr, w, h)
	case m.left == nil:
		content = "starting..."
	default:
		rows := m.left.RenderRows(w, h, focused)
		if m.left.Exited() && len(rows) > 0 {
			rows[0] = clipPad(exitBanner(m.left.ExitCode()), w)
		}
		content = joinRows(rows)
	}

	border := lipgloss.RoundedBorder()
	style := m.renderer.NewStyle().Border(border).Width(w).Height(h)
	if focused {
		style = style.BorderForeground(lipgloss.Color("51"))
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
	}

	if l.TooSmall {
		return style.Render("window too small")
	}

	rows := renderTetris(m.game, m.renderer)
	return style.Render(joinRows(rows))
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
	exited := ""
	if m.left != nil && m.left.Exited() {
		exited = " | child exited"
	}
	text := " focus:" + focus + prefix +
		" | score:" + itoa(m.game.Score) +
		" | lines:" + itoa(m.game.Lines) +
		" | " + gameStatusLine(m.game) +
		exited +
		" | C-b q quit, C-b h/l switch"
	return m.renderer.NewStyle().
		Reverse(true).
		Width(maxInt(m.width, 1)).
		Render(clipPad(text, m.width))
}
