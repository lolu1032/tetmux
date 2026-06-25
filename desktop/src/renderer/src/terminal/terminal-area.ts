import { TermWindow } from './term-window'

/**
 * The left side of the app: a stack of terminal windows plus a tmux-style tab
 * bar. Exactly one window is visible at a time; the rest stay alive in the
 * background (their ptys keep running).
 */
export class TerminalArea {
  readonly tabBar: HTMLElement
  readonly panes: HTMLElement

  private windows: TermWindow[] = []
  private active = -1
  private nextId = 1

  onActiveChange?: () => void

  constructor() {
    this.tabBar = document.createElement('div')
    this.tabBar.className = 'tab-bar'
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

  async newWindow(): Promise<void> {
    const win = new TermWindow(this.nextId++)
    win.onTitleChange = () => this.renderTabs()
    win.onExit = () => this.renderTabs()
    this.windows.push(win)
    this.panes.appendChild(win.el)
    await win.open()
    this.select(this.windows.length - 1)
    this.renderTabs()
  }

  select(index: number): void {
    if (index < 0 || index >= this.windows.length) return
    this.active = index
    this.windows.forEach((w, i) => w.setVisible(i === index))
    const win = this.windows[index]
    win.fit()
    this.renderTabs()
    this.onActiveChange?.()
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
    win.dispose()
    this.windows.splice(index, 1)
    if (this.windows.length === 0) {
      this.active = -1
      void this.newWindow()
      return
    }
    this.select(Math.min(index, this.windows.length - 1))
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

  private renderTabs(): void {
    this.tabBar.replaceChildren()
    this.windows.forEach((win, i) => {
      const tab = document.createElement('div')
      tab.className = 'tab' + (i === this.active ? ' active' : '') + (win.exited ? ' exited' : '')
      const label = document.createElement('span')
      label.className = 'tab-label'
      label.textContent = `${i + 1}:${win.title}`
      label.addEventListener('click', () => this.select(i))

      const close = document.createElement('button')
      close.className = 'tab-close'
      close.textContent = '✕'
      close.title = 'Close window'
      close.addEventListener('click', (e) => {
        e.stopPropagation()
        this.closeWindow(i)
      })

      tab.append(label, close)
      this.tabBar.appendChild(tab)
    })

    const add = document.createElement('button')
    add.className = 'tab-add'
    add.textContent = '+'
    add.title = 'New window  (⌘T)'
    add.addEventListener('click', () => void this.newWindow())
    this.tabBar.appendChild(add)
  }
}
