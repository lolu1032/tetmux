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
      cols: Math.max(opts.cols | 0, 2),
      rows: Math.max(opts.rows | 0, 1),
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
      session.proc.resize(Math.max(cols | 0, 2), Math.max(rows | 0, 1))
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
