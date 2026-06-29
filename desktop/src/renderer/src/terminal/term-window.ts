import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { WindowActivity } from './window-activity'

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
  /** Per-window attention state shown as a dot in the sidebar. */
  readonly activity = new WindowActivity()
  /** Working directory (from OSC 7) and its git branch, for the sidebar. */
  cwd: string | null = null
  branch: string | null = null

  onTitleChange?: () => void
  onExit?: () => void
  /** Fired when something the sidebar shows changes (activity, cwd, branch). */
  onActivity?: () => void

  private term: Terminal
  private fitAddon = new FitAddon()
  private ptyId: number | null = null
  private disposers: Array<() => void> = []
  /** Whether this is the currently selected (on-screen) window. */
  private isActive = false
  /** Debounce timer coalescing a flood of OSC 7 updates into one git lookup. */
  private branchTimer: number | null = null

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
    // OSC 7 (file://host/path) is how shells report their working directory.
    this.term.parser.registerOscHandler(7, (data) => {
      this.handleOsc7(data)
      return true
    })
    try {
      const webgl = new WebglAddon()
      webgl.onContextLoss(() => webgl.dispose())
      this.term.loadAddon(webgl)
    } catch {
      // WebGL unavailable — fall back to the DOM renderer silently.
    }

    this.safeFit()
    let ptyId: number
    try {
      ptyId = await window.tetmux.pty.create({
        cols: this.term.cols,
        rows: this.term.rows,
      })
    } catch (err) {
      // Spawning the shell failed (bad $SHELL, native binding mismatch, …).
      // Surface it in the pane and mark the tab closeable instead of leaving a
      // silent, blank, non-functional window.
      const msg = err instanceof Error ? err.message : String(err)
      this.exited = true
      this.activity.onExit()
      this.term.write(`\r\n\x1b[31m[failed to start shell: ${msg}]\x1b[0m\r\n`)
      this.onExit?.()
      return
    }
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
      window.tetmux.pty.onData(ptyId, (d) => {
        this.term.write(d)
        // Background output marks the window unread (and a bell makes it shout)
        // so the sidebar can nudge the user back from Tetris.
        if (this.activity.onOutput(this.isActive, d)) this.onActivity?.()
      }),
      window.tetmux.pty.onExit(ptyId, () => {
        this.exited = true
        this.activity.onExit()
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

  /** Mark this window selected/deselected; selecting clears its attention state. */
  markActive(active: boolean): void {
    this.isActive = active
    if (active) this.activity.onSelect()
  }

  private handleOsc7(data: string): void {
    // data looks like "file://hostname/Users/me/project"
    const match = /^file:\/\/[^/]*(\/.*)$/.exec(data)
    if (!match) return
    let path: string
    try {
      path = decodeURIComponent(match[1])
    } catch {
      return // malformed percent-encoding from a hostile OSC 7 — ignore
    }
    if (!path.startsWith('/') || path.length > 4096) return
    if (path === this.cwd) return
    this.cwd = path
    this.branch = null // don't show the previous repo's branch against the new cwd
    this.onActivity?.()
    // Debounce: a shell can emit OSC 7 on every prompt, and a hostile process
    // could flood them — coalesce into at most one git lookup per quiet period.
    if (this.branchTimer !== null) window.clearTimeout(this.branchTimer)
    this.branchTimer = window.setTimeout(() => {
      this.branchTimer = null
      void this.refreshBranch(path)
    }, 250)
  }

  private async refreshBranch(path: string): Promise<void> {
    try {
      const branch = await window.tetmux.git.branch(path)
      // Ignore a stale response if the cwd has since changed.
      if (path === this.cwd && branch !== this.branch) {
        this.branch = branch
        this.onActivity?.()
      }
    } catch {
      // git lookup is best-effort; never let it break the terminal.
    }
  }

  dispose(): void {
    if (this.branchTimer !== null) window.clearTimeout(this.branchTimer)
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
