package tetris

import "testing"

// TestSevenBagPermutation asserts the defining 7-bag property: every disjoint
// window of 7 consecutive pieces is a permutation of all seven kinds. A
// regression to plain rand.Intn(7) would pass the other tests but break this.
func TestSevenBagPermutation(t *testing.T) {
	for _, seed := range []int64{1, 2, 42, 99, 123456} {
		seq := PieceSequence(seed, 70)
		if len(seq) != 70 {
			t.Fatalf("seed %d: seq len=%d want 70", seed, len(seq))
		}
		for start := 0; start+7 <= len(seq); start += 7 {
			var seen [7]int
			for _, k := range seq[start : start+7] {
				if k < 0 || int(k) >= 7 {
					t.Fatalf("seed %d: bad kind %v", seed, k)
				}
				seen[k]++
			}
			for k, c := range seen {
				if c != 1 {
					t.Errorf("seed %d window %d: kind %v appears %d times (want 1)",
						seed, start/7, PieceKind(k), c)
				}
			}
		}
	}
}

func TestPieceSequenceDeterministic(t *testing.T) {
	a := PieceSequence(42, 24)
	b := PieceSequence(42, 24)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic at index %d: %v vs %v", i, a[i], b[i])
		}
	}
}

// TestNextMatchesUpcomingSpawn pins the preview contract: g.Next is exactly the
// kind that the following spawn produces, and introducing the look-ahead did
// not disturb the deterministic 7-bag spawn order.
func TestNextMatchesUpcomingSpawn(t *testing.T) {
	seq := PieceSequence(2024, 6)
	g := NewGame(2024)
	if g.Current.Kind != seq[0] {
		t.Fatalf("current=%v want %v", g.Current.Kind, seq[0])
	}
	for i := 1; i < len(seq); i++ {
		if g.Next != seq[i] {
			t.Errorf("step %d: Next=%v want %v", i, g.Next, seq[i])
		}
		// Land the current piece so the previewed Next becomes Current.
		g.Filled = [Height][Width]bool{}
		g.Board = [Height][Width]int{}
		g.SetCurrent(Piece{Kind: g.Current.Kind, Rotation: 0, X: 4, Y: 18})
		g.HardDrop()
		if g.Current.Kind != seq[i] {
			t.Errorf("step %d: current after drop=%v want %v", i, g.Current.Kind, seq[i])
		}
	}
}

func TestLevelRisesEveryTenLines(t *testing.T) {
	g := NewGame(1)
	if g.Level() != 0 {
		t.Errorf("fresh level=%d want 0", g.Level())
	}
	g.Lines = 9
	if g.Level() != 0 {
		t.Errorf("9 lines level=%d want 0", g.Level())
	}
	g.Lines = 25
	if g.Level() != 2 {
		t.Errorf("25 lines level=%d want 2", g.Level())
	}
}

func TestGhostYIsLandingRowAndPure(t *testing.T) {
	g := NewGame(1)
	y0 := g.Current.Y
	gy := g.GhostY()
	if gy < y0 {
		t.Fatalf("ghostY=%d above piece Y=%d", gy, y0)
	}
	if g.CanPlace(g.Current.X, gy+1, g.Current.Rotation) {
		t.Errorf("piece can still fall below ghostY=%d (not the landing row)", gy)
	}
	if g.Current.Y != y0 {
		t.Errorf("GhostY mutated the piece: Y=%d want %d", g.Current.Y, y0)
	}
}

func TestHoldFirstThenSwapOncePerPiece(t *testing.T) {
	g := NewGame(1)
	first := g.Current.Kind

	// First hold parks the current piece and brings in a fresh one.
	if !g.Hold() {
		t.Fatal("first hold should succeed")
	}
	if !g.HasHeld || g.Held != first {
		t.Fatalf("after hold: held=%v hasHeld=%v want %v/true", g.Held, g.HasHeld, first)
	}
	// Hold is limited to once per piece.
	if g.Hold() {
		t.Error("second hold on the same piece should be blocked")
	}

	// Land the piece so a new one spawns (which re-enables hold).
	cur := g.Current.Kind
	g.Filled = [Height][Width]bool{}
	g.Board = [Height][Width]int{}
	g.SetCurrent(Piece{Kind: cur, Rotation: 0, X: 4, Y: 18})
	g.HardDrop()

	heldBefore, curBefore := g.Held, g.Current.Kind
	if !g.Hold() {
		t.Fatal("hold after a new piece should succeed")
	}
	if g.Current.Kind != heldBefore {
		t.Errorf("swap brought in %v, want held %v", g.Current.Kind, heldBefore)
	}
	if g.Held != curBefore {
		t.Errorf("swap parked %v, want %v", g.Held, curBefore)
	}
}

func TestHoldNoOpWhenNotPlaying(t *testing.T) {
	g := NewGame(1)
	g.SetState(Paused)
	if g.Hold() {
		t.Error("hold should be a no-op while paused")
	}
	if g.HasHeld {
		t.Error("paused hold must not park a piece")
	}
}

func TestStateCyclePauseResume(t *testing.T) {
	g := NewGame(1)
	if g.State() != Playing {
		t.Fatalf("fresh game state=%v want Playing", g.State())
	}
	g.SetState(Paused)
	if g.State() != Paused {
		t.Fatalf("SetState(Paused) failed: %v", g.State())
	}
	before := g.Current
	g.Step() // gravity is a no-op while paused
	if g.Current != before {
		t.Errorf("Step mutated the piece while paused")
	}
	g.SetState(Playing)
	if g.State() != Playing {
		t.Errorf("resume failed: %v", g.State())
	}
}

func TestGameOverIsTerminal(t *testing.T) {
	g := NewGame(1)
	// Block the spawn rows so the next piece collides on spawn.
	for y := 0; y < 2; y++ {
		for x := 0; x < Width; x++ {
			g.SetFilled(x, y, true, I)
		}
	}
	g.ForceSpawn()
	if g.State() != GameOver {
		t.Fatalf("expected GameOver after blocked spawn, got %v", g.State())
	}
	// GameOver is terminal: SetState(Playing) must be ignored.
	g.SetState(Playing)
	if g.State() != GameOver {
		t.Errorf("GameOver should be terminal, became %v", g.State())
	}
	// Movement/rotation are no-ops once the game is over.
	if g.MoveLeft() || g.MoveRight() || g.RotateCW() || g.SoftDrop() {
		t.Errorf("movement should be rejected after game over")
	}
}
