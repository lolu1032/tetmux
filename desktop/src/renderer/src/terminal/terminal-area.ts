import { TermWindow } from './term-window'

/**
 * The left side of the app: a vertical sidebar listing the terminal windows plus
 * a stack of panes. Exactly one window is visible at a time; the rest stay alive
 * in the background (their ptys keep running) and accrue attention state shown as
 * a coloured dot in the sidebar.
 */
export class TerminalArea {
  readonly sidebar: HTMLElement
  readonly panes: HTMLElement

  private readonly winList: HTMLElement
  private windows: TermWindow[] = []
  private active = -1
  private nextId = 1

  /** Fired whenever anything the host renders changes (selection, attention, …). */
  onChange?: () => void

  constructor() {
    this.sidebar = document.createElement('div')
    this.sidebar.className = 'sidebar'

    const head = document.createElement('div')
    head.className = 'sidebar-head'
    const brand = document.createElement('span')
    brand.className = 'brand'
    brand.textContent = 'tetmux'
    const add = document.createElement('button')
    add.className = 'win-add'
    add.textContent = '+'
    add.title = 'New window  (⌘T)'
    add.addEventListener('click', () => void this.newWindow())
    head.append(brand, add)

    this.winList = document.createElement('div')
    this.winList.className = 'win-list'
    this.sidebar.append(head, this.winList)

    this.panes = document.createElement('div')
    this.panes.className = 'panes'
  }

  get count(): number {
    return this.windows.length
  }

  get activeWindow(): TermWindow | null {
    return this.windows[this.active] ?? null
  }

  get activeLabel(): string {
    const w = this.activeWindow
    return w ? `${this.active + 1}:${w.title}` : '—'
  }

  /** Background windows currently wanting attention (for the status bar). */
  get attentionCount(): number {
    return this.windows.filter((w, i) => i !== this.active && w.activity.needsAttention).length
  }

  async newWindow(): Promise<void> {
    const win = new TermWindow(this.nextId++)
    win.onTitleChange = () => this.refresh()
    win.onExit = () => this.refresh()
    win.onActivity = () => this.refresh()
    // The element must be in the DOM before open() so xterm can measure/fit it,
    // but the window only joins `this.windows` once open() resolves — a failed
    // open is rolled back so it never lingers as an orphan with no entry.
    this.panes.appendChild(win.el)
    try {
      await win.open()
    } catch {
      win.dispose()
      return
    }
    this.windows.push(win)
    this.select(this.windows.length - 1)
  }

  select(index: number): void {
    if (index < 0 || index >= this.windows.length) return
    this.active = index
    this.windows.forEach((w, i) => {
      w.setVisible(i === index)
      w.markActive(i === index)
    })
    this.windows[index].fit()
    this.refresh()
  }

  /** Re-render the sidebar and let the host refresh the status bar. */
  private refresh(): void {
    this.renderSidebar()
    this.onChange?.()
  }

  next(): void {
    if (this.windows.length) this.select((this.active + 1) % this.windows.length)
  }

  prev(): void {
    if (this.windows.length) {
      this.select((this.active - 1 + this.windows.length) % this.windows.length)
    }
  }

  closeWindow(index: number): void {
    const win = this.windows[index]
    if (!win) return
    const activeWin = this.windows[this.active]
    win.dispose()
    this.windows.splice(index, 1)
    if (this.windows.length === 0) {
      this.active = -1
      void this.newWindow()
      return
    }
    if (win === activeWin) {
      // Closed the active window — move to a neighbour.
      this.select(Math.min(index, this.windows.length - 1))
    } else {
      // Closing a background window must not change which window is selected
      // (nor clear its attention) — just fix the index and re-render.
      this.active = this.windows.indexOf(activeWin as TermWindow)
      this.refresh()
    }
  }

  closeActive(): void {
    if (this.active >= 0) this.closeWindow(this.active)
  }

  focusActive(): void {
    this.activeWindow?.focus()
  }

  blurActive(): void {
    this.activeWindow?.blur()
  }

  fitActive(): void {
    this.activeWindow?.fit()
  }

  private subLabel(win: TermWindow): string {
    const parts: string[] = []
    if (win.cwd) parts.push(win.cwd.split('/').filter(Boolean).pop() || '/')
    if (win.branch) parts.push(win.branch)
    return parts.join(' · ')
  }

  private renderSidebar(): void {
    const items = this.windows.map((win, i) => {
      const item = document.createElement('div')
      item.className = 'win-item' + (i === this.active ? ' active' : '')

      const dot = document.createElement('span')
      dot.className = `win-dot state-${win.activity.state}`
      dot.title = win.activity.state

      const meta = document.createElement('div')
      meta.className = 'win-meta'
      const title = document.createElement('div')
      title.className = 'win-title'
      title.textContent = `${i + 1}: ${win.title}` // textContent: PTY title stays inert
      meta.appendChild(title)
      const sub = this.subLabel(win)
      if (sub) {
        const subEl = document.createElement('div')
        subEl.className = 'win-sub'
        subEl.textContent = sub
        meta.appendChild(subEl)
      }

      const close = document.createElement('button')
      close.className = 'win-close'
      close.textContent = '✕'
      close.title = 'Close window'
      close.addEventListener('click', (e) => {
        e.stopPropagation()
        this.closeWindow(i)
      })

      item.append(dot, meta, close)
      item.addEventListener('click', () => this.select(i))
      return item
    })
    this.winList.replaceChildren(...items)
  }
}
