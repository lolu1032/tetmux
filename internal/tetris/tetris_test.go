package tetris

import (
	"reflect"
	"sort"
	"testing"
)

// sortedCells returns a copy of cells sorted for order-independent comparison.
func sortedCells(cs []Cell) []Cell {
	out := append([]Cell(nil), cs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

func cellsEqual(a, b []Cell) bool {
	return reflect.DeepEqual(sortedCells(a), sortedCells(b))
}

// --- pieces: shape definitions and cell counts ---

func TestPieceCellCounts(t *testing.T) {
	for _, k := range AllKinds {
		for rot := 0; rot < 4; rot++ {
			cells := shapeCells(k, rot)
			if len(cells) != 4 {
				t.Errorf("piece %c rotation %d: got %d cells, want 4", k.Rune(), rot, len(cells))
			}
			// No duplicate cells within a rotation state.
			seen := map[Cell]bool{}
			for _, c := range cells {
				if seen[c] {
					t.Errorf("piece %c rotation %d: duplicate cell %+v", k.Rune(), rot, c)
				}
				seen[c] = true
			}
		}
	}
}

func TestAllSevenDefined(t *testing.T) {
	for _, k := range AllKinds {
		if _, ok := spawnShapes[k]; !ok {
			t.Errorf("piece %c not defined", k.Rune())
		}
	}
	if len(AllKinds) != 7 {
		t.Fatalf("expected 7 kinds, got %d", len(AllKinds))
	}
}

func TestOPieceRotationInvariant(t *testing.T) {
	base := sortedCells(shapeCells(O, 0))
	for rot := 1; rot < 4; rot++ {
		got := sortedCells(shapeCells(O, rot))
		if !reflect.DeepEqual(base, got) {
			t.Errorf("O piece rotation %d differs from base: %v vs %v", rot, got, base)
		}
	}
}

// --- rotation ---

func TestRotateIFourTimesReturns(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: I, Rotation: 0, X: 3, Y: 5})
	orig := g.Current.Cells()
	for i := 0; i < 4; i++ {
		g.RotateCW()
	}
	if !cellsEqual(orig, g.Current.Cells()) {
		t.Errorf("I rotated 4x not original: %v vs %v", g.Current.Cells(), orig)
	}
}

func TestRotateTCWThenCCWReturns(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: T, Rotation: 0, X: 3, Y: 5})
	orig := g.Current.Cells()
	g.RotateCW()
	g.RotateCCW()
	if !cellsEqual(orig, g.Current.Cells()) {
		t.Errorf("T CW then CCW not original: %v vs %v", g.Current.Cells(), orig)
	}
}

func TestRotationClampedInBounds(t *testing.T) {
	// Place an I piece flush against the right wall, then rotate; the
	// wall-kick must keep all cells inside [0,9] x [0,19].
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: I, Rotation: 0, X: 6, Y: 5}) // cells at x=6..9
	g.RotateCW()
	for _, c := range g.Current.Cells() {
		if c.X < 0 || c.X >= Width || c.Y < 0 || c.Y >= Height {
			t.Errorf("rotation produced out-of-bounds cell %+v", c)
		}
	}
}

func TestRotationBlockedByLockedCellNeverOverlaps(t *testing.T) {
	g := NewGame(1)
	// Fill a wide locked region around where a rotation would expand.
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			g.SetFilled(x, y, false, I)
		}
	}
	// Surround the piece tightly so any rotation that overlaps is rejected.
	g.SetCurrent(Piece{Kind: T, Rotation: 0, X: 3, Y: 5})
	// Lock cells in all positions the rotated T could occupy except its current.
	for y := 4; y <= 8; y++ {
		for x := 0; x < Width; x++ {
			// leave the current occupied cells free
			occ := false
			for _, c := range g.Current.Cells() {
				if c.X == x && c.Y == y {
					occ = true
				}
			}
			if !occ {
				g.SetFilled(x, y, true, Z)
			}
		}
	}
	g.RotateCW()
	// Whatever happened, the piece must never overlap a locked cell.
	for _, c := range g.Current.Cells() {
		if g.Filled[c.Y][c.X] {
			t.Errorf("rotated piece overlaps locked cell at %+v", c)
		}
	}
}

// --- collision: walls ---

func TestMoveLeftAtWallNoOp(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: -1, Y: 5}) // O occupies x=0,1
	x := g.Current.X
	if g.MoveLeft() {
		t.Errorf("MoveLeft at left wall should be no-op")
	}
	if g.Current.X != x {
		t.Errorf("piece x changed on blocked move: %d -> %d", x, g.Current.X)
	}
}

func TestMoveRightAtWallNoOp(t *testing.T) {
	g := NewGame(1)
	// O occupies local x=1,2 so origin X=7 -> cells at 8,9 (rightmost = 9).
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 7, Y: 5})
	x := g.Current.X
	if g.MoveRight() {
		t.Errorf("MoveRight at right wall should be no-op")
	}
	if g.Current.X != x {
		t.Errorf("piece x changed on blocked move: %d -> %d", x, g.Current.X)
	}
}

func TestPieceStaysInBoundsDuringMoves(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: T, Rotation: 0, X: 3, Y: 5})
	for i := 0; i < 20; i++ {
		g.MoveLeft()
	}
	for i := 0; i < 40; i++ {
		g.MoveRight()
	}
	for _, c := range g.Current.Cells() {
		if c.X < 0 || c.X >= Width {
			t.Errorf("piece left horizontal bounds: %+v", c)
		}
	}
}

// --- collision: floor ---

func TestPieceOnFloorCannotMoveDown(t *testing.T) {
	g := NewGame(1)
	// O piece occupies local rows 0,1; put bottom at row 19 => origin Y=18.
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 18})
	if g.CanPlace(g.Current.X, g.Current.Y+1, g.Current.Rotation) {
		t.Errorf("piece on floor should not be able to move down")
	}
	cells := g.Current.CellsAt(g.Current.X, g.Current.Y+1, g.Current.Rotation)
	if !g.Collides(cells) {
		t.Errorf("collision against floor not detected")
	}
}

// --- collision: locked ---

func TestPieceAboveLockedCannotMoveDown(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 10}) // cells y=10,11 x=5,6
	// Lock cells directly below.
	g.SetFilled(5, 12, true, Z)
	g.SetFilled(6, 12, true, Z)
	if g.CanPlace(g.Current.X, g.Current.Y+1, g.Current.Rotation) {
		t.Errorf("piece above locked cell should not move down")
	}
	cells := g.Current.CellsAt(g.Current.X, g.Current.Y+1, g.Current.Rotation)
	if !g.Collides(cells) {
		t.Errorf("collision against locked cells not detected")
	}
}

// --- line clear ---

// fillRowExcept fills a row fully except for the given column.
func fillRow(g *Game, y int) {
	for x := 0; x < Width; x++ {
		g.SetFilled(x, y, true, Z)
	}
}

func TestLineClearSingle(t *testing.T) {
	g := NewGame(1)
	g.Score = 0
	// Fill bottom row except column 4; mark a piece (O) to complete it.
	for x := 0; x < Width; x++ {
		if x != 4 && x != 5 {
			g.SetFilled(x, 19, true, Z)
		}
	}
	// Put an O so its bottom completes row 19 at columns 4,5.
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 3, Y: 18}) // cells x4,5 y18,19
	cleared := g.LockCurrent()
	if cleared != 1 {
		t.Fatalf("expected 1 line cleared, got %d", cleared)
	}
	if g.Score != 100 {
		t.Errorf("expected score 100, got %d", g.Score)
	}
	// Row 19 should now contain the upper part of the O (shifted down).
	if !g.Filled[19][4] || !g.Filled[19][5] {
		t.Errorf("upper O cells not shifted to bottom row")
	}
}

func TestLineClearDoubleTripleQuad(t *testing.T) {
	cases := []struct {
		name      string
		rows      int
		wantScore int
	}{
		{"double", 2, 300},
		{"triple", 3, 500},
		{"quad", 4, 800},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGame(1)
			g.Score = 0
			// Fill the bottom `rows` rows fully except a 1-wide well at col 0.
			for r := 0; r < tc.rows; r++ {
				y := Height - 1 - r
				for x := 1; x < Width; x++ {
					g.SetFilled(x, y, true, Z)
				}
			}
			// Drop a vertical I piece into the col-0 well to complete them.
			// I vertical (rotation 1) occupies local x=2; origin X=-2 -> col 0.
			g.SetCurrent(Piece{Kind: I, Rotation: 1, X: -2, Y: Height - 4})
			// For double/triple we only need part of the I to land; but a
			// vertical I always fills 4 rows. To cleanly test 2/3 we instead
			// fill exactly `rows` rows and place I so its lowest `rows` cells
			// land in the wells. Simplest: fill 4 rows for quad, and for
			// 2/3 fill those rows and ensure I overlaps them.
			// Re-do board for clarity:
			g = NewGame(1)
			g.Score = 0
			for r := 0; r < tc.rows; r++ {
				y := Height - 1 - r
				for x := 1; x < Width; x++ {
					g.SetFilled(x, y, true, Z)
				}
			}
			// Place a vertical I so its bottom cell is at row Height-1 and it
			// fills col 0 across the bottom 4 rows; only the filled `rows`
			// rows will clear.
			g.SetCurrent(Piece{Kind: I, Rotation: 1, X: -2, Y: Height - 4})
			cleared := g.LockCurrent()
			if cleared != tc.rows {
				t.Fatalf("%s: expected %d cleared, got %d", tc.name, tc.rows, cleared)
			}
			if g.Score != tc.wantScore {
				t.Errorf("%s: expected score %d, got %d", tc.name, tc.wantScore, g.Score)
			}
		})
	}
}

func TestLineClearPartialNoClear(t *testing.T) {
	g := NewGame(1)
	g.Score = 0
	// Fill 9 of 10 cells in bottom row.
	for x := 0; x < Width-1; x++ {
		g.SetFilled(x, 19, true, Z)
	}
	// Place a piece elsewhere that does not complete row 19.
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 0, Y: 0})
	cleared := g.LockCurrent()
	if cleared != 0 {
		t.Errorf("partial row should not clear, got %d", cleared)
	}
	if g.Score != 0 {
		t.Errorf("no score for partial, got %d", g.Score)
	}
}

func TestLineClearGapPreserve(t *testing.T) {
	g := NewGame(1)
	// Upper row 17 has a gap at col 3 (not full).
	for x := 0; x < Width; x++ {
		if x != 3 {
			g.SetFilled(x, 17, true, L)
		}
	}
	// Middle row 18 is full and will be cleared.
	fillRow(g, 18)
	// Place a no-op piece far away (won't complete anything new in row 18
	// since it's already full -> it clears on lock).
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 0, Y: 0})
	cleared := g.LockCurrent()
	if cleared != 1 {
		t.Fatalf("expected 1 cleared (full middle row), got %d", cleared)
	}
	// Row 17's pattern should now have shifted down to row 18, gap preserved.
	if g.Filled[18][3] {
		t.Errorf("gap at col 3 not preserved after shift")
	}
	for x := 0; x < Width; x++ {
		if x == 3 {
			continue
		}
		if !g.Filled[18][x] {
			t.Errorf("expected filled at row 18 col %d after shift", x)
		}
	}
}

// --- soft drop ---

func TestSoftDropMovesOneRow(t *testing.T) {
	g := NewGame(1)
	g.Score = 0
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 5})
	y := g.Current.Y
	if !g.SoftDrop() {
		t.Fatalf("soft drop on free space should succeed")
	}
	if g.Current.Y != y+1 {
		t.Errorf("soft drop should move down exactly 1: %d -> %d", y, g.Current.Y)
	}
	if g.Score != SoftDropPoints {
		t.Errorf("soft drop should award %d, got %d", SoftDropPoints, g.Score)
	}
}

func TestSoftDropBlockedNoOp(t *testing.T) {
	g := NewGame(1)
	g.Score = 0
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 18}) // on floor
	if g.SoftDrop() {
		t.Errorf("soft drop into floor should be no-op")
	}
	if g.Score != 0 {
		t.Errorf("blocked soft drop should not score, got %d", g.Score)
	}
}

// --- hard drop ---

func TestHardDropLandsOnFloorEmptyBoard(t *testing.T) {
	g := NewGame(1)
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 0})
	// Predict landing: O bottom at row 19 => origin Y=18.
	g.HardDrop()
	// After hard drop the piece is locked; bottom two rows at cols 5,6 filled.
	if !g.Filled[18][5] || !g.Filled[18][6] || !g.Filled[19][5] || !g.Filled[19][6] {
		t.Errorf("hard drop did not land O on floor; board=%v", g.Filled[18])
	}
}

func TestHardDropPredictedRestingPosition(t *testing.T) {
	g := NewGame(1)
	// Stack a locked block so the piece rests on top of it.
	g.SetFilled(5, 19, true, Z)
	g.SetFilled(6, 19, true, Z)
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 0}) // cols 5,6
	g.HardDrop()
	// O should rest on top of row 19 block => bottom of O at row 18.
	if !g.Filled[17][5] || !g.Filled[17][6] || !g.Filled[18][5] || !g.Filled[18][6] {
		t.Errorf("hard drop resting position wrong")
	}
}

// --- lock on landing ---

func TestLockOnLandingSpawnsNew(t *testing.T) {
	g := NewGame(42)
	g.SetCurrent(Piece{Kind: O, Rotation: 0, X: 4, Y: 18}) // on floor
	before := g.Current
	g.Step() // cannot move down -> lock + spawn
	// The locked cells should be exactly the old O's cells.
	for _, c := range before.Cells() {
		if !g.Filled[c.Y][c.X] {
			t.Errorf("expected locked cell at %+v after landing", c)
		}
	}
	// A new piece should have spawned at the top.
	if g.Current.Y > 2 {
		t.Errorf("new piece did not spawn at top, Y=%d", g.Current.Y)
	}
}

// --- game over ---

func TestGameOverOnSpawnCollision(t *testing.T) {
	g := NewGame(7)
	// Fill the entire top region so any spawn collides.
	for y := 0; y < 3; y++ {
		for x := 0; x < Width; x++ {
			g.SetFilled(x, y, true, Z)
		}
	}
	// Force a spawn; it must collide and set GameOver.
	g.ForceSpawn()
	if g.State() != GameOver {
		t.Fatalf("expected GameOver after spawn into filled top, got %v", g.State())
	}
	// Gravity must not mutate a game-over board.
	snapshot := g.Filled
	g.Step()
	if g.Filled != snapshot {
		t.Errorf("Step mutated a GameOver board")
	}
}

// --- deterministic sequence ---

func TestDeterministicSequenceSameSeed(t *testing.T) {
	a := PieceSequence(12345, 30)
	b := PieceSequence(12345, 30)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("same seed produced different sequences:\n%v\n%v", a, b)
	}
}

func TestDeterministicSequenceDiffersWithDifferentSeed(t *testing.T) {
	a := PieceSequence(1, 30)
	b := PieceSequence(2, 30)
	if reflect.DeepEqual(a, b) {
		t.Errorf("different seeds produced identical sequences (very unlikely)")
	}
}

func TestGameSpawnMatchesSequence(t *testing.T) {
	// The Game's spawn order must match the standalone PieceSequence helper so
	// tests relying on either stay consistent.
	seq := PieceSequence(999, 8)
	g := NewGame(999)
	got := []PieceKind{g.Current.Kind}
	for i := 1; i < 8; i++ {
		// Force the current piece to land instantly via hard drop on an empty
		// column path; but to avoid board interactions just call spawn-like
		// logic via HardDrop which spawns next. Reset board each time.
		g.Filled = [Height][Width]bool{}
		g.Board = [Height][Width]int{}
		g.SetCurrent(Piece{Kind: g.Current.Kind, Rotation: 0, X: 4, Y: 18})
		g.HardDrop()
		got = append(got, g.Current.Kind)
	}
	if !reflect.DeepEqual(seq, got) {
		t.Errorf("game spawn order != PieceSequence:\nseq=%v\ngot=%v", seq, got)
	}
}

// --- step idle ---

func TestStepIdleOnGameOver(t *testing.T) {
	g := NewGame(1)
	g.state = GameOver
	snap := g.Current
	g.Step()
	if g.Current != snap {
		t.Errorf("Step mutated current piece on GameOver")
	}
}

func TestStepIdleOnPaused(t *testing.T) {
	g := NewGame(1)
	g.SetState(Paused)
	snap := g.Current
	g.Step()
	if g.Current != snap {
		t.Errorf("Step mutated current piece on Paused")
	}
}
