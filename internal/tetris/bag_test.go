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
