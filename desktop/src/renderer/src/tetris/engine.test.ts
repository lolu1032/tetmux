import { describe, expect, it } from 'vitest'
import {
  BOARD_WIDTH,
  NEXT_COUNT,
  TOTAL_HEIGHT,
  TetrisEngine,
} from './engine'
import { PIECE_TYPES } from './pieces'

// Deterministic PRNG so tests are reproducible.
function mulberry32(seed: number): () => number {
  let s = seed
  return () => {
    s |= 0
    s = (s + 0x6d2b79f5) | 0
    let t = Math.imul(s ^ (s >>> 15), 1 | s)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

describe('TetrisEngine', () => {
  it('starts playing with an active piece and a filled next queue', () => {
    const e = new TetrisEngine({ rng: mulberry32(1) })
    e.start()
    const s = e.snapshot()
    expect(s.status).toBe('playing')
    expect(s.active).not.toBeNull()
    expect(s.nextQueue).toHaveLength(NEXT_COUNT)
  })

  it('is deterministic for a fixed seed', () => {
    const seq = (seed: number): string[] => {
      const e = new TetrisEngine({ rng: mulberry32(seed) })
      e.start()
      const types: string[] = []
      for (let i = 0; i < 14; i++) {
        types.push(e.snapshot().active!.type)
        e.hardDrop()
      }
      return types
    }
    expect(seq(42)).toEqual(seq(42))
  })

  it('emits each of the 7 pieces once per bag', () => {
    const e = new TetrisEngine({ rng: mulberry32(7) })
    e.start()
    const first7 = new Set<string>()
    for (let i = 0; i < 7; i++) {
      first7.add(e.snapshot().active!.type)
      e.hardDrop()
    }
    expect(first7.size).toBe(7)
    PIECE_TYPES.forEach((t) => expect(first7.has(t)).toBe(true))
  })

  it('awards points and advances to a new piece on hard drop', () => {
    const e = new TetrisEngine({ rng: mulberry32(3) })
    e.start()
    expect(e.snapshot().score).toBe(0)
    e.hardDrop()
    expect(e.snapshot().score).toBeGreaterThan(0)
  })

  it('clears a full row, scores it and shifts the stack down', () => {
    const e = new TetrisEngine({ rng: mulberry32(9) })
    e.start()
    const internals = e as unknown as { board: number[][]; clearLines(): void }
    internals.board[TOTAL_HEIGHT - 1].fill(1)
    internals.clearLines()
    const s = e.snapshot()
    expect(s.lines).toBe(1)
    expect(s.score).toBe(100) // single line at level 0 => 100 * (0 + 1)
    expect(s.board[TOTAL_HEIGHT - 1].every((v) => v === 0)).toBe(true)
  })

  it('keeps the ghost at or below the active piece and inside the board', () => {
    const e = new TetrisEngine({ rng: mulberry32(5) })
    e.start()
    const { active, ghost } = e.snapshot()
    expect(ghost.length).toBe(active!.cells.length)
    const activeMaxY = Math.max(...active!.cells.map((c) => c.y))
    const ghostMaxY = Math.max(...ghost.map((c) => c.y))
    expect(ghostMaxY).toBeGreaterThanOrEqual(activeMaxY)
    for (const c of ghost) {
      expect(c.x).toBeGreaterThanOrEqual(0)
      expect(c.x).toBeLessThan(BOARD_WIDTH)
      expect(c.y).toBeLessThan(TOTAL_HEIGHT)
    }
  })

  it('pauses, ignores ticks while paused, then resumes', () => {
    const e = new TetrisEngine({ rng: mulberry32(1) })
    e.start()
    e.togglePause()
    expect(e.snapshot().status).toBe('paused')
    const before = JSON.stringify(e.snapshot().active!.cells)
    e.tick(10_000) // a paused engine must not advance
    expect(JSON.stringify(e.snapshot().active!.cells)).toBe(before)
    e.togglePause()
    expect(e.snapshot().status).toBe('playing')
  })

  // DEFECT 3 — infinite-spin lock stall: bounded lock-delay resets.
  it('eventually locks a resting piece despite continuous rotation', () => {
    const e = new TetrisEngine({ rng: mulberry32(1) }) // seed 1 spawns an O piece
    e.start()
    let locked = false
    for (let i = 0; i < 600; i++) {
      e.rotate(1)
      e.tick(50)
      if (e.snapshot().board.some((r) => r.some((v) => v !== 0))) {
        locked = true
        break
      }
    }
    // Before the fix rotate() zeroes lockAcc every iteration, so the piece never
    // locks and the board stays empty after 600 iterations.
    expect(locked).toBe(true)
  })

  it('does not lock a resting piece prematurely when rotated only a few times', () => {
    const e = new TetrisEngine({ rng: mulberry32(1) }) // O piece
    e.start()
    // Let it fall to the floor without rotating (stop the moment it rests).
    const onFloor = (): boolean =>
      e.snapshot().active!.cells.some((c) => c.y === TOTAL_HEIGHT - 1)
    for (let i = 0; i < 600 && !onFloor(); i++) e.tick(50)
    expect(onFloor()).toBe(true)
    // A handful of resets (well under the cap) must keep the piece alive.
    for (let i = 0; i < 5; i++) {
      e.rotate(1)
      e.tick(50)
    }
    expect(e.snapshot().board.every((r) => r.every((v) => v === 0))).toBe(true)
  })
})
