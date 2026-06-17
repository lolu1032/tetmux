# tetmux

Run a command beside Tetris while you wait.

You start an AI agent (or a long build, or a test run) in your terminal and then
you wait. 30 seconds here, two minutes there. tetmux splits the terminal: the
**left pane** runs your command through a real PTY, the **right pane** is a
Tetris game. You fill the dead time without leaving the screen, so the moment
the command needs you, you are already there.

It is the difference between a context-*destroying* distraction (picking up your
phone) and a context-*preserving* one (a game in the corner of the same screen).

```
┌───────────────────────────┬──────────────────────┐
│ $ claude                  │   . . . .[][]. . . . │
│ > building the parser...  │   . . .[][][]. . . . │
│ ✶ thinking                │   . . . . . . . . . . │
│                           │   [][]. .[]. .[][][] │
│ (left: your command)      │   (right: Tetris)    │
└───────────────────────────┴──────────────────────┘
 focus:LEFT | score:1200 | lines:6 | playing | C-b q quit, C-b h/l switch
```

## Install

Requires Go 1.26+.

```sh
go install tetmux@latest      # if published to a module path
# or, from a checkout:
go build -o tetmux . && mv tetmux ~/bin/   # or anywhere on $PATH
```

## Usage

```sh
tetmux                 # left pane runs your $SHELL
tetmux claude          # left pane runs claude
tetmux -- npm test     # use -- so the command's own flags aren't read by tetmux
```

`tetmux --help` and `tetmux --version` print help/version. Everything after a
`--` is treated as the command, so `tetmux -- mycmd --help` passes `--help` to
`mycmd`.

## Controls

tetmux uses a `Ctrl-b` **prefix**, like tmux. Press `Ctrl-b`, release, then the
next key is a tetmux command:

| Keys | Action |
|------|--------|
| `C-b h` / `C-b ←` | focus the left (command) pane |
| `C-b l` / `C-b →` | focus the right (Tetris) pane |
| `C-b q` | quit |
| `C-b C-b` | send a literal `Ctrl-b` to the focused pane |

Every other key goes to whichever pane is focused. With the **left** pane
focused you are typing into your command as normal. With the **right** pane
focused:

| Keys | Action |
|------|--------|
| `←` `→` / `h` `l` | move left / right |
| `↓` / `j` | soft drop |
| `↑` / `x` | rotate clockwise |
| `z` | rotate counter-clockwise |
| `space` | hard drop |
| `p` | pause / resume |
| `r` | restart after game over |

## How it works

- The left pane is a real PTY (`creack/pty`) parsed by a vt10x cell-grid
  terminal emulator, so full-screen / alt-screen programs (`vim`, `top`,
  `less`, an agent TUI) render correctly instead of as broken escape codes.
- The right pane is a self-contained Tetris (7-bag randomizer, SRS-lite wall
  kicks, standard 100/300/500/800 line scoring).
- Redraws are **push-based**: a reader goroutine notifies the UI only when the
  child actually emits output, and the gravity tick runs only while a piece is
  falling. An idle shell costs ~0 CPU.

The architecture keeps all game / routing / layout / render arithmetic in pure,
unit-tested packages (`internal/tetris`, `internal/router`, `internal/layout`,
`internal/vtrender`); `internal/app` is the bubbletea + PTY + vt10x glue.

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
