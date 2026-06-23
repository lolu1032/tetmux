package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/tetris"
)

// drives a key through the standalone TetrisModel.Update, returning the cmd.
func tdrive(m *TetrisModel, k tea.KeyMsg) tea.Cmd {
	_, cmd := m.Update(k)
	return cmd
}

func TestTetrisModelMovesAndDrops(t *testing.T) {
	m := NewTetris(1)
	x0 := m.game.Current.X
	tdrive(m, keyType(tea.KeyLeft))
	if m.game.Current.X != x0-1 {
		t.Errorf("left: X=%d want %d", m.game.Current.X, x0-1)
	}
	before := countFilled(m.game)
	tdrive(m, keyType(tea.KeySpace))
	if countFilled(m.game) <= before {
		t.Errorf("space should hard-drop and lock the piece")
	}
}

func TestTetrisModelPauseMenuAndRestart(t *testing.T) {
	m := NewTetris(1)
	// Esc opens the pause menu on the resume item.
	tdrive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.game.State() != tetris.Paused || m.pauseSel != pauseResume {
		t.Fatalf("Esc should pause on resume item: state=%v sel=%d", m.game.State(), m.pauseSel)
	}
	// Down → 재시작, Enter restarts into a playing game.
	tdrive(m, keyType(tea.KeyDown))
	if m.pauseSel != pauseRestart {
		t.Fatalf("Down should select restart, sel=%d", m.pauseSel)
	}
	cmd := tdrive(m, keyType(tea.KeyEnter))
	if m.game.State() != tetris.Playing {
		t.Errorf("Enter on 재시작 should restart, state=%v", m.game.State())
	}
	if cmd == nil {
		t.Errorf("restart should resume the gravity loop")
	}
}

func TestTetrisModelGameOverEnterRestarts(t *testing.T) {
	m := NewTetris(1)
	for y := 0; y < 2; y++ {
		for x := 0; x < tetris.Width; x++ {
			m.game.SetFilled(x, y, true, tetris.I)
		}
	}
	m.game.ForceSpawn()
	if m.game.State() != tetris.GameOver {
		t.Fatalf("expected GameOver, got %v", m.game.State())
	}
	tdrive(m, keyType(tea.KeyEnter))
	if m.game.State() != tetris.Playing {
		t.Errorf("Enter on game-over should restart, state=%v", m.game.State())
	}
}

func TestTetrisModelQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, runeKey('q')} {
		m := NewTetris(1)
		cmd := tdrive(m, k)
		if cmd == nil {
			t.Errorf("%v should return a quit command", k)
			continue
		}
		if msg := cmd(); msg == nil {
			t.Errorf("%v quit cmd should yield a tea.QuitMsg", k)
		}
	}
}

func TestTetrisModelViewRendersBoard(t *testing.T) {
	m := NewTetris(1)
	m.width, m.height = 60, 30
	out := m.View()
	if !strings.Contains(out, blockCell) && !strings.Contains(out, emptyCell) {
		t.Errorf("playing view should render the board grid")
	}
	// Pause overlay shows the menu instead.
	tdrive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if pv := m.View(); !strings.Contains(pv, "계속하기") {
		t.Errorf("paused view should show the pause menu")
	}
}
