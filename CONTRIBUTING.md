# Contributing to tetmux

Thanks for hacking on tetmux. It's a small Go TUI; the bar is "stays simple,
stays tested."

## Build & test

```sh
go build -o tetmux .
go test ./...                 # unit tests
go test -race ./...           # the concurrency-sensitive packages (app) too
go test -run '^$' -bench . -benchmem ./internal/vtrender/   # render hot path
```

CI runs build + vet + `go test -race -cover` on Go 1.24 and 1.26, plus the
vtrender benchmark and a guard that bans `term.Parse` in the read loop.

## Architecture & the one rule

All game / routing / layout / render arithmetic lives in **pure, TUI-free,
unit-tested packages**. `internal/app` is the only place allowed to touch
bubbletea, the PTY, or vt10x.

| Package | Responsibility | Deps |
|---------|----------------|------|
| `internal/tetris` | Pure game logic: board, 7-bag, pieces, hold, levels, scoring | none |
| `internal/router` | Pure key-routing state machine (prefix, focus, commands) | none |
| `internal/layout` | Pure split-pane geometry (pane widths, status row, too-small) | none |
| `internal/vtrender` | Render a VT cell grid to styled rows (cached ANSI SGR) | lipgloss/termenv |
| `internal/app` | bubbletea + PTY + vt10x glue (`Model`, panes, view) | everything |

**The rule:** if you can express logic without a terminal, put it in a pure
package and unit-test it. Keep `internal/app` thin glue.

## Gotchas worth knowing

- **Never use `term.Parse` in the PTY read loop** (`leftpane.go`). It holds the
  vt10x lock while blocking on the next read, which freezes the whole UI
  whenever the child is idle (an agent "thinking"). Use `term.Write` on chunks
  read outside the lock. CI greps for this.
- **vt10x colors:** values `[0,256)` are palette indices, `[256, 1<<24)` are a
  packed 24-bit truecolor (`r<<16|g<<8|b`), and `>= 1<<24` are default
  sentinels. `vtrender` down-converts truecolor through the active profile — do
  not pass a packed RGB value to termenv as a numeric color string.
- **Redraws are push-based and frame-gated.** The reader goroutine notifies the
  UI on output, coalesced to ~one redraw per frame; an idle child must stay at
  ~0 CPU (no polling timers).
- **The split divider has one clamp owner.** `layout.ComputeSplit` is the *only*
  place the Tetris board's `MinBoardWidth` floor is enforced; callers
  (`OffsetForDivider`, the mouse drag, the keyboard nudge) pass a raw offset and
  let `ComputeSplit` clamp. The model stores the *post-clamp* offset
  (`ClampedOffsetForDivider`) so the mouse and keyboard paths never desync. The
  mouse drag ends through the single `endDrag` chokepoint on release **and** every
  state transition (resize, window add/remove/select/cycle, game-over), so a lost
  release can't strand the divider glued to the cursor; `endDrag` flushes every
  window (incl. background sessions) to the final width.

## Style

- Match the surrounding code; keep files under ~600 lines.
- `gofmt` and `go vet` must be clean.
- New logic in a pure package needs a test. Bug fixes get a regression test.
