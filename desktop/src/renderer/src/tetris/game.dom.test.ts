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
})
