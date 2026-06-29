import { contextBridge, ipcRenderer } from 'electron'
import {
  IPC,
  type PtyCreateOptions,
  type PtyDataEvent,
  type PtyExitEvent,
  type TetmuxApi,
} from '../shared/ipc'

// Per-pty subscriber registries. A single ipcRenderer listener fans out to the
// handlers registered for that pty id, so the renderer never touches ipcRenderer
// directly (contextIsolation stays intact).
const dataHandlers = new Map<number, Set<(data: string) => void>>()
const exitHandlers = new Map<number, Set<(event: PtyExitEvent) => void>>()

ipcRenderer.on(IPC.ptyData, (_event, payload: PtyDataEvent) => {
  dataHandlers.get(payload.id)?.forEach((handler) => handler(payload.data))
})
ipcRenderer.on(IPC.ptyExit, (_event, payload: PtyExitEvent) => {
  exitHandlers.get(payload.id)?.forEach((handler) => handler(payload))
})

function subscribe<T>(registry: Map<number, Set<T>>, id: number, handler: T): () => void {
  let set = registry.get(id)
  if (!set) {
    set = new Set<T>()
    registry.set(id, set)
  }
  set.add(handler)
  return () => {
    const current = registry.get(id)
    if (!current) return
    current.delete(handler)
    if (current.size === 0) registry.delete(id)
  }
}

const api: TetmuxApi = {
  pty: {
    create: (opts: PtyCreateOptions) => ipcRenderer.invoke(IPC.ptyCreate, opts),
    write: (id, data) => ipcRenderer.send(IPC.ptyWrite, id, data),
    resize: (id, cols, rows) => ipcRenderer.send(IPC.ptyResize, id, cols, rows),
    kill: (id) => ipcRenderer.send(IPC.ptyKill, id),
    onData: (id, handler) => subscribe(dataHandlers, id, handler),
    onExit: (id, handler) => subscribe(exitHandlers, id, handler),
  },
  git: {
    branch: (cwd: string) => ipcRenderer.invoke(IPC.gitBranch, cwd),
  },
  platform: process.platform,
}

contextBridge.exposeInMainWorld('tetmux', api)
