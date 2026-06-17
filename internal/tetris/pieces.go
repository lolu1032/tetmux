// Package tetris implements pure, TUI-free Tetris game logic on a fixed
// 10x20 board. Everything in this package is deterministic given a seeded
// RNG so that tests are fully reproducible.
package tetris

// Board dimensions. Fixed per spec.
const (
	Width  = 10
	Height = 20
)

// PieceKind identifies one of the seven standard tetrominoes.
type PieceKind int

const (
	I PieceKind = iota
	O
	T
	S
	Z
	J
	L
)

// AllKinds is the canonical ordering of the seven tetrominoes. Index in this
// slice maps directly to the PieceKind iota value.
var AllKinds = []PieceKind{I, O, T, S, Z, J, L}

// Cell is a board coordinate. X grows to the right (0..Width-1), Y grows
// downward (0..Height-1) which matches the way terminals draw rows.
type Cell struct {
	X int
	Y int
}

// spawnShapes holds, for every piece kind, the four rotation states as a list
// of occupied cells relative to the piece's local origin. The origin is the
// top-left of a 4x4 (I) or 3x3 (others) bounding box. Rotation states are
// ordered 0,1,2,3 = CW quarter turns.
//
// These are hand-authored so that:
//   - each rotation state has exactly 4 cells,
//   - the O piece is identical in all four states (rotation-invariant),
//   - rotating four times returns to state 0.
var spawnShapes = map[PieceKind][4][]Cell{
	// I piece in a 4x4 box.
	I: {
		{{0, 1}, {1, 1}, {2, 1}, {3, 1}}, // horizontal
		{{2, 0}, {2, 1}, {2, 2}, {2, 3}}, // vertical
		{{0, 2}, {1, 2}, {2, 2}, {3, 2}}, // horizontal (lower)
		{{1, 0}, {1, 1}, {1, 2}, {1, 3}}, // vertical (left)
	},
	// O piece in a 3x3 box; all four states identical -> rotation invariant.
	O: {
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {2, 1}},
	},
	// T piece in a 3x3 box.
	T: {
		{{1, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {1, 2}},
		{{1, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	// S piece.
	S: {
		{{1, 0}, {2, 0}, {0, 1}, {1, 1}},
		{{1, 0}, {1, 1}, {2, 1}, {2, 2}},
		{{1, 1}, {2, 1}, {0, 2}, {1, 2}},
		{{0, 0}, {0, 1}, {1, 1}, {1, 2}},
	},
	// Z piece.
	Z: {
		{{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		{{2, 0}, {1, 1}, {2, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {1, 2}, {2, 2}},
		{{1, 0}, {0, 1}, {1, 1}, {0, 2}},
	},
	// J piece.
	J: {
		{{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {2, 0}, {1, 1}, {1, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {2, 2}},
		{{1, 0}, {1, 1}, {0, 2}, {1, 2}},
	},
	// L piece.
	L: {
		{{2, 0}, {0, 1}, {1, 1}, {2, 1}},
		{{1, 0}, {1, 1}, {1, 2}, {2, 2}},
		{{0, 1}, {1, 1}, {2, 1}, {0, 2}},
		{{0, 0}, {1, 0}, {1, 1}, {1, 2}},
	},
}

// Piece is a live, falling tetromino. X/Y is the position of the piece's local
// origin on the board; Rotation is the current rotation state index (0..3).
type Piece struct {
	Kind     PieceKind
	Rotation int
	X        int
	Y        int
}

// shapeCells returns the four occupied cells of the given kind/rotation in the
// piece's local box coordinates (not yet translated onto the board).
func shapeCells(kind PieceKind, rotation int) []Cell {
	states := spawnShapes[kind]
	r := ((rotation % 4) + 4) % 4
	src := states[r]
	out := make([]Cell, len(src))
	copy(out, src)
	return out
}

// Cells returns the absolute board cells occupied by the piece at its current
// position and rotation.
func (p Piece) Cells() []Cell {
	local := shapeCells(p.Kind, p.Rotation)
	out := make([]Cell, len(local))
	for i, c := range local {
		out[i] = Cell{X: c.X + p.X, Y: c.Y + p.Y}
	}
	return out
}

// CellsAt returns the absolute board cells the piece would occupy if it were
// at the supplied position and rotation. Used for collision tests without
// mutating the piece.
func (p Piece) CellsAt(x, y, rotation int) []Cell {
	local := shapeCells(p.Kind, rotation)
	out := make([]Cell, len(local))
	for i, c := range local {
		out[i] = Cell{X: c.X + x, Y: c.Y + y}
	}
	return out
}

// Color returns a stable identifier for a piece kind, used by the renderer to
// pick a color. It is part of the pure package so rendering stays testable.
func (k PieceKind) Rune() rune {
	switch k {
	case I:
		return 'I'
	case O:
		return 'O'
	case T:
		return 'T'
	case S:
		return 'S'
	case Z:
		return 'Z'
	case J:
		return 'J'
	case L:
		return 'L'
	}
	return '?'
}
