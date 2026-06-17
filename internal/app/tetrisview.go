package app

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/tetris"
)

// blockColors maps each tetromino kind to a lipgloss color for its blocks.
var blockColors = map[tetris.PieceKind]lipgloss.Color{
	tetris.I: lipgloss.Color("51"),  // cyan
	tetris.O: lipgloss.Color("226"), // yellow
	tetris.T: lipgloss.Color("129"), // purple
	tetris.S: lipgloss.Color("46"),  // green
	tetris.Z: lipgloss.Color("196"), // red
	tetris.J: lipgloss.Color("21"),  // blue
	tetris.L: lipgloss.Color("208"), // orange
}

// blockCell is the two-column glyph used for a filled block.
const blockCell = "[]"
const emptyCell = " ."

// Pre-rendered cell strings. The styled output for a given (color, glyph) is
// constant for the process's color profile, so we build the 8 strings once
// (7 piece colors + the faint empty cell) instead of ~200 NewStyle().Render()
// calls every frame.
var (
	cellOnce  sync.Once
	emptyStr  string
	blockStrs [7]string
)

func initCellStrings(renderer *lipgloss.Renderer) {
	cellOnce.Do(func() {
		emptyStr = renderer.NewStyle().Faint(true).Render(emptyCell)
		for k := range blockStrs {
			blockStrs[k] = renderer.NewStyle().
				Foreground(blockColors[tetris.PieceKind(k)]).
				Render(blockCell)
		}
	})
}

// renderTetris renders the current game board into styled string rows. Each
// board column is two terminal columns wide so blocks look roughly square.
// The result is suitable for placing inside the right pane.
func renderTetris(g *tetris.Game, renderer *lipgloss.Renderer) []string {
	// Build an occupancy + color overlay: locked cells from the board plus the
	// live falling piece.
	var occupied [tetris.Height][tetris.Width]int // 0 empty, else kind+1
	for y := 0; y < tetris.Height; y++ {
		for x := 0; x < tetris.Width; x++ {
			occupied[y][x] = g.Board[y][x]
		}
	}
	if g.State() != tetris.GameOver {
		for _, c := range g.Current.Cells() {
			if c.Y >= 0 && c.Y < tetris.Height && c.X >= 0 && c.X < tetris.Width {
				occupied[c.Y][c.X] = int(g.Current.Kind) + 1
			}
		}
	}

	initCellStrings(renderer)
	rows := make([]string, 0, tetris.Height)
	for y := 0; y < tetris.Height; y++ {
		var sb strings.Builder
		for x := 0; x < tetris.Width; x++ {
			v := occupied[y][x]
			if v == 0 {
				sb.WriteString(emptyStr)
				continue
			}
			kind := int(v - 1)
			if kind < 0 || kind >= len(blockStrs) {
				sb.WriteString(emptyStr)
				continue
			}
			sb.WriteString(blockStrs[kind])
		}
		rows = append(rows, sb.String())
	}
	return rows
}

// gameOverOverlay returns a short status string for the game state.
func gameStatusLine(g *tetris.Game) string {
	switch g.State() {
	case tetris.GameOver:
		return "GAME OVER - prefix+r to restart"
	case tetris.Paused:
		return "PAUSED"
	default:
		return "playing"
	}
}
