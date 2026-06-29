import os from 'node:os'
import { EventEmitter } from 'node:events'
import * as pty from 'node-pty'
import type { PtyCreateOptions, PtyDataEvent, PtyExitEvent } from '../shared/ipc'

function defaultShell(): string {
  if (process.platform === 'win32') {
    return process.env.COMSPEC || 'powershell.exe'
  }
  return process.env.SHELL || '/bin/bash'
}

// No real terminal is anywhere near this wide/tall; an absurd value almost
// certainly means a bad/hostile resize message, so cap it.
const MAX_DIM = 2000

/**
 * Coerce a terminal dimension to a safe integer in [min, MAX_DIM]. `cols | 0`
 * turns NaN/undefined into 0 (and a pty with 0 columns misbehaves), so guard
 * explicitly: fall back for non-finite input and clamp huge finite values.
 */
export function clampDim(value: number, min: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback
  return Math.min(Math.max(Math.trunc(value), min), MAX_DIM)
}

interface PtySession {
  id: number
  proc: pty.IPty
}

/**
 * Owns every live pty. Emits `data` (PtyDataEvent) and `exit` (PtyExitEvent)
 * which the main process forwards to the renderer.
 */
export class PtyManager extends EventEmitter {
  private sessions = new Map<number, PtySession>()
  private nextId = 1

  create(opts: PtyCreateOptions): number {
    const id = this.nextId++
    const shell = opts.shell || defaultShell()
    const proc = pty.spawn(shell, [], {
      name: 'xterm-256color',
      cols: clampDim(opts.cols, 2, 80),
      rows: clampDim(opts.rows, 1, 24),
      cwd: opts.cwd || os.homedir(),
      env: {
        ...process.env,
        TERM: 'xterm-256color',
        COLORTERM: 'truecolor',
        TETMUX: '1',
      },
    })

    this.sessions.set(id, { id, proc })

    proc.onData((data) => {
      const event: PtyDataEvent = { id, data }
      this.emit('data', event)
    })
    proc.onExit(({ exitCode, signal }) => {
      const event: PtyExitEvent = { id, exitCode, signal }
      this.emit('exit', event)
      this.sessions.delete(id)
    })

    return id
  }

  write(id: number, data: string): void {
    this.sessions.get(id)?.proc.write(data)
  }

  resize(id: number, cols: number, rows: number): void {
    const session = this.sessions.get(id)
    if (!session) return
    try {
      session.proc.resize(clampDim(cols, 2, 80), clampDim(rows, 1, 24))
    } catch {
      // resize can throw if the pty exited between the renderer's fit and here.
    }
  }

  kill(id: number): void {
    const session = this.sessions.get(id)
    if (!session) return
    try {
      session.proc.kill()
    } catch {
      // already gone
    }
    this.sessions.delete(id)
  }

  killAll(): void {
    for (const id of [...this.sessions.keys()]) this.kill(id)
  }
}
