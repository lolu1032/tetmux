// Tetromino definitions. Each piece is described by its spawn matrix; the four
// rotation states are derived by rotating the matrix 90° clockwise. Colour
// indices 1..7 map to PIECE_COLORS below (0 = empty cell).

export type PieceType = 'I' | 'O' | 'T' | 'S' | 'Z' | 'J' | 'L'

export const PIECE_TYPES: PieceType[] = ['I', 'O', 'T', 'S', 'Z', 'J', 'L']

export const PIECE_COLORS: Record<PieceType, string> = {
  I: '#2dd4bf', // teal
  O: '#facc15', // yellow
  T: '#c084fc', // purple
  S: '#4ade80', // green
  Z: '#f87171', // red
  J: '#60a5fa', // blue
  L: '#fb923c', // orange
}

export const PIECE_INDEX: Record<PieceType, number> = {
  I: 1,
  O: 2,
  T: 3,
  S: 4,
  Z: 5,
  J: 6,
  L: 7,
}

export const COLOR_BY_INDEX: string[] = [
  '', // 0 empty
  PIECE_COLORS.I,
  PIECE_COLORS.O,
  PIECE_COLORS.T,
  PIECE_COLORS.S,
  PIECE_COLORS.Z,
  PIECE_COLORS.J,
  PIECE_COLORS.L,
]

type Matrix = number[][]

const SPAWN_MATRIX: Record<PieceType, Matrix> = {
  I: [
    [0, 0, 0, 0],
    [1, 1, 1, 1],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  O: [
    [1, 1],
    [1, 1],
  ],
  T: [
    [0, 1, 0],
    [1, 1, 1],
    [0, 0, 0],
  ],
  S: [
    [0, 1, 1],
    [1, 1, 0],
    [0, 0, 0],
  ],
  Z: [
    [1, 1, 0],
    [0, 1, 1],
    [0, 0, 0],
  ],
  J: [
    [1, 0, 0],
    [1, 1, 1],
    [0, 0, 0],
  ],
  L: [
    [0, 0, 1],
    [1, 1, 1],
    [0, 0, 0],
  ],
}

export interface Cell {
  x: number
  y: number
}

function rotateCW(matrix: Matrix): Matrix {
  const n = matrix.length
  const out: Matrix = Array.from({ length: n }, () => new Array(n).fill(0))
  for (let r = 0; r < n; r++) {
    for (let c = 0; c < n; c++) {
      out[c][n - 1 - r] = matrix[r][c]
    }
  }
  return out
}

function cellsOf(matrix: Matrix): Cell[] {
  const cells: Cell[] = []
  for (let r = 0; r < matrix.length; r++) {
    for (let c = 0; c < matrix[r].length; c++) {
      if (matrix[r][c]) cells.push({ x: c, y: r })
    }
  }
  return cells
}

// Precomputed rotation states: ROTATIONS[type][rotation] -> filled cells.
export const ROTATIONS: Record<PieceType, Cell[][]> = Object.fromEntries(
  PIECE_TYPES.map((type) => {
    const states: Cell[][] = []
    let m = SPAWN_MATRIX[type]
    for (let i = 0; i < 4; i++) {
      states.push(cellsOf(m))
      m = rotateCW(m)
    }
    return [type, states]
  }),
) as Record<PieceType, Cell[][]>

// Matrix size per piece, used to centre the spawn horizontally.
export const MATRIX_SIZE: Record<PieceType, number> = Object.fromEntries(
  PIECE_TYPES.map((type) => [type, SPAWN_MATRIX[type].length]),
) as Record<PieceType, number>
