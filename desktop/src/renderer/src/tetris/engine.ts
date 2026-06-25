import {
  MATRIX_SIZE,
  PIECE_INDEX,
  PIECE_TYPES,
  ROTATIONS,
  type Cell,
  type PieceType,
} from './pieces'

export const BOARD_WIDTH = 10
export const VISIBLE_HEIGHT = 20
export const HIDDEN_ROWS = 2
export const TOTAL_HEIGHT = VISIBLE_HEIGHT + HIDDEN_ROWS
export const NEXT_COUNT = 5

const LINE_SCORES = [0, 100, 300, 500, 800]
const LOCK_DELAY_MS = 500
const SOFT_DROP_MS = 30
// Gravity (ms per row) indexed by level; clamps at the last entry.
const GRAVITY_MS = [800, 720, 630, 550, 470, 380, 300, 220, 130, 100, 80, 70, 60, 50, 40, 30]

// Wall-kick offsets tried in order when a rotation is blocked. Not full SRS,
// but enough to make rotations against walls and floors feel right.
const KICKS: Cell[] = [
  { x: 0, y: 0 },
  { x: -1, y: 0 },
  { x: 1, y: 0 },
  { x: 0, y: -1 },
  { x: -2, y: 0 },
  { x: 2, y: 0 },
  { x: 0, y: -2 },
]

export type GameStatus = 'ready' | 'playing' | 'paused' | 'gameover'

export interface ActivePiece {
  type: PieceType
  rotation: number
  x: number
  y: number
}

export interface GameSnapshot {
  board: readonly (readonly number[])[]
  active: { type: PieceType; cells: Cell[] } | null
  ghost: Cell[]
  nextQueue: PieceType[]
  hold: PieceType | null
  canHold: boolean
  score: number
  lines: number
  level: number
  status: GameStatus
}

type Rng = () => number

function emptyBoard(): number[][] {
  return Array.from({ length: TOTAL_HEIGHT }, () => new Array<number>(BOARD_WIDTH).fill(0))
}

export class TetrisEngine {
  private board = emptyBoard()
  private piece: ActivePiece | null = null
  private queue: PieceType[] = []
  private holdType: PieceType | null = null
  private canHold = true
  private status: GameStatus = 'ready'

  private score = 0
  private lines = 0
  private level = 0

  private gravityAcc = 0
  private lockAcc = 0
  private softDropping = false

  private readonly rng: Rng

  constructor(opts: { rng?: Rng } = {}) {
    this.rng = opts.rng ?? Math.random
  }

  // ---- lifecycle -----------------------------------------------------------

  start(): void {
    this.board = emptyBoard()
    this.queue = []
    this.holdType = null
    this.canHold = true
    this.score = 0
    this.lines = 0
    this.level = 0
    this.gravityAcc = 0
    this.lockAcc = 0
    this.softDropping = false
    this.status = 'playing'
    this.spawn()
  }

  togglePause(): void {
    if (this.status === 'playing') this.status = 'paused'
    else if (this.status === 'paused') this.status = 'playing'
  }

  get isOver(): boolean {
    return this.status === 'gameover'
  }

  // ---- bag / spawning ------------------------------------------------------

  private fillBag(): void {
    const pieces = [...PIECE_TYPES]
    for (let i = pieces.length - 1; i > 0; i--) {
      const j = Math.floor(this.rng() * (i + 1))
      ;[pieces[i], pieces[j]] = [pieces[j], pieces[i]]
    }
    this.queue.push(...pieces)
  }

  private pull(): PieceType {
    if (this.queue.length <= NEXT_COUNT) this.fillBag()
    return this.queue.shift() as PieceType
  }

  private spawn(type?: PieceType): void {
    const next = type ?? this.pull()
    const size = MATRIX_SIZE[next]
    const piece: ActivePiece = {
      type: next,
      rotation: 0,
      x: Math.floor((BOARD_WIDTH - size) / 2),
      y: 0,
    }
    this.gravityAcc = 0
    this.lockAcc = 0
    this.canHold = true
    if (!this.fits(piece)) {
      // Block out: the new piece overlaps the stack — game over.
      this.piece = piece
      this.status = 'gameover'
      return
    }
    this.piece = piece
  }

  // ---- collision -----------------------------------------------------------

  private cellsAt(piece: ActivePiece): Cell[] {
    return ROTATIONS[piece.type][piece.rotation].map((c) => ({
      x: piece.x + c.x,
      y: piece.y + c.y,
    }))
  }

  private fits(piece: ActivePiece): boolean {
    for (const cell of this.cellsAt(piece)) {
      if (cell.x < 0 || cell.x >= BOARD_WIDTH || cell.y >= TOTAL_HEIGHT) return false
      if (cell.y >= 0 && this.board[cell.y][cell.x]) return false
    }
    return true
  }

  // ---- input ---------------------------------------------------------------

  moveLeft(): void {
    this.shift(-1)
  }

  moveRight(): void {
    this.shift(1)
  }

  private shift(dx: number): void {
    if (this.status !== 'playing' || !this.piece) return
    const moved = { ...this.piece, x: this.piece.x + dx }
    if (this.fits(moved)) {
      this.piece = moved
      this.lockAcc = 0
    }
  }

  rotate(dir: 1 | -1): void {
    if (this.status !== 'playing' || !this.piece) return
    const rotation = (this.piece.rotation + (dir === 1 ? 1 : 3)) % 4
    for (const kick of KICKS) {
      const candidate: ActivePiece = {
        ...this.piece,
        rotation,
        x: this.piece.x + kick.x,
        y: this.piece.y + kick.y,
      }
      if (this.fits(candidate)) {
        this.piece = candidate
        this.lockAcc = 0
        return
      }
    }
  }

  setSoftDrop(active: boolean): void {
    this.softDropping = active
  }

  hardDrop(): void {
    if (this.status !== 'playing' || !this.piece) return
    let dropped = 0
    while (this.fits({ ...this.piece, y: this.piece.y + 1 })) {
      this.piece = { ...this.piece, y: this.piece.y + 1 }
      dropped++
    }
    this.score += dropped * 2
    this.lockPiece()
  }

  hold(): void {
    if (this.status !== 'playing' || !this.piece || !this.canHold) return
    const current = this.piece.type
    if (this.holdType === null) {
      this.holdType = current
      this.spawn()
    } else {
      const swap = this.holdType
      this.holdType = current
      this.spawn(swap)
    }
    this.canHold = false
  }

  // ---- gravity / locking ---------------------------------------------------

  private gravityInterval(): number {
    return GRAVITY_MS[Math.min(this.level, GRAVITY_MS.length - 1)]
  }

  tick(dtMs: number): void {
    if (this.status !== 'playing' || !this.piece) return
    const base = this.gravityInterval()
    const interval = this.softDropping ? Math.min(SOFT_DROP_MS, base) : base

    this.gravityAcc += dtMs
    while (this.gravityAcc >= interval) {
      this.gravityAcc -= interval
      if (this.fits({ ...this.piece, y: this.piece.y + 1 })) {
        this.piece = { ...this.piece, y: this.piece.y + 1 }
        if (this.softDropping) this.score += 1
        this.lockAcc = 0
      } else {
        break
      }
    }

    const resting = !this.fits({ ...this.piece, y: this.piece.y + 1 })
    if (resting) {
      this.lockAcc += dtMs
      if (this.lockAcc >= LOCK_DELAY_MS) this.lockPiece()
    } else {
      this.lockAcc = 0
    }
  }

  private lockPiece(): void {
    if (!this.piece) return
    const index = PIECE_INDEX[this.piece.type]
    for (const cell of this.cellsAt(this.piece)) {
      if (cell.y >= 0 && cell.y < TOTAL_HEIGHT && cell.x >= 0 && cell.x < BOARD_WIDTH) {
        this.board[cell.y][cell.x] = index
      }
    }
    this.clearLines()
    this.spawn()
  }

  private clearLines(): void {
    let cleared = 0
    for (let y = TOTAL_HEIGHT - 1; y >= 0; y--) {
      if (this.board[y].every((v) => v !== 0)) {
        this.board.splice(y, 1)
        this.board.unshift(new Array<number>(BOARD_WIDTH).fill(0))
        cleared++
        y++ // re-check the row that shifted down into this slot
      }
    }
    if (cleared > 0) {
      this.score += LINE_SCORES[cleared] * (this.level + 1)
      this.lines += cleared
      this.level = Math.floor(this.lines / 10)
    }
  }

  // ---- rendering snapshot --------------------------------------------------

  private ghostCells(): Cell[] {
    if (!this.piece || this.status === 'gameover') return []
    let ghost = { ...this.piece }
    while (this.fits({ ...ghost, y: ghost.y + 1 })) {
      ghost = { ...ghost, y: ghost.y + 1 }
    }
    return this.cellsAt(ghost)
  }

  snapshot(): GameSnapshot {
    return {
      board: this.board,
      active: this.piece ? { type: this.piece.type, cells: this.cellsAt(this.piece) } : null,
      ghost: this.ghostCells(),
      nextQueue: this.queue.slice(0, NEXT_COUNT),
      hold: this.holdType,
      canHold: this.canHold,
      score: this.score,
      lines: this.lines,
      level: this.level,
      status: this.status,
    }
  }
}
