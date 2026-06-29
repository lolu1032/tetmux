import { beforeAll, describe, expect, it } from 'vitest'
import { TetrisGame } from './game'
import type { GameSnapshot } from './engine'

// happy-dom does not implement <canvas> or ResizeObserver; stub just enough that
// TetrisGame can construct and render without a real canvas backend.
beforeAll(() => {
  if (!('ResizeObserver' in globalThis)) {
    ;(globalThis as any).ResizeObserver = class {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    }
  }
  const ls = (globalThis as any).localStorage
  if (!ls || typeof ls.getItem !== 'function') {
    const store = new Map<string, string>()
    ;(globalThis as any).localStorage = {
      getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
      setItem: (k: string, v: string) => void store.set(k, String(v)),
      removeItem: (k: string) => void store.delete(k),
      clear: () => store.clear(),
      key: () => null,
      length: 0,
    }
  }
  ;(HTMLCanvasElement.prototype as any).getContext = () => null
})

const key = (k: string): KeyboardEvent => new KeyboardEvent('keydown', { key: k })

/** Build a game and a getter for its latest snapshot (refreshed via render). */
function makeGame(): { game: TetrisGame; status: () => GameSnapshot['status'] } {
  let snap: GameSnapshot | null = null
  const game = new TetrisGame({ onSnapshot: (s) => (snap = s) })
  return {
    game,
    status: () => {
      game.resize() // forces a render(), which fires onSnapshot
      return snap!.status
    },
  }
}

describe('TetrisGame focus-driven pause', () => {
  it('auto-pauses when focus leaves and auto-resumes when it returns', () => {
    const { game, status } = makeGame()
    game.handleKeyDown(key('Enter')) // ready -> playing
    expect(status()).toBe('playing')

    game.setActive(false) // focus left the Tetris pane
    expect(status()).toBe('paused')

    game.setActive(true) // focus returned
    expect(status()).toBe('playing')
  })

  it('does NOT auto-resume a pause the user set with "p"', () => {
    const { game, status } = makeGame()
    game.handleKeyDown(key('Enter'))
    game.handleKeyDown(key('p')) // manual pause
    expect(status()).toBe('paused')

    game.setActive(false) // already paused; not our auto-pause
    game.setActive(true) // must stay paused — respect the manual pause
    expect(status()).toBe('paused')
  })

  it('restarts the game at any time with "r"', () => {
    const { game, status } = makeGame()
    game.handleKeyDown(key('Enter'))
    expect(status()).toBe('playing')
    game.handleKeyDown(key('r'))
    expect(status()).toBe('playing') // fresh board, still playing
  })

  it('the HUD restart button starts a fresh game (mouse restart)', () => {
    const { game, status } = makeGame()
    expect(status()).toBe('ready')
    const btn = game.el.querySelector('.hud-restart') as HTMLButtonElement
    btn.dispatchEvent(new Event('click'))
    expect(status()).toBe('playing')
  })

  it('clicking the overlay starts / retries the game', () => {
    const { game, status } = makeGame()
    status() // render so the overlay reflects the ready state
    const overlay = game.el.querySelector('.tetris-overlay') as HTMLElement
    overlay.dispatchEvent(new Event('click'))
    expect(status()).toBe('playing')
  })
})

// --- input-correctness repro tests -----------------------------------------

const ev = (key: string, opts: Partial<KeyboardEventInit> = {}): KeyboardEvent =>
  new KeyboardEvent('keydown', { key, ...opts })

/** Build a game and a getter for its full latest snapshot (refreshed via render). */
function makeFullGame(): { game: TetrisGame; snap: () => GameSnapshot } {
  let snap: GameSnapshot | null = null
  const game = new TetrisGame({ onSnapshot: (s) => (snap = s) })
  return {
    game,
    snap: () => {
      game.resize() // forces a render(), which fires onSnapshot
      return snap!
    },
  }
}

const boardEmpty = (s: GameSnapshot): boolean => s.board.every((r) => r.every((v) => v === 0))

describe('TetrisGame input correctness', () => {
  // DEFECT 1 — auto-repeat must not fire one-shot actions.
  it('ignores OS auto-repeat for Space (held key does not chain hard-drops)', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter')) // ready -> playing
    const before = JSON.stringify(snap().active)
    for (let i = 0; i < 20; i++) {
      // each repeat is still consumed (preventDefault) but performs no action
      expect(game.handleKeyDown(ev(' ', { repeat: true }))).toBe(true)
    }
    const s = snap()
    expect(s.status).toBe('playing')
    expect(JSON.stringify(s.active)).toBe(before)
    expect(boardEmpty(s)).toBe(true)
  })

  it('still hard-drops on the initial (non-repeat) Space', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    expect(snap().score).toBe(0)
    expect(game.handleKeyDown(ev(' ', { repeat: false }))).toBe(true)
    expect(snap().score).toBeGreaterThan(0) // guard against disabling Space entirely
  })

  const minX = (s: GameSnapshot): number => Math.min(...s.active!.cells.map((c) => c.x))

  it('moves once on the initial arrow press but ignores OS auto-repeat (DAS drives the rest)', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    const start = minX(snap())
    expect(game.handleKeyDown(ev('ArrowRight'))).toBe(true) // initial press: one cell
    const afterPress = minX(snap())
    expect(afterPress).toBe(start + 1)
    // An OS auto-repeat keydown must NOT itself move — the run loop's DAS does.
    expect(game.handleKeyDown(ev('ArrowRight', { repeat: true }))).toBe(true)
    expect(minX(snap())).toBe(afterPress)
  })

  it('auto-shifts via DAS/ARR while a direction is held, but only after the delay', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    game.handleKeyDown(ev('ArrowRight')) // initial move
    const afterPress = minX(snap())
    // Under the DAS delay (150ms) nothing extra moves yet.
    game.tickInput(100)
    expect(minX(snap())).toBe(afterPress)
    // Cross the DAS threshold → auto-shift engages and the piece keeps moving.
    game.tickInput(120)
    expect(minX(snap())).toBeGreaterThan(afterPress)
    // Releasing the key stops the auto-shift.
    game.handleKeyUp(new KeyboardEvent('keyup', { key: 'ArrowRight' }))
    const settled = minX(snap())
    game.tickInput(500)
    expect(minX(snap())).toBe(settled)
  })

  // DEFECT 2 — bare modifier keys must not be game actions; ctrl/meta passes through.
  it('does not rotate when the bare Control key is pressed', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    // O is rotation-invariant; restart until a non-O piece so a rotation would show.
    let guard = 0
    while (snap().active!.type === 'O' && guard++ < 50) game.handleKeyDown(ev('r'))
    const before = JSON.stringify(snap().active!.cells)
    expect(game.handleKeyDown(ev('Control'))).toBe(false)
    expect(JSON.stringify(snap().active!.cells)).toBe(before)
  })

  it('does not hold when the bare Shift key is pressed', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    expect(snap().hold).toBeNull()
    expect(snap().canHold).toBe(true)
    expect(game.handleKeyDown(ev('Shift'))).toBe(false)
    expect(snap().hold).toBeNull()
  })

  it('does not consume keydowns carrying ctrl/meta (app shortcuts pass through)', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    const before = JSON.stringify(snap().active)
    // Ctrl+Space must not hard-drop — exercises the e.ctrlKey early-return.
    expect(game.handleKeyDown(ev(' ', { ctrlKey: true }))).toBe(false)
    let s = snap()
    expect(JSON.stringify(s.active)).toBe(before)
    expect(boardEmpty(s)).toBe(true)
    // ⌘+x must not rotate — exercises the e.metaKey early-return.
    expect(game.handleKeyDown(ev('x', { metaKey: true }))).toBe(false)
    expect(JSON.stringify(snap().active)).toBe(before)
  })

  // Letter controls resolve by physical key (e.code), so they work under a
  // non-Latin layout / IME where e.key is a different character.
  it('hold works under a Korean layout (physical KeyC reports e.key="ㅊ")', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    expect(snap().hold).toBeNull()
    // Korean 2-bulsik: the C key emits "ㅊ" but e.code stays "KeyC".
    const koreanC = new KeyboardEvent('keydown', { key: 'ㅊ', code: 'KeyC' })
    expect(game.handleKeyDown(koreanC)).toBe(true)
    expect(snap().hold).not.toBeNull()
  })

  it('rotate works under a Korean layout (physical KeyX reports e.key="ㅌ")', () => {
    const { game, snap } = makeFullGame()
    game.handleKeyDown(ev('Enter'))
    // advance off the rotation-invariant O piece if needed
    let guard = 0
    while (snap().active!.type === 'O' && guard++ < 50) game.handleKeyDown(ev('r'))
    const before = JSON.stringify(snap().active!.cells)
    const koreanX = new KeyboardEvent('keydown', { key: 'ㅌ', code: 'KeyX' })
    expect(game.handleKeyDown(koreanX)).toBe(true)
    expect(JSON.stringify(snap().active!.cells)).not.toBe(before)
  })
})
