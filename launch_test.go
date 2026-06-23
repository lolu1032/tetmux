package main

import (
	"strings"
	"testing"
)

func TestParseArgsTetrisOnly(t *testing.T) {
	if p := parseArgs([]string{"--tetris-only"}); p.action != actionTetris {
		t.Errorf("--tetris-only should select actionTetris, got %v", p.action)
	}
	// A plain command runs as-is.
	if p := parseArgs([]string{"claude"}); p.action != actionRun || len(p.argv) != 1 || p.argv[0] != "claude" {
		t.Errorf("plain command should be actionRun with argv: %+v", p)
	}
	if p := parseArgs([]string{"--", "npm", "test"}); p.action != actionRun || strings.Join(p.argv, " ") != "npm test" {
		t.Errorf("-- should pass the rest through: %+v", p)
	}
}

// An unknown leading flag (the common "--claude" typo) is rejected with a hint,
// not run as a program (which gave a cryptic "executable not found").
func TestParseArgsUnknownFlagRejected(t *testing.T) {
	p := parseArgs([]string{"--claude"})
	if p.action != actionError {
		t.Fatalf("--claude should be actionError, got %v", p.action)
	}
	if !strings.Contains(p.errMsg, "tetmux claude") {
		t.Errorf("error should suggest 'tetmux claude', got: %s", p.errMsg)
	}
}

// The default (no flag) is the built-in multiplexer — no tmux.
func TestParseArgsDefaultIsBuiltin(t *testing.T) {
	if p := parseArgs([]string{"claude"}); p.useTmux {
		t.Errorf("default should NOT use tmux, got %+v", p)
	}
	if p := parseArgs(nil); p.action != actionRun || p.useTmux {
		t.Errorf("no args should run $SHELL in built-in mode, got %+v", p)
	}
}

// --tmux opts into the tmux session and passes the rest as the command.
func TestParseArgsTmuxOptIn(t *testing.T) {
	p := parseArgs([]string{"--tmux", "claude"})
	if p.action != actionRun || !p.useTmux {
		t.Fatalf("--tmux should be actionRun+useTmux, got %+v", p)
	}
	if len(p.argv) != 1 || p.argv[0] != "claude" {
		t.Errorf("--tmux should pass the command through, got argv=%v", p.argv)
	}
}

func TestTmuxNewSessionArgs(t *testing.T) {
	got := tmuxNewSessionArgs([]string{"claude", "--flag"})
	want := []string{"new-session", "-d", "-P", "-F", "#{session_id}", "--", "claude", "--flag"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("new-session args = %v\nwant %v", got, want)
	}
}

func TestTmuxSplitArgsRunsTetrisOnly(t *testing.T) {
	got := tmuxSplitArgs("$3", "/usr/local/bin/tetmux", 40)
	joined := strings.Join(got, " ")
	// Right pane, targeted at the session, fixed width, running self --tetris-only.
	for _, want := range []string{"split-window", "-h", "-t $3", "-l 40", "-- /usr/local/bin/tetmux --tetris-only"} {
		if !strings.Contains(joined, want) {
			t.Errorf("split args missing %q: %v", want, got)
		}
	}
}

func TestShellPathFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := shellPath(); got != "/bin/sh" {
		t.Errorf("empty $SHELL should fall back to /bin/sh, got %q", got)
	}
	t.Setenv("SHELL", "/bin/zsh")
	if got := shellPath(); got != "/bin/zsh" {
		t.Errorf("shellPath should honor $SHELL, got %q", got)
	}
}

func TestTmuxNewWindowArgs(t *testing.T) {
	got := tmuxNewWindowArgs([]string{"claude", "--flag"})
	want := []string{"new-window", "-P", "-F", "#{window_id}", "--", "claude", "--flag"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("new-window args = %v\nwant %v", got, want)
	}
}

func TestLeftName(t *testing.T) {
	if got := leftName([]string{"claude", "x"}); got != "claude" {
		t.Errorf("leftName(argv) = %q want claude", got)
	}
	t.Setenv("SHELL", "/bin/zsh")
	if got := leftName(nil); got != "/bin/zsh" {
		t.Errorf("leftName(nil) = %q want /bin/zsh", got)
	}
	t.Setenv("SHELL", "")
	if got := leftName(nil); got != "/bin/sh" {
		t.Errorf("leftName(nil) empty SHELL = %q want /bin/sh", got)
	}
}

// When tmux can't be found, launchViaTmux reports errTmuxNotFound so main can
// print the install hint / fall back — without spawning anything.
func TestLaunchViaTmuxNotFound(t *testing.T) {
	restore := stubTmux(t)
	defer restore()
	lookPath = func(string) (string, error) { return "", errTmuxNotFound }
	if err := launchViaTmux([]string{"claude"}); err != errTmuxNotFound {
		t.Errorf("missing tmux should yield errTmuxNotFound, got %v", err)
	}
}

// Outside tmux: new detached session → split the Tetris pane → focus left →
// attach. The exact sequence and order are asserted via the recorded calls.
func TestLaunchViaTmuxSessionSequence(t *testing.T) {
	restore := stubTmux(t)
	defer restore()
	t.Setenv("TMUX", "") // force the not-inside-tmux path

	var calls []string
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	tmuxCapture = func(_ string, args []string) (string, error) {
		calls = append(calls, "capture:"+strings.Join(args, " "))
		return "$7", nil // pretend session id
	}
	tmuxRun = func(_ string, args []string) error {
		calls = append(calls, "run:"+strings.Join(args, " "))
		return nil
	}
	attached := false
	tmuxAttach = func(_ string, args []string) error {
		attached = true
		calls = append(calls, "attach:"+strings.Join(args, " "))
		return nil
	}

	if err := launchViaTmux([]string{"claude"}); err != nil {
		t.Fatalf("launchViaTmux: %v", err)
	}
	if !attached {
		t.Error("outside tmux must attach to the new session")
	}
	joined := strings.Join(calls, " | ")
	for _, want := range []string{
		"capture:new-session", "run:split-window", "run:select-pane -t $7 -L", "attach:attach-session -t $7",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("sequence missing %q:\n%s", want, joined)
		}
	}
	// new-session must come before split, which comes before attach.
	if i, j, k := idx(calls, "capture:new"), idx(calls, "run:split"), idx(calls, "attach:"); !(i < j && j < k) {
		t.Errorf("out-of-order sequence: new=%d split=%d attach=%d (%s)", i, j, k, joined)
	}
}

// Inside tmux (TMUX set): open a new window in the current session and split it —
// but never attach (a client can't attach to itself).
func TestLaunchViaTmuxWindowSequenceNoAttach(t *testing.T) {
	restore := stubTmux(t)
	defer restore()
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0") // force the inside-tmux path

	var calls []string
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	tmuxCapture = func(_ string, args []string) (string, error) {
		calls = append(calls, "capture:"+strings.Join(args, " "))
		return "@3", nil // pretend window id
	}
	tmuxRun = func(_ string, args []string) error {
		calls = append(calls, "run:"+strings.Join(args, " "))
		return nil
	}
	tmuxAttach = func(_ string, _ []string) error {
		t.Fatal("inside tmux must NOT attach (cannot attach to self)")
		return nil
	}

	if err := launchViaTmux([]string{"claude"}); err != nil {
		t.Fatalf("launchViaTmux: %v", err)
	}
	joined := strings.Join(calls, " | ")
	if !strings.Contains(joined, "capture:new-window") {
		t.Errorf("inside tmux should open a new-window: %s", joined)
	}
	if !strings.Contains(joined, "run:select-pane -t @3 -L") {
		t.Errorf("inside tmux should focus the left pane: %s", joined)
	}
}

// idx returns the index of the first call with the given prefix, or -1.
func idx(calls []string, prefix string) int {
	for i, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

// stubTmux snapshots the exec seam and returns a restore func, so each test can
// override the vars without leaking into others.
func stubTmux(t *testing.T) func() {
	t.Helper()
	oLook, oCap, oRun, oAtt := lookPath, tmuxCapture, tmuxRun, tmuxAttach
	return func() { lookPath, tmuxCapture, tmuxRun, tmuxAttach = oLook, oCap, oRun, oAtt }
}
