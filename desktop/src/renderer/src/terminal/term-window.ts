import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'

const THEME = {
  background: '#0b0e14',
  foreground: '#c9d1d9',
  cursor: '#2dd4bf',
  cursorAccent: '#0b0e14',
  selectionBackground: 'rgba(45,212,191,0.30)',
  black: '#0b0e14',
  brightBlack: '#5c6370',
  red: '#f87171',
  brightRed: '#fca5a5',
  green: '#4ade80',
  brightGreen: '#86efac',
  yellow: '#facc15',
  brightYellow: '#fde047',
  blue: '#60a5fa',
  brightBlue: '#93c5fd',
  magenta: '#c084fc',
  brightMagenta: '#d8b4fe',
  cyan: '#2dd4bf',
  brightCyan: '#5eead4',
  white: '#c9d1d9',
  brightWhite: '#f0f6fc',
}

/**
 * One terminal window: an xterm.js instance bound to a backing pty in the main
 * process. The element stays in the DOM (hidden when inactive) so scrollback and
 * the running shell survive tab switches.
 */
export class TermWindow {
  readonly id: number
  readonly el: HTMLElement
  title = 'shell'
  exited = false

  onTitleChange?: () => void
  onExit?: () => void

  private term: Terminal
  private fitAddon = new FitAddon()
  private ptyId: number | null = null
  private disposers: Array<() => void> = []

  constructor(id: number) {
    this.id = id
    this.el = document.createElement('div')
    this.el.className = 'term-window'
    this.term = new Terminal({
      fontFamily: 'Menlo, "SF Mono", "Cascadia Code", "JetBrains Mono", monospace',
      fontSize: 13,
      lineHeight: 1.1,
      cursorBlink: true,
      allowProposedApi: true,
      scrollback: 5000,
      theme: THEME,
    })
  }

  async open(): Promise<void> {
    this.term.open(this.el)
    this.term.loadAddon(this.fitAddon)
    try {
      const webgl = new WebglAddon()
      webgl.onContextLoss(() => webgl.dispose())
      this.term.loadAddon(webgl)
    } catch {
      // WebGL unavailable — fall back to the DOM renderer silently.
    }

    this.safeFit()
    const ptyId = await window.tetmux.pty.create({
      cols: this.term.cols,
      rows: this.term.rows,
      name: this.title,
    })
    this.ptyId = ptyId

    const onData = this.term.onData((d) => window.tetmux.pty.write(ptyId, d))
    const onResize = this.term.onResize(({ cols, rows }) =>
      window.tetmux.pty.resize(ptyId, cols, rows),
    )
    const onTitle = this.term.onTitleChange((t) => {
      this.title = t || 'shell'
      this.onTitleChange?.()
    })
    this.disposers.push(
      () => onData.dispose(),
      () => onResize.dispose(),
      () => onTitle.dispose(),
      window.tetmux.pty.onData(ptyId, (d) => this.term.write(d)),
      window.tetmux.pty.onExit(ptyId, () => {
        this.exited = true
        this.term.write('\r\n\x1b[2m[process exited — press the ✕ to close this tab]\x1b[0m\r\n')
        this.onExit?.()
      }),
    )
  }

  private safeFit(): void {
    try {
      this.fitAddon.fit()
    } catch {
      // fit throws if the element has no size yet (hidden); ignored.
    }
  }

  fit(): void {
    this.safeFit()
  }

  focus(): void {
    this.term.focus()
  }

  blur(): void {
    this.term.blur()
  }

  setVisible(visible: boolean): void {
    this.el.style.display = visible ? 'block' : 'none'
  }

  dispose(): void {
    for (const d of this.disposers) {
      try {
        d()
      } catch {
        /* ignore */
      }
    }
    this.disposers = []
    if (this.ptyId !== null) window.tetmux.pty.kill(this.ptyId)
    this.term.dispose()
    this.el.remove()
  }
}
