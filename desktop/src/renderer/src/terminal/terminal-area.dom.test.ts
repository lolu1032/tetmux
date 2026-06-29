import { beforeEach, describe, expect, it, vi } from 'vitest'

// xterm + its addons need a real canvas/WebGL backend that happy-dom lacks, and
// here we only exercise TerminalArea/TermWindow *wiring* (cwd threading + the
// onActivate focus hook), so stub the terminal engine out entirely.
vi.mock('@xterm/xterm', () => {
  class Terminal {
    cols = 80
    rows = 24
    parser = { registerOscHandler: (): void => {} }
    constructor(_opts: unknown) {}
    open(): void {}
    loadAddon(): void {}
    onData(): { dispose(): void } {
      return { dispose() {} }
    }
    onResize(): { dispose(): void } {
      return { dispose() {} }
    }
    onTitleChange(): { dispose(): void } {
      return { dispose() {} }
    }
    write(): void {}
    focus(): void {}
    blur(): void {}
    dispose(): void {}
  }
  return { Terminal }
})
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit(): void {}
  },
}))
vi.mock('@xterm/addon-webgl', () => ({
  WebglAddon: class {
    onContextLoss(): void {}
    dispose(): void {}
  },
}))

import { TerminalArea } from './terminal-area'

type CreateOpts = { cols: number; rows: number; cwd?: string }
const created: CreateOpts[] = []

beforeEach(() => {
  created.length = 0
  let nextPtyId = 1
  ;(window as unknown as { tetmux: unknown }).tetmux = {
    pty: {
      create: async (opts: CreateOpts) => {
        created.push(opts)
        return nextPtyId++
      },
      write: () => {},
      resize: () => {},
      kill: () => {},
      onData: () => () => {},
      onExit: () => () => {},
    },
    git: { branch: async () => null },
    platform: 'darwin',
  }
})

describe('TerminalArea — cwd inheritance + focus routing', () => {
  it('DEFECT 1: a new window inherits the active window cwd; the first window falls back to homedir', async () => {
    const area = new TerminalArea()

    // First window: no active window yet -> no cwd threaded (PtyManager uses homedir).
    await area.newWindow()
    expect(created).toHaveLength(1)
    expect(created[0].cwd).toBeUndefined()

    // The active window now reports a working directory (as OSC 7 would set it).
    area.activeWindow!.cwd = '/Users/me/project'
    await area.newWindow()
    expect(created).toHaveLength(2)
    expect(created[1].cwd).toBe('/Users/me/project')
  })

  it('DEFECT 1: a relative or empty cwd is never threaded (falls back to homedir)', async () => {
    const area = new TerminalArea()
    await area.newWindow()

    area.activeWindow!.cwd = 'relative/dir'
    await area.newWindow()
    expect(created[1].cwd).toBeUndefined()

    area.activeWindow!.cwd = ''
    await area.newWindow()
    expect(created[2].cwd).toBeUndefined()
  })

  it('DEFECT 2: user-driven selection fires onActivate; programmatic refresh does not', async () => {
    const area = new TerminalArea()
    const onActivate = vi.fn()
    area.onActivate = onActivate

    await area.newWindow() // window 1 -> select()
    await area.newWindow() // window 2 -> select()
    const afterCreate = onActivate.mock.calls.length
    expect(afterCreate).toBeGreaterThanOrEqual(2) // newWindow funnels through select()

    area.select(0)
    area.next()
    area.prev()
    expect(onActivate.mock.calls.length).toBe(afterCreate + 3)

    // Closing a BACKGROUND window goes through refresh(), not select(): no focus steal.
    area.select(1) // make window index 1 the active one
    onActivate.mockClear()
    area.closeWindow(0) // closes a background window
    expect(onActivate).not.toHaveBeenCalled()
  })

  it('sendInput writes raw bytes to the active window pty (literal C-b passthrough)', async () => {
    const area = new TerminalArea()
    await area.newWindow() // active window gets ptyId 1 (mock create returns 1,2,…)
    const writes: Array<[number, string]> = []
    ;(window as unknown as { tetmux: { pty: { write: unknown } } }).tetmux.pty.write = (
      id: number,
      data: string,
    ): void => void writes.push([id, data])
    area.sendInput('\x02')
    expect(writes).toEqual([[1, '\x02']])
  })
})
