import type { TetmuxApi } from '../../shared/ipc'

declare global {
  interface Window {
    tetmux: TetmuxApi
  }
}

export {}
