package tetris

// ApplyKey applies a gameplay MOVE key to the game and reports whether the key
// was a recognized move. The key names match bubbletea's KeyMsg.String() (arrow
// names plus the letter aliases) so the embedded right pane and the standalone
// --tetris-only model share ONE mapping instead of duplicating the switch.
//
// Only movement keys are handled here; pause/restart and menu navigation are
// lifecycle concerns the callers own (they differ between the two models), so an
// unrecognized key returns false and the caller decides what to do with it.
func (g *Game) ApplyKey(key string) bool {
	switch key {
	case "left", "h":
		g.MoveLeft()
	case "right", "l":
		g.MoveRight()
	case "up", "x":
		g.RotateCW()
	case "z":
		g.RotateCCW()
	case "down", "j":
		g.SoftDrop()
	case " ", "spacebar", "space":
		g.HardDrop()
	case "c":
		g.Hold()
	default:
		return false
	}
	return true
}
