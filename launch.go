package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// errTmuxNotFound signals that the tmux binary is unavailable, so the caller can
// fall back to the built-in embedded multiplexer.
var errTmuxNotFound = errors.New("tmux not found")

// tetrisPaneCols is the column width handed to the Tetris pane. It comfortably
// fits the 20-column board plus the HOLD/NEXT side panel and borders; the
// command pane takes the rest of the terminal.
const tetrisPaneCols = 40

// launchViaTmux runs the user's command beside the Tetris pane using a real tmux
// session, so tmux's battle-tested terminal emulator renders the command (e.g.
// claude) — tetmux only supplies the game via `--tetris-only`. It returns
// errTmuxNotFound when tmux is absent so main can fall back to embedded mode.
//
// Layout: the command runs in the LEFT pane and `tetmux --tetris-only` in a
// fixed-width RIGHT pane. Outside tmux a fresh session is created and attached;
// inside tmux (a nested invocation) a new window is opened in the current
// session instead, since a client cannot attach to itself.
func launchViaTmux(argv []string) error {
	tmux, err := lookPath("tmux")
	if err != nil {
		return errTmuxNotFound
	}
	self, err := os.Executable()
	if err != nil || self == "" {
		self = os.Args[0]
	}

	left := argv
	if len(left) == 0 {
		left = []string{shellPath()}
	}

	if insideTmux() {
		return runTmuxWindow(tmux, self, left)
	}
	return runTmuxSession(tmux, self, left)
}

// insideTmux reports whether we are already running inside a tmux client.
func insideTmux() bool { return os.Getenv("TMUX") != "" }

// shellPath returns the user's $SHELL, defaulting to /bin/sh.
func shellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// runTmuxSession creates a detached session running the command, splits in the
// Tetris pane, focuses the command pane, then attaches (blocking until exit).
func runTmuxSession(tmux, self string, left []string) error {
	sid, err := tmuxCapture(tmux, tmuxNewSessionArgs(left))
	if err != nil {
		return err
	}
	if err := tmuxRun(tmux, tmuxSplitArgs(sid, self, tetrisPaneCols)); err != nil {
		return err
	}
	_ = tmuxRun(tmux, []string{"select-pane", "-t", sid, "-L"})
	return tmuxAttach(tmux, []string{"attach-session", "-t", sid})
}

// runTmuxWindow (nested case) opens a new window in the current session with the
// same command|Tetris split and switches to it; no attach is needed.
func runTmuxWindow(tmux, self string, left []string) error {
	wid, err := tmuxCapture(tmux, tmuxNewWindowArgs(left))
	if err != nil {
		return err
	}
	if err := tmuxRun(tmux, tmuxSplitArgs(wid, self, tetrisPaneCols)); err != nil {
		return err
	}
	return tmuxRun(tmux, []string{"select-pane", "-t", wid, "-L"})
}

// tmuxNewSessionArgs builds a detached `new-session` that prints the new session
// id and runs the command (after `--` so its own flags are untouched).
func tmuxNewSessionArgs(left []string) []string {
	a := []string{"new-session", "-d", "-P", "-F", "#{session_id}", "--"}
	return append(a, left...)
}

// tmuxNewWindowArgs is the nested-session analogue, printing the new window id.
func tmuxNewWindowArgs(left []string) []string {
	a := []string{"new-window", "-P", "-F", "#{window_id}", "--"}
	return append(a, left...)
}

// tmuxSplitArgs builds a horizontal split that runs `self --tetris-only` in a
// new fixed-width pane to the right of target.
func tmuxSplitArgs(target, self string, cols int) []string {
	return []string{"split-window", "-h", "-t", target, "-l", strconv.Itoa(cols), "--", self, "--tetris-only"}
}

// The exec seam. These are package vars (not plain funcs) so tests can override
// them to assert the exact tmux orchestration — LookPath failure, the
// new-session→split→select-pane→attach sequence, and the inside-tmux window
// variant — without a real tmux binary on the runner.
var (
	// lookPath resolves the tmux binary (overridable to simulate "not installed").
	lookPath = exec.LookPath

	// tmuxCapture runs tmux with args and returns trimmed stdout (e.g. a session id).
	tmuxCapture = func(tmux string, args []string) (string, error) {
		out, err := exec.Command(tmux, args...).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}

	// tmuxRun runs a tmux control command, discarding output.
	tmuxRun = func(tmux string, args []string) error {
		return exec.Command(tmux, args...).Run()
	}

	// tmuxAttach runs tmux attached to the real terminal (inherited stdio); it
	// blocks until the user detaches or the session ends.
	tmuxAttach = func(tmux string, args []string) error {
		cmd := exec.Command(tmux, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
)
