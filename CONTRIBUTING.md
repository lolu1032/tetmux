# Contributing to tetmux

Thanks for wanting to help! Please read this before opening a pull request.

## Contributor License Agreement (required)

By submitting a contribution (a pull request, patch, or any code/content) to this
project, **you agree to the [Contributor License Agreement](CLA.md)**.

In short: you keep authorship of your work, but you grant the maintainer a broad
license — **including the right to relicense it commercially** — so the project can
keep its noncommercial-by-default, commercial-on-request model (see
[LICENSE](LICENSE)). If you can't agree to that, please don't submit a
contribution.

## Project layout

The app lives in [`desktop/`](desktop/) — Electron (main/preload) + a TypeScript
renderer (xterm.js terminals + a canvas Tetris). See
[`desktop/README.md`](desktop/README.md) for the architecture.

```sh
cd desktop
pnpm install      # deps + rebuild node-pty for Electron
pnpm dev          # run with hot reload
pnpm test         # vitest
pnpm typecheck    # tsc (main/preload + renderer)
pnpm build        # production bundle
```

## Pull requests

- Keep changes focused, and match the surrounding style and comment density.
- Add or adjust tests for any behavior change (`*.test.ts`, `*.dom.test.ts`).
- Run `pnpm test` and `pnpm typecheck` locally — both must be green.
- Describe the change and how you verified it.

## Reporting bugs / ideas

Open an issue with clear steps to reproduce, and your OS / terminal if relevant.
