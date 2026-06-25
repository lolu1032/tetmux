import { defineConfig } from 'vitest/config'

// The Tetris engine is pure TypeScript with no DOM dependency, so the node
// environment is enough. Tests live next to the code as *.test.ts.
export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
