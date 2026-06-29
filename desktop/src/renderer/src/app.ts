import { TerminalArea } from './terminal/terminal-area'
import { TetrisGame } from './tetris/game'
import type { GameSnapshot } from './tetris/engine'
import { renderStatus, statusKey } from './status-bar'
import type { Focus } from './types'

const RATIO_KEY = 'tetmux.ratio'
const MIN_RATIO = 0.3
const MAX_RATIO = 0.82

/**
 * Top-level controller. Builds the split layout (terminals left, Tetris right),
 * owns focus routing and global keybindings, and keeps the status bar current.
 */
export class App {
  private readonly root: HTMLElement
  private readonly workspace = document.createElement('div')
  private readonly left = document.createElement('div')
  private readonly divider = document.createElement('div')
  private readonly right = document.createElement('div')
  private readonly statusBar = document.createElement('div')

  private readonly terminals = new TerminalArea()
  private readonly tetris: TetrisGame

  private focus: Focus = 'terminal'
  private ratio = Number(localStorage.getItem(RATIO_KEY) || 0.62)
  private prefixActive = false
  private prefixTimer = 0
  private lastSnapshot: GameSnapshot | null = null
  private lastBest = 0
  private lastStatus = ''

  constructor(root: HTMLElement) {
    this.root = root
    this.tetris = new TetrisGame({
      onSnapshot: (snap, best) => {
        this.lastSnapshot = snap
        this.lastBest = best
        this.updateStatus()
      },
    })
    this.buildDom()
  }

  async start(): Promise<void> {
    await this.terminals.newWindow()
    this.tetris.run()
    this.applyRatio()
    this.setFocus('terminal')
    window.addEventListener('resize', () => this.onResize())
    window.addEventListener('keydown', (e) => this.onKeyDown(e), true)
    window.addEventListener('keyup', (e) => this.onKeyUp(e), true)
    this.updateStatus()
  }

  // ---- DOM -----------------------------------------------------------------

  private buildDom(): void {
    this.workspace.className = 'workspace'
    this.left.className = 'left'
    this.divider.className = 'divider'
    this.right.className = 'right'
    this.statusBar.className = 'status-bar'

    this.left.append(this.terminals.panes)
    this.right.appendChild(this.tetris.el)
    // [ sidebar | terminals | divider | tetris ]
    this.workspace.append(this.terminals.sidebar, this.left, this.divider, this.right)
    this.root.append(this.workspace, this.statusBar)

    this.terminals.onChange = () => this.updateStatus()

    // Click a pane to focus it.
    this.terminals.panes.addEventListener('mousedown', () => this.setFocus('terminal'))
    this.tetris.el.addEventListener('mousedown', () => this.setFocus('tetris'))

    this.setupDividerDrag()
  }

  private applyRatio(): void {
    // The fixed-width sidebar sits outside the split; the terminal and Tetris
    // panes share the remaining space in ratio : (1 - ratio).
    this.left.style.flex = `${this.ratio} 1 0`
    this.right.style.flex = `${1 - this.ratio} 1 0`
    this.terminals.fitActive()
    this.tetris.resize()
  }

  private setupDividerDrag(): void {
    const onMove = (e: MouseEvent): void => {
      // Ratio is measured across the terminal+Tetris region only (excludes sidebar).
      const start = this.left.getBoundingClientRect().left
      const total = this.right.getBoundingClientRect().right - start
      if (total <= 0) return
      const r = (e.clientX - start) / total
      this.ratio = Math.min(MAX_RATIO, Math.max(MIN_RATIO, r))
      this.applyRatio()
    }
    const onUp = (): void => {
      document.body.classList.remove('dragging')
      localStorage.setItem(RATIO_KEY, String(this.ratio))
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    this.divider.addEventListener('mousedown', (e) => {
      e.preventDefault()
      document.body.classList.add('dragging')
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
    })
  }

  private onResize(): void {
    this.terminals.fitActive()
    this.tetris.resize()
  }

  // ---- focus ---------------------------------------------------------------

  private setFocus(focus: Focus): void {
    this.focus = focus
    if (focus === 'terminal') {
      this.tetris.setActive(false)
      this.terminals.focusActive()
    } else {
      this.terminals.blurActive()
      this.tetris.setActive(true)
    }
    document.body.dataset.focus = focus
    this.updateStatus()
  }

  private toggleFocus(): void {
    this.setFocus(this.focus === 'terminal' ? 'tetris' : 'terminal')
  }

  // ---- keyboard ------------------------------------------------------------

  private onKeyDown(e: KeyboardEvent): void {
    if (this.prefixActive) {
      this.handlePrefixCommand(e)
      return
    }
    // tmux-style prefix: Ctrl+B
    if (e.ctrlKey && (e.key === 'b' || e.key === 'B')) {
      e.preventDefault()
      this.enterPrefix()
      return
    }
    // macOS command shortcuts
    if (e.metaKey && this.handleMetaShortcut(e)) return

    if (this.focus === 'tetris') {
      if (e.key === 'Tab' || e.key === 'Escape') {
        e.preventDefault()
        this.setFocus('terminal')
        return
      }
      if (this.tetris.handleKeyDown(e)) e.preventDefault()
    }
  }

  private onKeyUp(e: KeyboardEvent): void {
    if (this.focus === 'tetris') this.tetris.handleKeyUp(e)
  }

  private enterPrefix(): void {
    this.prefixActive = true
    document.body.classList.add('prefix')
    this.updateStatus()
    window.clearTimeout(this.prefixTimer)
    this.prefixTimer = window.setTimeout(() => this.exitPrefix(), 2000)
  }

  private exitPrefix(): void {
    this.prefixActive = false
    document.body.classList.remove('prefix')
    window.clearTimeout(this.prefixTimer)
    this.updateStatus()
  }

  private handlePrefixCommand(e: KeyboardEvent): void {
    e.preventDefault()
    const key = e.key
    if (key === 'Control' || key === 'Shift' || key === 'Alt' || key === 'Meta') {
      return // ignore lone modifiers, stay in prefix
    }
    if (/^[1-9]$/.test(key)) {
      this.terminals.select(Number(key) - 1)
    } else {
      switch (key) {
        case 'c':
          void this.terminals.newWindow()
          break
        case 'n':
          this.terminals.next()
          break
        case 'p':
          this.terminals.prev()
          break
        case '&':
        case 'x':
          this.terminals.closeActive()
          break
        case ' ':
          this.setFocus('tetris')
          break
        case 'Tab':
          this.toggleFocus()
          break
      }
    }
    this.exitPrefix()
  }

  private handleMetaShortcut(e: KeyboardEvent): boolean {
    if (/^[1-9]$/.test(e.key)) {
      e.preventDefault()
      this.terminals.select(Number(e.key) - 1)
      return true
    }
    switch (e.key) {
      case 't':
        e.preventDefault()
        void this.terminals.newWindow()
        return true
      case 'w':
        e.preventDefault()
        this.terminals.closeActive()
        return true
      case '}':
      case ']':
        e.preventDefault()
        this.terminals.next()
        return true
      case '{':
      case '[':
        e.preventDefault()
        this.terminals.prev()
        return true
      default:
        return false
    }
  }

  // ---- status bar ----------------------------------------------------------

  private updateStatus(): void {
    const snap = this.lastSnapshot
    const hint = this.prefixActive
      ? 'prefix: c new · n/p next/prev · 1-9 window · space play · & close'
      : 'C-b c:new · C-b space:play · click pane to focus · ⌘T new'

    const data = {
      winLabel: this.terminals.activeLabel,
      winCount: this.terminals.count,
      attention: this.terminals.attentionCount,
      focus: this.focus,
      score: snap ? snap.score : 0,
      best: this.lastBest,
      level: snap ? snap.level : 0,
      gameStatus: snap ? snap.status : ('ready' as const),
      hint,
    }
    const key = statusKey(data)
    if (key === this.lastStatus) return
    this.lastStatus = key
    renderStatus(this.statusBar, data)
  }
}
