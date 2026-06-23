# tetmux

Run a command beside Tetris while you wait.

You start an AI agent (or a long build, or a test run) in your terminal and then
you wait. 30 seconds here, two minutes there. tetmux splits the terminal: the
**left side** runs your commands through real PTYs — one or more cmux-style
windows that all run in parallel (Claude in one, Codex in another, a build in a
third) — and the **right pane** is a Tetris game. You fill the dead time without
leaving the screen, so the moment a command needs you, you are already there.

It is the difference between a context-*destroying* distraction (picking up your
phone) and a context-*preserving* one (a game in the corner of the same screen).

```
 1:claude*  2:zsh  +                          ← click a tab / + (mouse)
┌───────────────────────────┬──────────────────────┐
│ $ claude                  │   · · · ·████· · · · │
│ > building the parser...  │   · · ·██████· · · · │
│ ✶ thinking                │   · · · · · · · · · · │
│                           │   ████· ·██· ·██████ │
│ (left: your command)      │   (right: Tetris)    │
└───────────────────────────┴──────────────────────┘
 focus:LEFT | score:1200 best:3400 lv:0 | playing | Tab/click:play | C-b z:ratio C-b q:quit
```

## Install

Requires Go 1.24+. Build from a checkout:

```sh
git clone <this repo> && cd tetmux
go build -o tetmux .          # then run ./tetmux
# optionally put it on $PATH:
go build -o "$(go env GOPATH)/bin/tetmux" .
```

(The module is named `tetmux` locally, so `go install tetmux@latest` does not
work until it is published under a real VCS path like `github.com/<owner>/tetmux`.)

## Usage

```sh
tetmux                 # left pane runs your $SHELL
tetmux claude          # left pane runs claude
tetmux -- npm test     # use -- so the command's own flags aren't read by tetmux
```

`tetmux --help` and `tetmux --version` print help/version. Everything after a
`--` is treated as the command, so `tetmux -- mycmd --help` passes `--help` to
`mycmd`.

When you run a command with `--`, tetmux exits with that command's own exit code
once you quit, so `tetmux -- npm test` is usable in scripts and `&&` chains.

## Modes

By default tetmux runs its **built-in multiplexer** — the left command pane is a
vt10x-emulated PTY, so it works out of the box with no extra software. Two opt-in
modes:

- `tetmux --tmux [command]` delegates the command pane to a **real tmux session**
  (tmux renders any TUI perfectly); tetmux supplies only the Tetris pane via
  `--tetris-only`. Needs `tmux` installed — without it tetmux tells you how to
  install it, or just drop `--tmux`.
- `tetmux --tetris-only` runs just the standalone Tetris pane (the game half of
  the `--tmux` layout).

In `--tmux` mode the command pane is a real tmux pane, so tmux's own keybindings
apply there; the `Ctrl-b` controls below describe the **built-in** mode.

## Controls

**Mouse** (cmux-style): the top row is a **tab bar** — click a window tab to
switch to it, click **`+`** to open a new window, and click a pane to focus it.
**Drag the divider** between the two panes (grab the boundary column and drag) to
resize the split live — the command pane reflows as you drag, and the Tetris board
is never crushed below its minimum width. No keyboard prefix needed.

**`Tab` toggles focus** between the command pane and the game — one key to jump
between typing and playing. The focused pane has a bright border.

When the **game** is focused, every key plays directly (no prefix):

| Keys | Action |
|------|--------|
| `Tab` | back to the command pane |
| `←` `→` / `h` `l` | move left / right |
| `↓` / `j` | soft drop |
| `↑` / `x` | rotate clockwise |
| `z` | rotate counter-clockwise |
| `space` | hard drop |
| `c` | hold / swap the current piece |
| `Esc` / `p` | pause — opens a 계속하기 / 재시작 menu (↑↓ to move, `Enter` to pick; `Esc`/`p` quick-resume) |
| `r` | restart (new board, any time) |

After a loss, a game-over overlay shows your final score and best — press `Enter`
or `r` to start a new game.

When the **command** pane is focused you type into your program as normal,
including `Esc` (so claude's Esc-to-interrupt still works). `Tab` switches to
the game; for a literal Tab (shell completion) use `C-b Tab`.

Window/meta actions use a `Ctrl-b` **prefix**, like tmux — press `Ctrl-b`,
release, then:

| Keys | Action |
|------|--------|
| `C-b c` | new command window (cmux-style — runs `$SHELL`) |
| `C-b n` / `C-b p` | next / previous window |
| `C-b 1`…`9` | jump to window N |
| `C-b x` | close the current window |
| `C-b >` / `C-b <` | grow / shrink the left pane (aliases: `.` / `,`) |
| `C-b =` | reset the panes to an even split |
| `C-b z` | cycle preset split ratios (2:1 / 1:1 / 1:2) |
| `C-b q` | quit |
| `C-b l` / `C-b h` | focus right / left (alternative to `Tab`) |
| `C-b Tab` | send a literal `Tab` to the focused pane (shell completion) |
| `C-b C-b` | send a literal `Ctrl-b` to the focused pane |

### Multiple windows (cmux-style)

The left side is a stack of **command windows** that all run in parallel — one
shown at a time, the rest live in the background. The **top tab bar** lists them
(e.g. `1:zsh  2:claude  3:npm`, the active one highlighted) with a `+` button.

- **Mouse:** click a tab to switch, click `+` to add a window.
- **Keyboard:** `C-b c` new, `C-b n`/`C-b p` and `C-b 1`…`9` switch, `C-b x` close.

Run a different agent in each — Claude in one, Codex in another — and keep
playing Tetris on the right while they all work.

## How it works

- Each command window is a real PTY (`creack/pty`) parsed by a vt10x cell-grid
  terminal emulator, so full-screen / alt-screen programs (`vim`, `top`,
  `less`, an agent TUI) render correctly instead of as broken escape codes.
  Windows run in parallel; only the active one is drawn, and 24-bit truecolor
  output is down-converted to the terminal's profile so it never corrupts.
- The right pane is a self-contained Tetris (7-bag randomizer, SRS-lite wall
  kicks, standard 100/300/500/800 line scoring) with **HOLD**, a **ghost**
  landing outline, **next-piece preview**, and **level-based speed-up** (gravity
  accelerates every 10 lines). The HOLD/NEXT side panel appears when the pane is
  wide enough. The best score persists across runs (under `$XDG_STATE_HOME`).
- Redraws are **push-based**: a reader goroutine notifies the UI only when the
  child actually emits output, and the gravity tick runs only while a piece is
  falling. An idle shell costs ~0 CPU.

The architecture keeps all game / routing / layout / render arithmetic in pure,
unit-tested packages (`internal/tetris`, `internal/router`, `internal/layout`,
`internal/vtrender`); `internal/app` is the bubbletea + PTY + vt10x glue.

## Korean / CJK input (IME)

Composing Korean/CJK in a full-screen TUI (an agent's input box) is handled by your
**terminal**, not by tetmux: the in-progress 조합 글자 (preedit) is an OS overlay the
terminal draws at the cursor. tetmux anchors the real cursor on the command pane's
input cell (like tmux does) so a capable terminal composes inline — but where the
preedit actually appears is the terminal's decision.

- **Composes inline:** iTerm2, Ghostty, WezTerm, Kitty.
- **Apple Terminal** mis-places the preedit in full-screen apps — a known
  Terminal.app limitation (it happens with the agent in a plain terminal too, not
  just tetmux). For heavy Korean input use one of the terminals above.
- `TETMUX_NO_IME_CURSOR=1` disables the cursor anchoring and hands the cursor back to
  the default behavior, if you prefer it.

(Committed Korean/CJK text always renders correctly regardless of terminal — only the
live IME composition overlay is terminal-dependent.)

## Caveats

- If the terminal is too small to fit the 10×20 board, the right pane shows
  "window too small" and preserves your game state until you resize back.
- If the left command fails to start (typo, not on `$PATH`), the left pane shows
  the error and tetmux exits non-zero after you quit.
- Set `TETMUX_LOG=/path/to/log` to append debug logs (spawn errors, exit codes).

## Development

```sh
go build ./...
go vet ./...
go test -race ./...
```

## License

MIT. See [LICENSE](LICENSE).
