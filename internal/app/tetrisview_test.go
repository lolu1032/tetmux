package app

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/tetris"
)

// TestRenderTetrisRowWidths asserts the board renders Height rows, each exactly
// Width*2 visible columns wide (two terminal columns per block), so the right
// pane never over/underflows its border.
func TestRenderTetrisRowWidths(t *testing.T) {
	r := lipgloss.NewRenderer(os.Stdout)
	g := tetris.NewGame(7)
	rows := renderTetris(g, r)
	if len(rows) != tetris.Height {
		t.Fatalf("rows=%d want %d", len(rows), tetris.Height)
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != tetris.Width*2 {
			t.Errorf("row %d visible width=%d want %d", i, w, tetris.Width*2)
		}
	}
}

func TestGameStatusLine(t *testing.T) {
	g := tetris.NewGame(1)
	if got := gameStatusLine(g); got != "playing" {
		t.Errorf("playing: got %q", got)
	}
	g.SetState(tetris.Paused)
	if got := gameStatusLine(g); got != "PAUSED" {
		t.Errorf("paused: got %q", got)
	}

	over := tetris.NewGame(1)
	for y := 0; y < 2; y++ {
		for x := 0; x < tetris.Width; x++ {
			over.SetFilled(x, y, true, tetris.I)
		}
	}
	over.ForceSpawn()
	if over.State() != tetris.GameOver {
		t.Fatalf("expected GameOver, got %v", over.State())
	}
	if got := gameStatusLine(over); got != "GAME OVER - prefix+r to restart" {
		t.Errorf("gameover: got %q", got)
	}
}
