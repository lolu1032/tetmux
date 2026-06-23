package tetris

import "testing"

func TestApplyKeyMovesAndReports(t *testing.T) {
	g := NewGame(1)
	x0 := g.Current.X
	if !g.ApplyKey("left") || g.Current.X != x0-1 {
		t.Errorf("left should move the piece left")
	}
	if !g.ApplyKey("right") || g.Current.X != x0 {
		t.Errorf("right should move the piece right")
	}
	// Letter aliases work too.
	if !g.ApplyKey("h") || g.Current.X != x0-1 {
		t.Errorf("h alias should move left")
	}
	// Hold is a recognized move.
	if !g.ApplyKey("c") || !g.HasHeld {
		t.Errorf("c should hold")
	}
	// Lifecycle / unknown keys are NOT moves.
	for _, k := range []string{"p", "esc", "r", "q", "ctrl+c", ""} {
		if g.ApplyKey(k) {
			t.Errorf("%q should not be treated as a movement key", k)
		}
	}
}

func TestApplyKeyHardDropLocks(t *testing.T) {
	g := NewGame(1)
	before := 0
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if g.Filled[y][x] {
				before++
			}
		}
	}
	if !g.ApplyKey("space") {
		t.Fatal("space should be a recognized move")
	}
	after := 0
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if g.Filled[y][x] {
				after++
			}
		}
	}
	if after <= before {
		t.Errorf("space (hard drop) should lock cells: before=%d after=%d", before, after)
	}
}
