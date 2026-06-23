// Command tetmux runs a command (default $SHELL, e.g. claude) beside a Tetris
// game so you can fill the dead time while waiting on a terminal AI agent.
//
// It works out of the box with no extra software: the LEFT side is a cmux-style
// stack of command windows (each an arbitrary command through a PTY emulated by
// a vt10x cell grid), and the RIGHT pane is a self-rendered Tetris game. Tab
// toggles focus; window/meta actions use the Ctrl-b prefix.
//
// For a heavier setup you can offload the command pane to a real tmux session
// with --tmux (tmux must be installed); tetmux then only supplies the Tetris
// pane via --tetris-only.
//
// Game controls: arrows/h/j/l move and soft-drop, up/x rotate CW, z rotates CCW,
// space hard-drops, c holds, Esc/p pause (selectable Resume/Restart overlay),
// r restarts.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/app"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `tetmux - run a command beside Tetris while you wait

Usage:
  tetmux [command [args...]]
  tetmux -- command [args...]

tetmux runs the command in the LEFT pane and Tetris in the RIGHT pane. It needs
no extra software — just run it. With no command it runs your $SHELL.

Examples:
  tetmux                 # left pane = $SHELL
  tetmux claude          # left pane = claude
  tetmux -- npm test     # use -- so flags go to the command, not tetmux
  tetmux --tmux claude   # offload the command pane to a real tmux session

Controls:
  Mouse                  click a tab to switch windows, + to add one, a pane to
                         focus it; drag the divider between panes to resize live
  Tab                    toggle focus between the command pane and the game
  Game focused:          arrows/h j l move & soft-drop, up/x rotate CW,
                         z rotate CCW, space hard-drop, c hold, Esc/p pause,
                         r restart
  C-b c / n / p / x      new / next / prev / close command window
  C-b > / < / =          grow / shrink / reset the split
  C-b z                  cycle preset split ratios (2:1 / 1:1 / 1:2)
  C-b q                  quit

Flags:
  -h, --help             show this help
  --version              show version
  --tmux [command]       use a real tmux session for the command pane
                         (requires tmux; renders any TUI perfectly)
  --tetris-only          run just the Tetris pane (standalone game)

Env:
  TETMUX_LOG=path        append debug logs to path
  TETMUX_NO_IME_CURSOR=1 don't anchor the cursor to the command pane's input cell
                         (Korean/CJK IME composition is terminal-dependent; this is
                         the escape hatch — see the README)
`

func main() {
	p := parseArgs(os.Args[1:])
	switch p.action {
	case actionHelp:
		fmt.Print(usage)
		return
	case actionVersion:
		fmt.Println("tetmux", version)
		return
	case actionError:
		fmt.Fprintln(os.Stderr, "tetmux:", p.errMsg)
		os.Exit(2)
	case actionTetris:
		runTetris()
		return
	}

	if logPath := os.Getenv("TETMUX_LOG"); logPath != "" {
		if f, err := tea.LogToFile(logPath, "tetmux"); err == nil {
			defer f.Close()
		}
	}

	// Opt-in: delegate to a real tmux session (tmux renders the command pane).
	if p.useTmux {
		if err := launchViaTmux(p.argv); err != nil {
			if errors.Is(err, errTmuxNotFound) {
				fmt.Fprint(os.Stderr, tmuxMissingMsg)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "tetmux:", err)
			os.Exit(1)
		}
		return
	}

	// Default mode: the built-in multiplexer — no tmux, no install needed.
	runEmbedded(p.argv)
}

// tmuxMissingMsg is shown when --tmux was requested but tmux is not installed.
const tmuxMissingMsg = `tetmux: --tmux needs tmux installed, but it was not found.

  • install it:   brew install tmux      (macOS)   |   apt install tmux  (Debian/Ubuntu)
  • or just run without --tmux (the built-in mode needs nothing):

      tetmux claude
`

// runTetris runs the standalone Tetris TUI used by `tetmux --tetris-only` (the
// game pane of the tmux layout).
func runTetris() {
	model := app.NewTetris(time.Now().UnixNano())
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, runErr := p.Run()
	model.Close()
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "tetmux:", runErr)
		os.Exit(1)
	}
}

// runEmbedded runs the built-in multiplexer (left command pane via the vt10x
// emulator + right Tetris pane) — the fallback when tmux is not installed.
func runEmbedded(argv []string) {
	seed := time.Now().UnixNano()
	model := app.New(argv, seed)

	// WrapOutput re-homes the hardware cursor onto the focused command pane's
	// cursor cell after each frame so Korean/CJK IME composition renders inline
	// (otherwise bubbletea parks the cursor bottom-left and the preedit lags a
	// keystroke). The wrapper keeps stdout's tty identity so size detection works.
	// WithMouseCellMotion enables cell-level mouse tracking (mode 1002): the app
	// gets press/release plus motion reports ONLY while a button is held, which is
	// exactly what the divider drag-resize needs. Note the release encoding is
	// terminal-dependent — SGR terminals report it with Button=Left, X10 ones with
	// Button=None — so handleMouse must end a drag on any release regardless of the
	// button. Switching to WithMouseAllMotion (1003, motion without a button) would
	// require revisiting the e.Button / m.drag guards in handleMouse.
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithOutput(model.WrapOutput(os.Stdout)),
	)
	model.SetSend(p.Send)

	// Ensure the PTY child is cleaned up on SIGTERM/SIGHUP so it is not
	// orphaned. (SIGINT/Ctrl-C is routed to the focused pane, not handled here.)
	//
	// The handler only asks the program to quit (p.Quit); it must NOT call
	// model.Close() itself. Touching Model state (its windows slice, best score)
	// from this goroutine races the bubbletea Update goroutine that also mutates
	// them (new/close window, score tracking). Quitting lets p.Run() return, after
	// which model.Close() runs once below on the main goroutine — a single, safe
	// teardown caller.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		p.Quit()
	}()

	_, runErr := p.Run()
	signal.Stop(sigCh)
	// Capture the launched command's exit status BEFORE teardown kills children,
	// so a `tetmux -- npm test` that finished naturally can be propagated.
	exitCode, hasExitCode := model.PrimaryExitCode()
	model.Close()

	if se := model.SpawnErr(); se != nil {
		fmt.Fprintf(os.Stderr, "tetmux: failed to start %q: %v\n", leftName(argv), se)
		os.Exit(1)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "tetmux:", runErr)
		os.Exit(1)
	}
	// Propagate a non-zero command exit code so tetmux is usable in scripts/CI
	// (`tetmux -- npm test && deploy`, `$?` checks). A zero/absent code exits 0.
	if hasExitCode && exitCode != 0 {
		os.Exit(exitCode)
	}
}

type action int

const (
	actionRun action = iota
	actionHelp
	actionVersion
	// actionTetris runs only the Tetris pane (the game half of the tmux layout).
	actionTetris
	// actionError means the args were invalid; errMsg explains why.
	actionError
)

// parsed is the result of interpreting the command line.
type parsed struct {
	argv    []string // the command to run in the left pane (nil => $SHELL)
	action  action
	useTmux bool   // --tmux: offload the command pane to a real tmux session
	errMsg  string // set when action == actionError
}

// parseArgs interprets tetmux's own flags before the rest becomes the left-pane
// command. A leading "--" forces everything after it to be the command, so a
// command's own -h/--version is not swallowed by tetmux. An unknown leading
// flag (e.g. "--claude") is rejected with a hint rather than being run as a
// program, which produced a cryptic "executable file not found" error.
func parseArgs(args []string) parsed {
	if len(args) == 0 {
		return parsed{action: actionRun}
	}
	if args[0] == "--" {
		return parsed{argv: args[1:], action: actionRun}
	}
	switch args[0] {
	case "-h", "--help":
		return parsed{action: actionHelp}
	case "--version":
		return parsed{action: actionVersion}
	case "--tetris-only":
		return parsed{action: actionTetris}
	case "--tmux":
		return parsed{argv: args[1:], action: actionRun, useTmux: true}
	case "--built-in", "--builtin":
		// Explicit form of the default; accepted so it is never a confusing error.
		return parsed{argv: args[1:], action: actionRun}
	}
	// An unknown flag as the command is almost always a typo (e.g. "--claude"
	// instead of "claude"). Fail loudly with the likely fix.
	if strings.HasPrefix(args[0], "-") {
		cmd := strings.TrimLeft(args[0], "-")
		return parsed{action: actionError, errMsg: fmt.Sprintf(
			"unknown flag %q. Did you mean:  tetmux %s\n"+
				"(use 'tetmux -- %s' to pass it through as the command, or 'tetmux --help')",
			args[0], cmd, args[0])}
	}
	return parsed{argv: args, action: actionRun}
}

func leftName(argv []string) string {
	if len(argv) > 0 && argv[0] != "" {
		return argv[0]
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}
