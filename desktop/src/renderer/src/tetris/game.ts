import { NEXT_COUNT, TetrisEngine, type GameSnapshot } from './engine'
import { drawMini, drawPlayfield } from './render'

const BEST_KEY = 'tetmux.best'

// Keys whose action must fire only on the INITIAL press. The OS auto-repeats
// keydown (~30/s) while a key is held; for these one-shot actions a repeat must
// be a no-op (e.g. holding Space must not chain hard-drops). Movement
// (ArrowLeft/ArrowRight) and soft-drop (ArrowDown) are intentionally absent so
// they keep honoring auto-repeat.
const ONE_SHOT = new Set([' ', 'ArrowUp', 'x', 'X', 'z', 'Z', 'c', 'C', 'r', 'R', 'Enter', 'p', 'P'])

function fit(canvas: HTMLCanvasElement, cssW: number, cssH: number): CanvasRenderingContext2D {
  const dpr = window.devicePixelRatio || 1
  canvas.width = Math.max(1, Math.round(cssW * dpr))
  canvas.height = Math.max(1, Math.round(cssH * dpr))
  canvas.style.width = `${cssW}px`
  canvas.style.height = `${cssH}px`
  const ctx = canvas.getContext('2d')!
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  return ctx
}

/**
 * The Tetris pane: owns the engine, the canvases, the render loop and keyboard
 * handling. The host app decides *when* keys reach it (focus routing) by calling
 * handleKeyDown / handleKeyUp.
 */
export class TetrisGame {
  readonly el: HTMLElement
  private readonly engine: TetrisEngine
  private readonly boardCanvas: HTMLCanvasElement
  private readonly holdCanvas: HTMLCanvasElement
  private readonly nextCanvas: HTMLCanvasElement
  private readonly boardWrap: HTMLElement
  private readonly overlay: HTMLElement
  private readonly stats: HTMLElement

  private best = Number(localStorage.getItem(BEST_KEY) || 0)
  private lastTime = 0
  private raf = 0
  // True only when a pause was triggered by losing focus (not by the user's
  // own 'p'). Lets focus-resume distinguish "I auto-paused this" from "the user
  // deliberately paused", so a manual pause is never silently resumed.
  private autoPaused = false
  private readonly onSnapshot?: (snap: GameSnapshot, best: number) => void

  constructor(opts: { onSnapshot?: (snap: GameSnapshot, best: number) => void } = {}) {
    this.engine = new TetrisEngine()
    this.onSnapshot = opts.onSnapshot

    this.el = document.createElement('div')
    this.el.className = 'tetris'
    this.el.innerHTML = `
      <div class="tetris-board">
        <canvas class="board-canvas"></canvas>
        <div class="tetris-overlay"></div>
      </div>
      <div class="tetris-side">
        <div class="hud-box">
          <div class="hud-label">HOLD</div>
          <canvas class="hold-canvas" width="92" height="72"></canvas>
        </div>
        <div class="hud-box">
          <div class="hud-label">NEXT</div>
          <canvas class="next-canvas" width="92" height="220"></canvas>
        </div>
        <div class="hud-stats"></div>
      </div>`

    this.boardWrap = this.el.querySelector('.tetris-board') as HTMLElement
    this.boardCanvas = this.el.querySelector('.board-canvas') as HTMLCanvasElement
    this.holdCanvas = this.el.querySelector('.hold-canvas') as HTMLCanvasElement
    this.nextCanvas = this.el.querySelector('.next-canvas') as HTMLCanvasElement
    this.overlay = this.el.querySelector('.tetris-overlay') as HTMLElement
    this.stats = this.el.querySelector('.hud-stats') as HTMLElement

    const ro = new ResizeObserver(() => this.resize())
    ro.observe(this.boardWrap)
  }

  setActive(active: boolean): void {
    this.el.classList.toggle('focused', active)
    const status = this.engine.snapshot().status
    if (!active) {
      // A soft-drop key held at the moment focus leaves would never receive its
      // keyup (handleKeyUp only runs while focused), so clear it here — otherwise
      // the piece keeps fast-dropping (and scoring) after focus returns.
      this.engine.setSoftDrop(false)
      // Focus left the Tetris pane — auto-pause a live game so it does not keep
      // falling (and topping out) while the user works in the terminal. This is
      // the whole point of the app: the game waits for you.
      if (status === 'playing') {
        this.engine.togglePause()
        this.autoPaused = true
      }
    } else {
      // Focus returned — resume only what *we* auto-paused; respect a manual 'p'.
      if (this.autoPaused && status === 'paused') this.engine.togglePause()
      this.autoPaused = false
    }
  }

  /** Start the requestAnimationFrame loop. Idempotent. */
  run(): void {
    if (this.raf) return
    const loop = (t: number): void => {
      const dt = this.lastTime ? Math.min(t - this.lastTime, 100) : 0
      this.lastTime = t
      this.engine.tick(dt)
      this.render()
      this.raf = requestAnimationFrame(loop)
    }
    this.raf = requestAnimationFrame(loop)
  }

  // ---- input ---------------------------------------------------------------

  /** Returns true if the key was a game action (caller should preventDefault). */
  handleKeyDown(e: KeyboardEvent): boolean {
    // Let app-level shortcuts (the Ctrl+B window prefix, ⌘ shortcuts) pass
    // through untouched — never swallow a key carrying a ctrl/meta modifier.
    if (e.ctrlKey || e.metaKey) return false
    // Auto-repeat must not re-trigger one-shot actions; still consume the key so
    // it never leaks to the page (caller preventDefaults on a true return).
    if (e.repeat && ONE_SHOT.has(e.key)) return true
    switch (e.key) {
      case 'ArrowLeft':
        this.engine.moveLeft()
        return true
      case 'ArrowRight':
        this.engine.moveRight()
        return true
      case 'ArrowDown':
        this.engine.setSoftDrop(true)
        return true
      case 'ArrowUp':
      case 'x':
      case 'X':
        this.engine.rotate(1)
        return true
      case 'z':
      case 'Z':
        this.engine.rotate(-1)
        return true
      case ' ':
        this.engine.hardDrop()
        return true
      case 'c':
      case 'C':
        this.engine.hold()
        return true
      case 'p':
      case 'P':
        this.engine.togglePause()
        return true
      case 'r':
      case 'R':
        // Restart with a fresh board at any time (matches the original Go TUI).
        this.engine.start()
        this.autoPaused = false
        return true
      case 'Enter': {
        const status = this.engine.snapshot().status
        if (status === 'ready' || status === 'gameover') this.engine.start()
        return true
      }
      default:
        return false
    }
  }

  handleKeyUp(e: KeyboardEvent): void {
    if (e.key === 'ArrowDown') this.engine.setSoftDrop(false)
  }

  // ---- rendering -----------------------------------------------------------

  resize(): void {
    const w = this.boardWrap.clientWidth
    const h = this.boardWrap.clientHeight
    if (w > 0 && h > 0) fit(this.boardCanvas, w, h)
    this.render()
  }

  private render(): void {
    const snap = this.engine.snapshot()
    if (snap.score > this.best) {
      this.best = snap.score
      localStorage.setItem(BEST_KEY, String(this.best))
    }

    const boardCtx = this.boardCanvas.getContext('2d')
    if (boardCtx) {
      const w = parseFloat(this.boardCanvas.style.width) || this.boardWrap.clientWidth
      const h = parseFloat(this.boardCanvas.style.height) || this.boardWrap.clientHeight
      drawPlayfield(boardCtx, w, h, snap)
    }

    const holdCtx = this.holdCanvas.getContext('2d')
    if (holdCtx) drawMini(holdCtx, this.holdCanvas.width, this.holdCanvas.height, snap.hold)

    const nextCtx = this.nextCanvas.getContext('2d')
    if (nextCtx) {
      const w = this.nextCanvas.width
      const h = this.nextCanvas.height
      nextCtx.clearRect(0, 0, w, h)
      const shown = snap.nextQueue.slice(0, NEXT_COUNT)
      const slotH = h / Math.max(shown.length, 1)
      shown.forEach((type, i) => {
        nextCtx.save()
        nextCtx.translate(0, i * slotH)
        drawMini(nextCtx, w, slotH, type)
        nextCtx.restore()
      })
    }

    this.stats.innerHTML = `
      <div class="stat"><span>SCORE</span><b>${snap.score}</b></div>
      <div class="stat"><span>BEST</span><b>${this.best}</b></div>
      <div class="stat"><span>LINES</span><b>${snap.lines}</b></div>
      <div class="stat"><span>LEVEL</span><b>${snap.level}</b></div>`

    this.renderOverlay(snap)
    this.onSnapshot?.(snap, this.best)
  }

  private renderOverlay(snap: GameSnapshot): void {
    let html = ''
    if (snap.status === 'ready') {
      html = `<div class="overlay-card"><h2>tetris</h2><p>press <kbd>Enter</kbd> to play</p></div>`
    } else if (snap.status === 'paused') {
      html = `<div class="overlay-card"><h2>paused</h2><p><kbd>P</kbd> to resume</p></div>`
    } else if (snap.status === 'gameover') {
      html = `<div class="overlay-card"><h2>game over</h2><p>score ${snap.score} · <kbd>Enter</kbd> to retry</p></div>`
    }
    this.overlay.innerHTML = html
    this.overlay.classList.toggle('visible', html !== '')
  }
}
