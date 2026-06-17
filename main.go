// Command tetmux is a tmux-style terminal multiplexer: the LEFT pane runs an
// arbitrary command (default $SHELL, or argv) through a PTY emulated by a
// vt10x cell grid, and the RIGHT pane is a self-rendered Tetris game so you can
// fill the dead time while waiting on a terminal AI agent.
//
// Controls: Ctrl-b is the prefix. Ctrl-b then h/l (or Left/Right arrow)
// switches focus; Ctrl-b then q quits; Ctrl-b twice sends one literal Ctrl-b
// to the focused pane. With the RIGHT pane focused: arrows/h/j/l move and
// soft-drop, up/x rotates CW, z rotates CCW, space hard-drops, p pauses,
// r restarts after game over.
package main

import (
	"fmt"
	"os"
	"os/signal"
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

With no command, tetmux runs your $SHELL in the left pane.

Examples:
  tetmux                 # left pane = $SHELL
  tetmux claude          # left pane = claude
  tetmux -- npm test     # use -- so flags go to the command, not tetmux

Controls:
  C-b h / C-b Left       focus the left (command) pane
  C-b l / C-b Right      focus the right (Tetris) pane
  C-b q                  quit
  C-b C-b                send a literal Ctrl-b to the focused pane
  Tetris (right focused): arrows/h j l move & soft-drop, up/x rotate CW,
                          z rotate CCW, space hard-drop, p pause, r restart

Flags:
  -h, --help             show this help
  --version              show version

Env:
  TETMUX_LOG=path        append debug logs (spawn errors, exit codes) to path
`

func main() {
	argv, action := parseArgs(os.Args[1:])
	switch action {
	case actionHelp:
		fmt.Print(usage)
		return
	case actionVersion:
		fmt.Println("tetmux", version)
		return
	}

	if logPath := os.Getenv("TETMUX_LOG"); logPath != "" {
		if f, err := tea.LogToFile(logPath, "tetmux"); err == nil {
			defer f.Close()
		}
	}

	seed := time.Now().UnixNano()
	model := app.New(argv, seed)

	p := tea.NewProgram(model, tea.WithAltScreen())
	model.SetSend(p.Send)

	// Ensure the PTY child is cleaned up on SIGTERM/SIGHUP so it is not
	// orphaned. (SIGINT/Ctrl-C is routed to the focused pane, not handled here.)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		model.Close()
		p.Kill()
	}()

	_, runErr := p.Run()
	signal.Stop(sigCh)
	model.Close()

	if se := model.SpawnErr(); se != nil {
		fmt.Fprintf(os.Stderr, "tetmux: failed to start %q: %v\n", leftName(argv), se)
		os.Exit(1)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "tetmux:", runErr)
		os.Exit(1)
	}
}

type action int

const (
	actionRun action = iota
	actionHelp
	actionVersion
)

// parseArgs interprets tetmux's own flags before the rest becomes the left-pane
// command. A leading "--" forces everything after it to be the command, so a
// command's own -h/--version is not swallowed by tetmux.
func parseArgs(args []string) ([]string, action) {
	if len(args) == 0 {
		return nil, actionRun
	}
	if args[0] == "--" {
		return args[1:], actionRun
	}
	switch args[0] {
	case "-h", "--help":
		return nil, actionHelp
	case "--version":
		return nil, actionVersion
	}
	return args, actionRun
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
