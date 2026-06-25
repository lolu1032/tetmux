import {
  BOARD_WIDTH,
  HIDDEN_ROWS,
  VISIBLE_HEIGHT,
  type GameSnapshot,
} from './engine'
import { COLOR_BY_INDEX, PIECE_COLORS, ROTATIONS, type PieceType } from './pieces'

const BG = '#0b0e14'
const GRID = '#161b26'
const GRID_LINE = 'rgba(255,255,255,0.04)'

interface BoardMetrics {
  cell: number
  offsetX: number
  offsetY: number
  width: number
  height: number
}

function metrics(canvasW: number, canvasH: number): BoardMetrics {
  const cell = Math.max(4, Math.floor(Math.min(canvasW / BOARD_WIDTH, canvasH / VISIBLE_HEIGHT)))
  const width = cell * BOARD_WIDTH
  const height = cell * VISIBLE_HEIGHT
  return {
    cell,
    width,
    height,
    offsetX: Math.floor((canvasW - width) / 2),
    offsetY: Math.floor((canvasH - height) / 2),
  }
}

function block(
  ctx: CanvasRenderingContext2D,
  px: number,
  py: number,
  size: number,
  color: string,
  opts: { ghost?: boolean } = {},
): void {
  const inset = Math.max(1, Math.floor(size * 0.06))
  const x = px + inset
  const y = py + inset
  const s = size - inset * 2
  if (opts.ghost) {
    ctx.strokeStyle = color
    ctx.globalAlpha = 0.35
    ctx.lineWidth = Math.max(1, Math.floor(size * 0.08))
    ctx.strokeRect(x + 0.5, y + 0.5, s - 1, s - 1)
    ctx.globalAlpha = 1
    return
  }
  ctx.fillStyle = color
  ctx.fillRect(x, y, s, s)
  // subtle top highlight for a little depth
  ctx.fillStyle = 'rgba(255,255,255,0.18)'
  ctx.fillRect(x, y, s, Math.max(1, Math.floor(s * 0.18)))
}

/** Render the playfield (locked stack + ghost + active piece). */
export function drawPlayfield(
  ctx: CanvasRenderingContext2D,
  canvasW: number,
  canvasH: number,
  snap: GameSnapshot,
): void {
  ctx.fillStyle = BG
  ctx.fillRect(0, 0, canvasW, canvasH)

  const m = metrics(canvasW, canvasH)

  // playfield backdrop + grid
  ctx.fillStyle = GRID
  ctx.fillRect(m.offsetX, m.offsetY, m.width, m.height)
  ctx.strokeStyle = GRID_LINE
  ctx.lineWidth = 1
  for (let x = 0; x <= BOARD_WIDTH; x++) {
    const gx = m.offsetX + x * m.cell + 0.5
    ctx.beginPath()
    ctx.moveTo(gx, m.offsetY)
    ctx.lineTo(gx, m.offsetY + m.height)
    ctx.stroke()
  }
  for (let y = 0; y <= VISIBLE_HEIGHT; y++) {
    const gy = m.offsetY + y * m.cell + 0.5
    ctx.beginPath()
    ctx.moveTo(m.offsetX, gy)
    ctx.lineTo(m.offsetX + m.width, gy)
    ctx.stroke()
  }

  const px = (x: number) => m.offsetX + x * m.cell
  const py = (vy: number) => m.offsetY + vy * m.cell

  // locked stack
  for (let y = HIDDEN_ROWS; y < snap.board.length; y++) {
    const vy = y - HIDDEN_ROWS
    for (let x = 0; x < BOARD_WIDTH; x++) {
      const v = snap.board[y][x]
      if (v) block(ctx, px(x), py(vy), m.cell, COLOR_BY_INDEX[v])
    }
  }

  // ghost
  for (const c of snap.ghost) {
    const vy = c.y - HIDDEN_ROWS
    if (vy >= 0) block(ctx, px(c.x), py(vy), m.cell, PIECE_COLORS[snap.active!.type], { ghost: true })
  }

  // active piece
  if (snap.active) {
    const color = PIECE_COLORS[snap.active.type]
    for (const c of snap.active.cells) {
      const vy = c.y - HIDDEN_ROWS
      if (vy >= 0) block(ctx, px(c.x), py(vy), m.cell, color)
    }
  }
}

/** Render a single tetromino centred in a small canvas (hold / next previews). */
export function drawMini(
  ctx: CanvasRenderingContext2D,
  canvasW: number,
  canvasH: number,
  type: PieceType | null,
): void {
  ctx.clearRect(0, 0, canvasW, canvasH)
  if (!type) return
  const cells = ROTATIONS[type][0]
  const xs = cells.map((c) => c.x)
  const ys = cells.map((c) => c.y)
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)
  const cols = maxX - minX + 1
  const rows = maxY - minY + 1
  const cell = Math.max(4, Math.floor(Math.min((canvasW * 0.8) / cols, (canvasH * 0.8) / rows)))
  const offsetX = Math.floor((canvasW - cols * cell) / 2)
  const offsetY = Math.floor((canvasH - rows * cell) / 2)
  const color = PIECE_COLORS[type]
  for (const c of cells) {
    block(ctx, offsetX + (c.x - minX) * cell, offsetY + (c.y - minY) * cell, cell, color)
  }
}
