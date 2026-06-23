package app

import (
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/tetris"
)

// TetrisModel is the standalone Tetris TUI behind `tetmux --tetris-only`. In the
// default (tmux) mode this runs in its OWN tmux pane next to the command pane,
// so it carries none of the PTY / terminal-emulator / window-management
// machinery of the embedded Model — tmux owns all of that. It is just the game:
// the pure tetris package plus the shared render helpers, pause/game-over
// overlays, gravity timing, and high-score persistence.
type TetrisModel struct {
	game     *tetris.Game
	renderer *lipgloss.Renderer
	width    int
	height   int

	// pauseSel mirrors Model.pauseSel: the highlighted item in the pause menu.
	pauseSel   int
	best       int
	bestOnce   sync.Once
	gravitySeq int
}

// NewTetris constructs the standalone game model seeded with seed.
func NewTetris(seed int64) *TetrisModel {
	return &TetrisModel{
		game:     tetris.NewGame(seed),
		renderer: lipgloss.NewRenderer(os.Stdout),
		best:     loadBestScore(),
	}
}

// Init starts the gravity loop.
func (m *TetrisModel) Init() tea.Cmd { return m.startGravity() }

// Close persists the best score exactly once (mirrors Model.Close).
func (m *TetrisModel) Close() { m.bestOnce.Do(func() { saveBestScore(m.best) }) }

// startGravity schedules a gravity tick only while playing (see Model.startGravity).
func (m *TetrisModel) startGravity() tea.Cmd {
	if m.game.State() != tetris.Playing {
		return nil
	}
	m.gravitySeq++
	seq := m.gravitySeq
	return tea.Tick(tetris.GravityInterval(m.game.Level()), func(time.Time) tea.Msg {
		return gravityTickMsg{seq: seq}
	})
}

func (m *TetrisModel) trackBest() {
	if m.game.Score > m.best {
		m.best = m.game.Score
	}
}

// persistBest eagerly saves the best score at game-over (idempotent), mirroring
// Model.persistBest so a standalone-game high score also survives an abrupt exit.
func (m *TetrisModel) persistBest() { saveBestScore(m.best) }

// Update routes window/key/gravity events.
func (m *TetrisModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		// Ctrl-C or q leaves the game (closes the tmux pane). Esc is reserved for
		// pause, so it is NOT a quit key.
		if s := msg.String(); s == "ctrl+c" || s == "q" {
			m.Close()
			return m, tea.Quit
		}
		return m, m.handleGameKey(msg)
	case gravityTickMsg:
		if msg.seq != m.gravitySeq || m.game.State() != tetris.Playing {
			return m, nil
		}
		m.game.Step()
		m.trackBest()
		if m.game.State() == tetris.GameOver {
			m.persistBest()
		}
		return m, m.startGravity()
	}
	return m, nil
}

// handleGameKey applies a gameplay key, delegating to the pause/game-over menu
// handlers while the game is stopped — the same control scheme as the embedded
// right pane (Model.handleGameKey), minus focus/routing.
func (m *TetrisModel) handleGameKey(msg tea.KeyMsg) tea.Cmd {
	if m.game.State() == tetris.Paused {
		return m.handlePauseMenuKey(msg)
	}
	if m.game.State() == tetris.GameOver {
		return m.handleGameOverKey(msg)
	}

	// Shared movement mapping (see tetris.ApplyKey); lifecycle keys handled here.
	if !m.game.ApplyKey(msg.String()) {
		switch msg.String() {
		case "p", "esc":
			m.game.SetState(tetris.Paused)
			m.pauseSel = pauseResume
		case "r":
			m.restart()
			return m.startGravity()
		}
	}
	m.trackBest()
	if m.game.State() == tetris.GameOver {
		m.persistBest() // a hard drop / hold can top out the board
	}
	return nil
}

func (m *TetrisModel) handlePauseMenuKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k", "left", "h":
		m.pauseSel = (m.pauseSel - 1 + pauseMenuItems) % pauseMenuItems
	case "down", "j", "right", "l":
		m.pauseSel = (m.pauseSel + 1) % pauseMenuItems
	case "esc", "p":
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

func (m *TetrisModel) handleGameOverKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter", "r":
		m.restart()
		return m.startGravity()
	}
	return nil
}

func (m *TetrisModel) restart() {
	m.game = tetris.NewGame(time.Now().UnixNano())
	m.pauseSel = pauseResume
	m.trackBest()
}

// View centers the board (with the HOLD/NEXT panel when wide enough) over the
// pane, or the pause / game-over overlay while stopped.
func (m *TetrisModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "tetmux tetris..."
	}
	switch m.game.State() {
	case tetris.Paused:
		return m.center(renderPauseMenu(m.renderer, m.pauseSel))
	case tetris.GameOver:
		return m.center(renderGameOverMenu(m.renderer, m.game.Score, m.best))
	}

	board := joinRows(renderTetris(m.game, m.renderer))
	if m.width >= tetris.Width*2+previewGap+previewCols {
		side := renderSidePanel(m.game, m.renderer)
		gap := m.renderer.NewStyle().Width(previewGap).Render("")
		board = lipgloss.JoinHorizontal(lipgloss.Top, board, gap, side)
	}
	status := m.renderer.NewStyle().Faint(true).Render(m.statusText())
	// Two short ASCII lines so the hint fits the ~40-column Tetris pane and
	// avoids ambiguous-width glyphs (arrows/·) that could misalign.
	hintStyle := m.renderer.NewStyle().Faint(true)
	hint1 := hintStyle.Render("move:arrows/hjkl  rot:x/z  drop:space")
	hint2 := hintStyle.Render("hold:c  pause:Esc  restart:r  quit:q")
	body := lipgloss.JoinVertical(lipgloss.Center, board, "", status, hint1, hint2)
	return m.center(body)
}

func (m *TetrisModel) center(s string) string {
	return lipgloss.Place(maxInt(m.width, 1), maxInt(m.height, 1), lipgloss.Center, lipgloss.Center, s)
}

func (m *TetrisModel) statusText() string {
	t := "score " + itoa(m.game.Score) + "  lv " + itoa(m.game.Level())
	if m.best > 0 {
		t += "  best " + itoa(m.best)
	}
	return t
}
