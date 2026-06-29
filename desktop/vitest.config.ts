import { defineConfig } from 'vitest/config'

// Two test environments:
//  - node:  pure logic (Tetris engine, PtyManager) — fast, no DOM.
//  - dom:   renderer behaviour that touches the DOM (focus-pause, status bar).
//           These files are named *.dom.test.ts and run under happy-dom.
export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: 'node',
          environment: 'node',
          include: ['src/**/*.test.ts'],
          exclude: ['src/**/*.dom.test.ts'],
        },
      },
      {
        test: {
          name: 'dom',
          environment: 'happy-dom',
          include: ['src/**/*.dom.test.ts'],
        },
      },
    ],
  },
})
