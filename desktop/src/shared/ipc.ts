// Shared contract between the main, preload and renderer processes.
// Keep this dependency-free so every layer can import it.

export interface PtyCreateOptions {
  cols: number
  rows: number
  cwd?: string
  shell?: string
  name?: string
}

export interface PtyDataEvent {
  id: number
  data: string
}

export interface PtyExitEvent {
  id: number
  exitCode: number
  signal?: number
}

// IPC channel names. Centralised so main/preload never drift apart.
export const IPC = {
  ptyCreate: 'pty:create',
  ptyWrite: 'pty:write',
  ptyResize: 'pty:resize',
  ptyKill: 'pty:kill',
  ptyData: 'pty:data',
  ptyExit: 'pty:exit',
} as const

// The surface exposed on `window.tetmux` by the preload bridge.
export interface TetmuxPtyApi {
  create(opts: PtyCreateOptions): Promise<number>
  write(id: number, data: string): void
  resize(id: number, cols: number, rows: number): void
  kill(id: number): void
  /** Subscribe to output for one pty. Returns an unsubscribe function. */
  onData(id: number, handler: (data: string) => void): () => void
  /** Subscribe to the exit event for one pty. Returns an unsubscribe function. */
  onExit(id: number, handler: (event: PtyExitEvent) => void): () => void
}

export interface TetmuxApi {
  pty: TetmuxPtyApi
  platform: string
}
