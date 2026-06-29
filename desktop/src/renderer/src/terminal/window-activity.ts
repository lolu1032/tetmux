export type ActivityState = 'idle' | 'unread' | 'bell' | 'exited'

const BELL = '\x07'

/**
 * Per-window attention state for the sidebar. A background (non-selected) window
 * accrues "unread" when it emits output and "bell" when it rings the terminal
 * bell; selecting the window clears those. Exit is sticky (the process is gone).
 * Pure and DOM-free so it can be unit-tested directly.
 */
export class WindowActivity {
  private unread = false
  private bell = false
  private exited = false

  /**
   * Record output. `isActive` is whether this is the currently selected window
   * (the one on screen). Returns true if the visible state changed.
   */
  onOutput(isActive: boolean, data: string): boolean {
    if (this.exited || isActive) return false
    let changed = false
    if (!this.unread) {
      this.unread = true
      changed = true
    }
    if (!this.bell && data.includes(BELL)) {
      this.bell = true
      changed = true
    }
    return changed
  }

  /** The backing process exited. Returns true if the state changed. */
  onExit(): boolean {
    if (this.exited) return false
    this.exited = true
    return true
  }

  /** The window became the selected one — clear unread/bell (not exit). */
  onSelect(): boolean {
    if (!this.unread && !this.bell) return false
    this.unread = false
    this.bell = false
    return true
  }

  get state(): ActivityState {
    if (this.exited) return 'exited'
    if (this.bell) return 'bell'
    if (this.unread) return 'unread'
    return 'idle'
  }

  /** True when the window wants the user's attention (for the status-bar count). */
  get needsAttention(): boolean {
    return this.exited || this.bell || this.unread
  }
}
