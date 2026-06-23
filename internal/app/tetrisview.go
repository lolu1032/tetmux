package app

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/tetris"
)

// blockColors maps each tetromino kind to a lipgloss color for its blocks.
// Colors are picked bright enough to read clearly on a dark terminal (the old
// J blue 21 was nearly invisible).
var blockColors = map[tetris.PieceKind]lipgloss.Color{
	tetris.I: lipgloss.Color("51"),  // cyan
	tetris.O: lipgloss.Color("226"), // yellow
	tetris.T: lipgloss.Color("171"), // magenta-purple
	tetris.S: lipgloss.Color("46"),  // green
	tetris.Z: lipgloss.Color("196"), // red
	tetris.J: lipgloss.Color("39"),  // bright blue
	tetris.L: lipgloss.Color("208"), // orange
}

// Cell glyphs — each is exactly TWO terminal columns so the 10-wide board is 20
// columns. Filled cells use a SOLID block so it is obvious how the well fills
// up; empty cells are a faint grid dot; the ghost is a dim shaded outline of
// where the piece will land.
const blockCell = "██"
const emptyCell = " ·"
const ghostCell = "░░"

// Pre-rendered cell strings. The styled output for a given (color, glyph) is
// constant for the process's color profile, so we build the 8 strings once
// (7 piece colors + the faint empty cell) instead of ~200 NewStyle().Render()
// calls every frame.
var (
	cellOnce  sync.Once
	emptyStr  string
	ghostStr  string
	blockStrs [7]string
)

func initCellStrings(renderer *lipgloss.Renderer) {
	cellOnce.Do(func() {
		emptyStr = renderer.NewStyle().Faint(true).Render(emptyCell)
		ghostStr = renderer.NewStyle().Foreground(lipgloss.Color("243")).Render(ghostCell)
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
	// live falling piece, with a "ghost" overlay marking where it would land.
	var occupied [tetris.Height][tetris.Width]int // 0 empty, else kind+1
	var ghost [tetris.Height][tetris.Width]bool
	for y := 0; y < tetris.Height; y++ {
		for x := 0; x < tetris.Width; x++ {
			occupied[y][x] = g.Board[y][x]
		}
	}
	if g.State() != tetris.GameOver {
		// Ghost first, so the live piece (and any overlap when it's resting)
		// draws over it.
		gy := g.GhostY()
		for _, c := range g.Current.CellsAt(g.Current.X, gy, g.Current.Rotation) {
			if c.Y >= 0 && c.Y < tetris.Height && c.X >= 0 && c.X < tetris.Width {
				ghost[c.Y][c.X] = true
			}
		}
		for _, c := range g.Current.Cells() {
			if c.Y >= 0 && c.Y < tetris.Height && c.X >= 0 && c.X < tetris.Width {
				occupied[c.Y][c.X] = int(g.Current.Kind) + 1
				ghost[c.Y][c.X] = false
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
				if ghost[y][x] {
					sb.WriteString(ghostStr)
				} else {
					sb.WriteString(emptyStr)
				}
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

// previewCols is the visible width of the next-piece preview column (4 cells *
// 2 terminal columns); previewGap is the spacing between the board and it.
const (
	previewCols = 8
	previewGap  = 2
)

// renderPieceBox renders a small labeled 4x4 box showing one tetromino (or an
// empty box when present is false), using the same colored cells as the board.
// It returns a label row followed by four grid rows, each previewCols wide.
func renderPieceBox(label string, kind tetris.PieceKind, present bool, renderer *lipgloss.Renderer) []string {
	initCellStrings(renderer)

	var occ [4][4]bool
	if present {
		for _, c := range (tetris.Piece{Kind: kind, Rotation: 0}).Cells() {
			if c.X >= 0 && c.X < 4 && c.Y >= 0 && c.Y < 4 {
				occ[c.Y][c.X] = true
			}
		}
	}

	cell := emptyStr
	if present && int(kind) >= 0 && int(kind) < len(blockStrs) {
		cell = blockStrs[int(kind)]
	}

	rows := make([]string, 0, 5)
	rows = append(rows, renderer.NewStyle().Faint(true).Render(label))
	for y := 0; y < 4; y++ {
		var sb strings.Builder
		for x := 0; x < 4; x++ {
			if occ[y][x] {
				sb.WriteString(cell)
			} else {
				sb.WriteString(emptyStr)
			}
		}
		rows = append(rows, sb.String())
	}
	return rows
}

// renderSidePanel stacks the HOLD box above the NEXT box for the right pane.
func renderSidePanel(g *tetris.Game, renderer *lipgloss.Renderer) string {
	hold := renderPieceBox("HOLD", g.Held, g.HasHeld, renderer)
	next := renderPieceBox("NEXT", g.Next, true, renderer)
	return joinRows(hold) + "\n\n" + joinRows(next)
}

// pauseMenuLabels are the selectable items in the pause overlay, indexed to
// match the pauseResume/pauseRestart consts in model.go.
var pauseMenuLabels = []string{"계속하기", "재시작"}

// renderPauseMenu returns the centered "paused" overlay shown while the game is
// stopped. It draws a selectable menu (계속하기 / 재시작) with the item at
// `selected` highlighted, plus a one-line key hint.
func renderPauseMenu(renderer *lipgloss.Renderer, selected int) string {
	title := renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("226")).Render("⏸  일시정지 / PAUSED")

	rows := make([]string, len(pauseMenuLabels))
	for i, label := range pauseMenuLabels {
		if i == selected {
			rows[i] = renderer.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("226")).
				Render(" ▶ " + label + " ")
		} else {
			rows[i] = renderer.NewStyle().Faint(true).Render("   " + label + " ")
		}
	}
	menu := strings.Join(rows, "\n")

	hint := renderer.NewStyle().Faint(true).Render("↑↓ 이동 · Enter 선택 · Esc 계속")
	box := renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("226")).
		Padding(1, 3)
	return box.Render(title + "\n\n" + menu + "\n\n" + hint)
}

// renderGameOverMenu returns the centered overlay shown after a loss: a
// "게임 오버" banner, the final (and best) score, and the 재시작 prompt. It
// mirrors renderPauseMenu's framed look but in red.
func renderGameOverMenu(renderer *lipgloss.Renderer, score, best int) string {
	title := renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("💀  게임 오버 / GAME OVER")

	scoreLine := renderer.NewStyle().Render("점수 " + itoa(score))
	if best > 0 {
		scoreLine += renderer.NewStyle().Faint(true).Render("   최고 " + itoa(best))
	}

	item := renderer.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("196")).
		Render(" ▶ 재시작 ")

	hint := renderer.NewStyle().Faint(true).Render("Enter 또는 r: 재시작")
	box := renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 3)
	return box.Render(title + "\n\n" + scoreLine + "\n\n" + item + "\n\n" + hint)
}

// gameOverOverlay returns a short status string for the game state.
func gameStatusLine(g *tetris.Game) string {
	switch g.State() {
	case tetris.GameOver:
		// Restart is a plain 'r' delivered to the focused right pane (see
		// handleGameKey); the router has no prefix+r command, so "prefix+r"
		// would be a no-op. Tell the player the key that actually works.
		return "GAME OVER - press r to restart"
	case tetris.Paused:
		return "PAUSED"
	default:
		return "playing"
	}
}
