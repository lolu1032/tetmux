# tetmux — desktop edition

Run a command beside Tetris while you wait — now as a downloadable desktop app.

You start an AI agent (or a long build, or a test run) and then you wait. tetmux
splits the window: the **left** runs your commands through real PTYs — one or
more tmux-style windows that all run in parallel (Claude in one, Codex in
another, a build in a third) — and the **right** is a Tetris game. You fill the
dead time without leaving the screen, so the moment a command needs you, you are
already there.

This is the Electron rewrite of the original Go TUI: same idea, packaged as a
real desktop app (`.dmg` / `.exe` / `.AppImage` / `.deb`) with a GUI terminal
(xterm.js) instead of a terminal-inside-a-terminal.

```
┌ tetmux    + ┬───────────────────────────┬──────────────────────────┐
│ ● 1: zsh    │ $ claude                  │   ░░░░██░░░░   HOLD  [ ]  │
│   ~ · main  │ > building the parser...  │   ░░██████░░   NEXT  [J]  │
│ ◉ 2: claude │ ✶ thinking                │   ░░░░░░░░░░         [T]  │
│   app · fix │                           │   ██░░██░░██   SCORE 1200 │
│ ○ 3: build  │ (middle: your real shell) │   (right: Tetris)  LV 3   │
└─────────────┴───────────────────────────┴──────────────────────────┘
 win 2:claude (3) ● 2 waiting | focus:TERMINAL | score 1200 · best 3400 · lv 3 | playing
```

The left **sidebar** lists your windows with a status dot — `●` unread output,
`◉` bell, `○` exited — plus each window's directory and git branch. A background
window that needs you lights up (and the status bar counts how many are
`waiting`), so you know when to look up from Tetris.

## Architecture

| Layer | Tech | Responsibility |
|-------|------|----------------|
| Main process | Electron + `node-pty` | spawns/owns real PTYs, window lifecycle, IPC |
| Preload | `contextBridge` | safe `window.tetmux` API (no `nodeIntegration`) |
| Renderer | TypeScript + Vite | xterm.js terminals, canvas Tetris, layout, keybindings |

- `src/main` — `index.ts` (app + IPC), `pty-manager.ts` (pty sessions), `git.ts` (branch lookup)
- `src/preload` — the `window.tetmux` bridge
- `src/renderer/src/terminal` — `term-window.ts` (xterm wrapper + OSC 7 cwd), `terminal-area.ts` (sidebar), `window-activity.ts` (attention state, tested)
- `src/renderer/src/tetris` — `engine.ts` (pure, tested), `render.ts` (canvas), `game.ts` (loop + input)
- `src/renderer/src/app.ts` — split layout, focus routing · `status-bar.ts` — status segments
- `src/shared/ipc.ts` — the typed IPC contract shared by all three layers

## Develop

Requires Node 20+ and a C/C++ toolchain for `node-pty` (Xcode CLT on macOS).

```sh
cd desktop
pnpm install        # installs deps + rebuilds node-pty for Electron
pnpm dev            # launch the app with hot reload
pnpm typecheck      # tsc for main/preload + renderer
pnpm test           # vitest — Tetris engine unit tests
pnpm build          # production bundles into out/
```

## Package (downloadable installers)

```sh
pnpm package:mac    # → dist/tetmux-<version>-arm64.dmg  (+ .zip)
pnpm package:win    # → dist/tetmux Setup <version>.exe  (NSIS)
pnpm package:linux  # → dist/tetmux-<version>.AppImage   (+ .deb)
pnpm package        # current platform
```

Output lands in `dist/`. Packaging config is `electron-builder.yml`.

> Icons: drop `icon.icns` / `icon.ico` / `icon.png` (512×512) into `build/`.
> Without them electron-builder falls back to the default Electron icon.
> For notarized macOS builds, set `CSC_LINK` / `CSC_KEY_PASSWORD` and enable
> `hardenedRuntime` in `electron-builder.yml`.

## Controls

**Focus** — click a pane to focus it. The status bar shows `focus:TERMINAL` or
`focus:TETRIS`. While playing, `Tab` or `Esc` returns focus to the terminal.
Leaving the Tetris pane **auto-pauses** a running game (it resumes when you focus
it again) — so the board waits for you while you work. A pause you set yourself
with `p` is left paused until you resume it. **Double-click a window's title** in
the sidebar to rename it.

**Windows (tmux-style prefix `Ctrl+B`, then):**

| key | action | | key | action |
|-----|--------|-|-----|--------|
| `c` | new window (in the current dir) | | `n` / `p` | next / prev window |
| `1`–`9` | select window | | `&` / `x` | close window |
| `Space` | focus Tetris (play) | | `Tab` | toggle focus |
| `Ctrl+B` | send a literal `Ctrl+B` to the terminal | | | |

On macOS, `⌘T` new · `⌘W` close · `⌘1`–`9` select · `⌘[` / `⌘]` prev/next also work.

**Tetris (when focused):** `←`/`→` move (hold to auto-shift) · `↓` soft drop ·
`↑`/`x` rotate CW · `z` rotate CCW · `Space` hard drop · `c` hold · `p` pause ·
`r` restart (fresh board, any time) · `Enter` start / retry.
