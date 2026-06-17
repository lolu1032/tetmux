package tetris

import "math/rand"

// State is the high-level lifecycle of a game.
type State int

const (
	// Playing means a piece is actively falling and gravity ticks should run.
	Playing State = iota
	// Paused means the game is frozen; gravity does nothing.
	Paused
	// GameOver means a spawn collided; no further mutation occurs.
	GameOver
)

// scoreTable maps the number of simultaneously-cleared lines to the awarded
// score, per the spec (1/2/3/4 => 100/300/500/800).
var scoreTable = map[int]int{
	1: 100,
	2: 300,
	3: 500,
	4: 800,
}

// SoftDropPoints is awarded per cell of a successful soft drop.
const SoftDropPoints = 1

// Game holds the full, pure game state. The board is row-major: board[y][x].
// A non-nil pointer in a cell means it is locked/filled; we store the kind so
// the renderer can color it.
type Game struct {
	Board   [Height][Width]int // 0 = empty, otherwise (kind+1)
	Filled  [Height][Width]bool
	Current Piece
	Score   int
	Lines   int
	state   State

	rng *rand.Rand
	// bag is a simple sequence source. We use a fresh shuffled 7-bag so the
	// sequence is deterministic for a given seed and statistically fair.
	bag    []PieceKind
	bagIdx int
}

// NewGame creates a game seeded with the given seed. The first piece is
// spawned immediately and the game starts in the Playing state (unless the
// board were somehow already blocked, which cannot happen on an empty board).
func NewGame(seed int64) *Game {
	g := &Game{
		rng:   rand.New(rand.NewSource(seed)),
		state: Playing,
	}
	g.spawn()
	return g
}

// State returns the current lifecycle state.
func (g *Game) State() State { return g.state }

// SetState lets the glue layer pause/unpause. Switching to Playing from a
// non-GameOver state is allowed; GameOver is terminal.
func (g *Game) SetState(s State) {
	if g.state == GameOver {
		return
	}
	g.state = s
}

// nextKind draws the next piece kind from a deterministic shuffled 7-bag.
func (g *Game) nextKind() PieceKind {
	if g.bagIdx >= len(g.bag) {
		// Refill and shuffle a new bag of all seven kinds.
		g.bag = append([]PieceKind(nil), AllKinds...)
		g.rng.Shuffle(len(g.bag), func(i, j int) {
			g.bag[i], g.bag[j] = g.bag[j], g.bag[i]
		})
		g.bagIdx = 0
	}
	k := g.bag[g.bagIdx]
	g.bagIdx++
	return k
}

// spawnOriginX is the local-origin X at which every piece spawns. The shape
// tables in pieces.go already center each tetromino inside its 3x3/4x4 box, so
// a single origin keeps all kinds roughly centered on the 10-wide board.
const spawnOriginX = 3

// spawn places a new current piece at the top. If it immediately collides
// with locked cells, the game transitions to GameOver.
func (g *Game) spawn() {
	kind := g.nextKind()
	p := Piece{Kind: kind, Rotation: 0, X: spawnOriginX, Y: 0}
	g.Current = p
	if g.collides(p.Cells()) {
		g.state = GameOver
	}
}

// collides reports whether any of the supplied cells is out of bounds or
// overlaps a locked cell.
func (g *Game) collides(cells []Cell) bool {
	for _, c := range cells {
		if c.X < 0 || c.X >= Width || c.Y < 0 || c.Y >= Height {
			return true
		}
		if g.Filled[c.Y][c.X] {
			return true
		}
	}
	return false
}

// Collides is the exported wrapper for tests that want to probe collision.
func (g *Game) Collides(cells []Cell) bool { return g.collides(cells) }

// CanPlace reports whether the current piece could legally sit at the given
// position/rotation.
func (g *Game) CanPlace(x, y, rotation int) bool {
	return !g.collides(g.Current.CellsAt(x, y, rotation))
}

// MoveLeft shifts the current piece left one column if legal. Returns true if
// it moved.
func (g *Game) MoveLeft() bool {
	if g.state != Playing {
		return false
	}
	if g.CanPlace(g.Current.X-1, g.Current.Y, g.Current.Rotation) {
		g.Current.X--
		return true
	}
	return false
}

// MoveRight shifts the current piece right one column if legal.
func (g *Game) MoveRight() bool {
	if g.state != Playing {
		return false
	}
	if g.CanPlace(g.Current.X+1, g.Current.Y, g.Current.Rotation) {
		g.Current.X++
		return true
	}
	return false
}

// kickOffsets are the horizontal nudges tried when a rotation is blocked,
// implementing a simple wall-kick / clamp (SRS-lite).
var kickOffsets = []int{0, -1, 1, -2, 2}

// Rotate attempts to rotate the current piece clockwise (dir=+1) or
// counter-clockwise (dir=-1), trying small horizontal kicks so a rotation
// near a wall succeeds instead of being lost. Returns true if it rotated.
func (g *Game) Rotate(dir int) bool {
	if g.state != Playing {
		return false
	}
	newRot := ((g.Current.Rotation+dir)%4 + 4) % 4
	for _, dx := range kickOffsets {
		if g.CanPlace(g.Current.X+dx, g.Current.Y, newRot) {
			g.Current.X += dx
			g.Current.Rotation = newRot
			return true
		}
	}
	return false
}

// RotateCW rotates clockwise.
func (g *Game) RotateCW() bool { return g.Rotate(1) }

// RotateCCW rotates counter-clockwise.
func (g *Game) RotateCCW() bool { return g.Rotate(-1) }

// SoftDrop moves the piece down one row if free, awarding soft-drop points.
// Returns true if it moved. If blocked, it is a no-op (does NOT lock here; the
// caller / gravity decides when to lock).
func (g *Game) SoftDrop() bool {
	if g.state != Playing {
		return false
	}
	if g.CanPlace(g.Current.X, g.Current.Y+1, g.Current.Rotation) {
		g.Current.Y++
		g.Score += SoftDropPoints
		return true
	}
	return false
}

// HardDrop drops the piece to the lowest legal row, locks it immediately, and
// spawns the next piece. Returns the number of rows fallen.
func (g *Game) HardDrop() int {
	if g.state != Playing {
		return 0
	}
	dropped := 0
	for g.CanPlace(g.Current.X, g.Current.Y+1, g.Current.Rotation) {
		g.Current.Y++
		dropped++
	}
	g.lockAndSpawn()
	return dropped
}

// Step performs one gravity tick: move down if possible, otherwise lock the
// piece and spawn a new one. On a GameOver or Paused board this is a no-op.
func (g *Game) Step() {
	if g.state != Playing {
		return
	}
	if g.CanPlace(g.Current.X, g.Current.Y+1, g.Current.Rotation) {
		g.Current.Y++
		return
	}
	g.lockAndSpawn()
}

// lockAndSpawn writes the current piece into the board, clears any full lines,
// updates score, and spawns the next piece.
func (g *Game) lockAndSpawn() {
	g.lock()
	cleared := g.clearLines()
	if cleared > 0 {
		g.Lines += cleared
		g.Score += scoreTable[cleared]
	}
	g.spawn()
}

// lock writes the current piece's cells into the locked board.
func (g *Game) lock() {
	val := int(g.Current.Kind) + 1
	for _, c := range g.Current.Cells() {
		if c.Y >= 0 && c.Y < Height && c.X >= 0 && c.X < Width {
			g.Filled[c.Y][c.X] = true
			g.Board[c.Y][c.X] = val
		}
	}
}

// clearLines removes every fully-filled row, shifting the rows above down by
// the number cleared, and returns how many rows were cleared.
func (g *Game) clearLines() int {
	cleared := 0
	// Walk from the bottom up. We rebuild the board into a fresh one keeping
	// only non-full rows, stacked at the bottom, preserving gaps exactly.
	var newFilled [Height][Width]bool
	var newBoard [Height][Width]int
	dst := Height - 1
	for src := Height - 1; src >= 0; src-- {
		if rowFull(g.Filled[src]) {
			cleared++
			continue
		}
		newFilled[dst] = g.Filled[src]
		newBoard[dst] = g.Board[src]
		dst--
	}
	g.Filled = newFilled
	g.Board = newBoard
	return cleared
}

// rowFull reports whether every column in a row is filled.
func rowFull(row [Width]bool) bool {
	for x := 0; x < Width; x++ {
		if !row[x] {
			return false
		}
	}
	return true
}

// LockCurrent is a test/helper hook: lock the current piece and clear lines
// WITHOUT spawning a new piece, so tests can assert resulting board state and
// score deterministically.
func (g *Game) LockCurrent() int {
	g.lock()
	cleared := g.clearLines()
	if cleared > 0 {
		g.Lines += cleared
		g.Score += scoreTable[cleared]
	}
	return cleared
}

// SetFilled is a test helper to set up board scenarios deterministically.
func (g *Game) SetFilled(x, y int, filled bool, kind PieceKind) {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return
	}
	g.Filled[y][x] = filled
	if filled {
		g.Board[y][x] = int(kind) + 1
	} else {
		g.Board[y][x] = 0
	}
}

// SetCurrent is a test helper to position the current piece precisely.
func (g *Game) SetCurrent(p Piece) { g.Current = p }

// ForceSpawn is a test helper to invoke spawn (drawing from the bag) directly.
func (g *Game) ForceSpawn() { g.spawn() }
