package tetris

import "testing"

func TestGravityInterval(t *testing.T) {
	// Level 0 is the slow baseline; it speeds up per level and floors out.
	if g0, g1 := GravityInterval(0), GravityInterval(1); !(g0 > g1) {
		t.Errorf("higher level should fall faster: lvl0=%v lvl1=%v", g0, g1)
	}
	// Deep levels clamp at the floor and never go below it (or invert).
	floor := GravityInterval(1000)
	if floor <= 0 {
		t.Errorf("gravity must stay positive at high levels, got %v", floor)
	}
	if GravityInterval(999) < floor {
		t.Errorf("gravity must be monotonic down to the floor")
	}
}
