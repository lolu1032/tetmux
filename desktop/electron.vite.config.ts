import { resolve } from 'node:path'
import { defineConfig, externalizeDepsPlugin } from 'electron-vite'

// node-pty is a native module and the @xterm/* packages are bundled into the
// renderer by Vite. externalizeDepsPlugin keeps node-pty out of the main/preload
// bundles so Electron loads the rebuilt native addon at runtime.
export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
  },
  renderer: {
    root: 'src/renderer',
    build: {
      rollupOptions: {
        input: {
          index: resolve(__dirname, 'src/renderer/index.html'),
        },
      },
    },
  },
})
