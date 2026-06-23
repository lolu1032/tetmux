// Package router implements the pure key-routing state machine for tetmux.
// It decides, for an incoming key, whether the key goes to the LEFT pane (the
// PTY child), the RIGHT pane (Tetris), or is consumed by tetmux itself as a
// prefix command. It has no TUI dependencies so it is fully unit-testable.
package router

// Focus identifies which pane currently receives input.
type Focus int

const (
	// FocusLeft routes ordinary keys to the PTY child.
	FocusLeft Focus = iota
	// FocusRight routes ordinary keys to the Tetris game.
	FocusRight
)

// RouteTo identifies the destination of a key for this event.
type RouteTo int

const (
	// RouteNone means the key was consumed by tetmux (no pane sees it).
	RouteNone RouteTo = iota
	// RouteLeft means deliver the key to the PTY child.
	RouteLeft
	// RouteRight means deliver the key to the Tetris game.
	RouteRight
)

// Command is a tetmux-level action triggered by the prefix.
type Command int

const (
	// CmdNone means no tetmux command.
	CmdNone Command = iota
	// CmdSwitchFocusLeft focuses the left pane.
	CmdSwitchFocusLeft
	// CmdSwitchFocusRight focuses the right pane.
	CmdSwitchFocusRight
	// CmdQuit quits the whole program.
	CmdQuit
	// CmdSplitGrowLeft moves the divider right: the left (command) pane grows
	// and the right (Tetris) pane shrinks.
	CmdSplitGrowLeft
	// CmdSplitShrinkLeft moves the divider left: the left (command) pane
	// shrinks and the right (Tetris) pane grows.
	CmdSplitShrinkLeft
	// CmdSplitReset restores the even 50/50 split.
	CmdSplitReset
	// CmdSplitRatioCycle cycles the divider through preset left:right ratios
	// (2:1 -> 1:1 -> 1:2). Unlike the > / < nudge it is resize-stable.
	CmdSplitRatioCycle
	// CmdToggleFocus flips focus between the two panes. It is the Tab shortcut:
	// one key to jump between typing in the command pane and playing Tetris.
	CmdToggleFocus
	// CmdNewWindow opens a new command window (cmux-style parallel session).
	CmdNewWindow
	// CmdNextWindow / CmdPrevWindow cycle the visible command window.
	CmdNextWindow
	CmdPrevWindow
	// CmdCloseWindow closes the current command window.
	CmdCloseWindow
	// CmdSelectWindow jumps to the window numbered in Action.Arg (1-based).
	CmdSelectWindow
)

// PrefixByte is the Ctrl-b control byte (0x02) that arms the prefix.
const PrefixByte = 0x02

// KeyType classifies an incoming key so the state machine can treat special
// keys (arrows) the same as their letter equivalents.
type KeyType int

const (
	// KeyRune is an ordinary printable/character key carrying a Rune.
	KeyRune KeyType = iota
	// KeyCtrlB is the Ctrl-b control key.
	KeyCtrlB
	// KeyTab is the Tab key, used as the one-press focus toggle.
	KeyTab
	// KeyEsc is the Escape key. It routes to the focused pane (the game treats
	// it as pause/resume; the command pane receives a literal Esc).
	KeyEsc
	// KeyCtrlC is Ctrl-C. With the game focused it quits tetmux (back to the
	// host terminal); with the command pane focused it is forwarded to the
	// child as a literal interrupt (0x03).
	KeyCtrlC
	// KeyLeft / KeyRight / KeyUp / KeyDown are arrow keys.
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	// KeyOther is any other special key (enter, tab, function keys...).
	KeyOther
)

// Key is a normalized incoming key. The glue layer maps bubbletea key messages
// onto this struct. Bytes holds the raw bytes to forward to the PTY when the
// key routes left.
type Key struct {
	Type  KeyType
	Rune  rune
	Bytes []byte
}

// State is the router's mutable state: which pane is focused and whether the
// Ctrl-b prefix is currently armed (waiting for the next key).
type State struct {
	Focus       Focus
	PrefixArmed bool
}

// Action is the result of routing one key: where (if anywhere) to send the
// key, which tetmux command (if any) to run, the bytes to forward when routing
// to the PTY, and the resulting State.
type Action struct {
	RouteTo  RouteTo
	Command  Command
	Bytes    []byte
	Arg      int // command parameter, e.g. the 1-based window number for CmdSelectWindow
	NewState State
}

// Route is the pure routing function. Given the current state and an incoming
// key, it returns the Action describing what to do and the next state.
//
// Rules (per spec):
//   - prefix disarmed + Tab               => toggle focus (CmdToggleFocus). One
//     key to jump between the command pane and the game.
//   - prefix disarmed + Esc               => route to focused pane (the game
//     reads it as pause/resume; the command pane gets a literal Esc).
//   - prefix disarmed + Ctrl-c, game      => CmdQuit (exit to host terminal).
//   - prefix disarmed + Ctrl-c, command   => forward 0x03 to the child.
//   - prefix disarmed + Ctrl-b            => arm prefix, consume key.
//   - prefix disarmed + any other key     => route to focused pane.
//   - prefix armed   + Ctrl-b             => send one literal Ctrl-b, disarm.
//   - prefix armed   + Tab                => send one literal Tab, disarm.
//   - prefix armed   + 'l' / RightArrow   => CmdSwitchFocusRight, disarm.
//   - prefix armed   + 'h' / LeftArrow    => CmdSwitchFocusLeft, disarm.
//   - prefix armed   + 'q'                => CmdQuit, disarm.
//   - prefix armed   + > < . , =          => pane resize, disarm.
//   - prefix armed   + z                   => cycle preset split ratio, disarm.
//   - prefix armed   + c n p x 1-9        => window new/next/prev/close/select.
//   - prefix armed   + unknown key        => disarm, swallow.
func Route(st State, key Key) Action {
	if st.PrefixArmed {
		return routeArmed(st, key)
	}

	// Prefix disarmed. Tab toggles focus; Esc falls through to the focused pane
	// (handled as pause by the game, delivered literally to the command pane).
	if key.Type == KeyTab {
		ns := st
		ns.Focus = otherFocus(st.Focus)
		return Action{RouteTo: RouteNone, Command: CmdToggleFocus, NewState: ns}
	}
	if key.Type == KeyCtrlB {
		ns := st
		ns.PrefixArmed = true
		return Action{RouteTo: RouteNone, Command: CmdNone, NewState: ns}
	}
	// Ctrl-C with the game focused quits tetmux (exits to the host terminal);
	// with the command pane focused it is forwarded to the child so you can
	// still interrupt a running command.
	if key.Type == KeyCtrlC {
		if st.Focus == FocusRight {
			return Action{RouteTo: RouteNone, Command: CmdQuit, NewState: st}
		}
		return routeToFocus(st, key)
	}
	return routeToFocus(st, key)
}

// otherFocus returns the opposite pane.
func otherFocus(f Focus) Focus {
	if f == FocusLeft {
		return FocusRight
	}
	return FocusLeft
}

// routeArmed handles a key while the prefix is armed. The prefix is always
// disarmed in the resulting state.
func routeArmed(st State, key Key) Action {
	ns := st
	ns.PrefixArmed = false

	// Literal prefix: Ctrl-b twice sends one Ctrl-b to the focused pane.
	if key.Type == KeyCtrlB {
		return routeBytesToFocus(ns, []byte{PrefixByte})
	}

	// Literal Tab: since a bare Tab toggles focus, C-b Tab is the escape hatch
	// that delivers a real Tab to the focused pane (e.g. shell completion).
	if key.Type == KeyTab {
		return routeBytesToFocus(ns, []byte{'\t'})
	}

	// Focus switches via h/l or arrow keys.
	if key.Type == KeyLeft || (key.Type == KeyRune && key.Rune == 'h') {
		ns.Focus = FocusLeft
		return Action{RouteTo: RouteNone, Command: CmdSwitchFocusLeft, NewState: ns}
	}
	if key.Type == KeyRight || (key.Type == KeyRune && key.Rune == 'l') {
		ns.Focus = FocusRight
		return Action{RouteTo: RouteNone, Command: CmdSwitchFocusRight, NewState: ns}
	}

	// Quit.
	if key.Type == KeyRune && key.Rune == 'q' {
		return Action{RouteTo: RouteNone, Command: CmdQuit, NewState: ns}
	}

	// Pane resize: '>'/'.' grow the left pane (shrink Tetris), '<'/',' shrink
	// the left pane (grow Tetris), '='/'0' reset to an even split.
	if key.Type == KeyRune && (key.Rune == '>' || key.Rune == '.') {
		return Action{RouteTo: RouteNone, Command: CmdSplitGrowLeft, NewState: ns}
	}
	if key.Type == KeyRune && (key.Rune == '<' || key.Rune == ',') {
		return Action{RouteTo: RouteNone, Command: CmdSplitShrinkLeft, NewState: ns}
	}
	if key.Type == KeyRune && key.Rune == '=' {
		return Action{RouteTo: RouteNone, Command: CmdSplitReset, NewState: ns}
	}
	// 'z' cycles preset divider ratios (2:1 -> 1:1 -> 1:2). Distinct from the
	// 1-9 window-select keys, and resize-stable unlike the >/< nudge.
	if key.Type == KeyRune && key.Rune == 'z' {
		return Action{RouteTo: RouteNone, Command: CmdSplitRatioCycle, NewState: ns}
	}

	// cmux-style command windows: c new, n/p next/prev, x close, 1-9 select.
	if key.Type == KeyRune {
		switch key.Rune {
		case 'c':
			return Action{RouteTo: RouteNone, Command: CmdNewWindow, NewState: ns}
		case 'n':
			return Action{RouteTo: RouteNone, Command: CmdNextWindow, NewState: ns}
		case 'p':
			return Action{RouteTo: RouteNone, Command: CmdPrevWindow, NewState: ns}
		case 'x':
			return Action{RouteTo: RouteNone, Command: CmdCloseWindow, NewState: ns}
		}
		if key.Rune >= '1' && key.Rune <= '9' {
			return Action{RouteTo: RouteNone, Command: CmdSelectWindow, Arg: int(key.Rune - '0'), NewState: ns}
		}
	}

	// Unknown key while armed: disarm and swallow (no command, no routing).
	// Game stop/start/restart live on the game keys (p/r) when the board is
	// focused; reach the board with Tab.
	return Action{RouteTo: RouteNone, Command: CmdNone, NewState: ns}
}

// routeToFocus routes a normal key to the focused pane, forwarding its raw
// bytes (used for the PTY).
func routeToFocus(st State, key Key) Action {
	if st.Focus == FocusLeft {
		return Action{RouteTo: RouteLeft, Command: CmdNone, Bytes: key.Bytes, NewState: st}
	}
	return Action{RouteTo: RouteRight, Command: CmdNone, Bytes: key.Bytes, NewState: st}
}

// routeBytesToFocus routes specific bytes to the focused pane (used for the
// literal Ctrl-b case).
func routeBytesToFocus(st State, bytes []byte) Action {
	if st.Focus == FocusLeft {
		return Action{RouteTo: RouteLeft, Command: CmdNone, Bytes: bytes, NewState: st}
	}
	return Action{RouteTo: RouteRight, Command: CmdNone, Bytes: bytes, NewState: st}
}
