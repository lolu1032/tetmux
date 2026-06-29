import { describe, expect, it, vi } from 'vitest'

// A controllable fake node-pty. vi.hoisted runs before the vi.mock factory so the
// factory (which is itself hoisted above imports) can reference it safely.
const h = vi.hoisted(() => {
  const spawned: any[] = []
  const state = {
    spawned,
    // overridable so a test can make spawn throw
    spawn: (file: string, _args: string[], opts: any) => {
      const fake: any = {
        file,
        cols: opts.cols,
        rows: opts.rows,
        written: [] as string[],
        resizeCalls: [] as Array<[number, number]>,
        killed: 0,
        resizeThrows: false,
        onData(cb: (d: string) => void) {
          fake.dataCb = cb
        },
        onExit(cb: (e: { exitCode: number; signal?: number }) => void) {
          fake.exitCb = cb
        },
        write(d: string) {
          fake.written.push(d)
        },
        resize(c: number, r: number) {
          if (fake.resizeThrows) throw new Error('resize after exit')
          fake.resizeCalls.push([c, r])
        },
        kill() {
          fake.killed++
        },
      }
      spawned.push(fake)
      return fake
    },
  }
  return state
})

vi.mock('node-pty', () => ({
  spawn: (file: string, args: string[], opts: any) => h.spawn(file, args, opts),
}))

import { PtyManager, clampDim } from './pty-manager'

const opts = (cols: number, rows: number) => ({ cols, rows })

describe('clampDim', () => {
  it('truncates, enforces a minimum, caps the maximum, and falls back for non-finite input', () => {
    expect(clampDim(10.9, 2, 80)).toBe(10)
    expect(clampDim(0, 1, 24)).toBe(1)
    expect(clampDim(-5, 2, 80)).toBe(2)
    expect(clampDim(NaN, 2, 80)).toBe(80)
    expect(clampDim(Infinity, 1, 24)).toBe(24)
    expect(clampDim(100, 2, 80)).toBe(100)
    // huge finite values are capped, not passed through to the pty
    expect(clampDim(Number.MAX_VALUE, 2, 80)).toBe(2000)
    expect(clampDim(1e9, 1, 24)).toBe(2000)
  })
})

describe('PtyManager', () => {
  it('assigns increasing ids and clamps bad dimensions before spawning', () => {
    const m = new PtyManager()
    const id1 = m.create(opts(80, 24))
    const id2 = m.create({ cols: NaN as unknown as number, rows: 0 })
    expect(id1).toBe(1)
    expect(id2).toBe(2)
    const second = h.spawned[h.spawned.length - 1]
    expect(second.cols).toBe(80) // NaN -> fallback
    expect(second.rows).toBe(1) // 0 -> min
  })

  it('re-emits data and exit with the originating id', () => {
    const m = new PtyManager()
    const id = m.create(opts(80, 24))
    const proc = h.spawned[h.spawned.length - 1]
    const data: any[] = []
    const exits: any[] = []
    m.on('data', (e) => data.push(e))
    m.on('exit', (e) => exits.push(e))
    proc.dataCb('hello')
    proc.exitCb({ exitCode: 3, signal: 15 })
    expect(data).toEqual([{ id, data: 'hello' }])
    expect(exits).toEqual([{ id, exitCode: 3, signal: 15 }])
  })

  it('drops the session on exit so later writes do not reach the dead proc', () => {
    const m = new PtyManager()
    const id = m.create(opts(80, 24))
    const proc = h.spawned[h.spawned.length - 1]
    proc.exitCb({ exitCode: 0 })
    m.write(id, 'x') // session is gone -> no-op
    expect(proc.written).toEqual([])
  })

  it('kill is idempotent and removes the session', () => {
    const m = new PtyManager()
    const id = m.create(opts(80, 24))
    const proc = h.spawned[h.spawned.length - 1]
    m.kill(id)
    m.kill(id)
    expect(proc.killed).toBe(1)
  })

  it('swallows a throwing resize (resize-after-exit race)', () => {
    const m = new PtyManager()
    const id = m.create(opts(80, 24))
    const proc = h.spawned[h.spawned.length - 1]
    proc.resizeThrows = true
    expect(() => m.resize(id, 120, 40)).not.toThrow()
  })

  it('killAll kills every session and empties the map', () => {
    const m = new PtyManager()
    const a = m.create(opts(80, 24))
    const procA = h.spawned[h.spawned.length - 1]
    const b = m.create(opts(80, 24))
    const procB = h.spawned[h.spawned.length - 1]
    m.killAll()
    expect(procA.killed).toBe(1)
    expect(procB.killed).toBe(1)
    m.write(a, 'x')
    m.write(b, 'y')
    expect(procA.written).toEqual([])
    expect(procB.written).toEqual([])
  })
})
