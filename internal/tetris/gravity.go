package tetris

import "time"

// Gravity timing belongs with the game, not the TUI glue: a piece descends every
// baseGravity at level 0, getting levelStep faster per level down to a minGravity
// floor. Level rises every 10 cleared lines (see Game.Level).
const (
	baseGravity = 600 * time.Millisecond
	levelStep   = 45 * time.Millisecond
	minGravity  = 90 * time.Millisecond
)

// GravityInterval returns the per-row fall delay for a given level, clamped at
// minGravity so the game stays playable at high levels. It is pure, so the speed
// curve is unit-tested here instead of in the glue layer.
func GravityInterval(level int) time.Duration {
	d := baseGravity - time.Duration(level)*levelStep
	if d < minGravity {
		d = minGravity
	}
	return d
}
