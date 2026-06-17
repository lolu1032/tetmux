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
	// KeyLeft / KeyRight / KeyUp / KeyDown are arrow keys.
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	// KeyOther is any other special key (enter, esc, tab, function keys...).
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
	NewState State
}

// Route is the pure routing function. Given the current state and an incoming
// key, it returns the Action describing what to do and the next state.
//
// Rules (per spec):
//   - prefix disarmed + Ctrl-b           => arm prefix, consume key.
//   - prefix disarmed + any other key    => route to focused pane.
//   - prefix armed   + Ctrl-b            => send one literal Ctrl-b to focused
//     pane, disarm.
//   - prefix armed   + 'l' / RightArrow  => CmdSwitchFocusRight, disarm.
//   - prefix armed   + 'h' / LeftArrow   => CmdSwitchFocusLeft, disarm.
//   - prefix armed   + 'q'               => CmdQuit, disarm.
//   - prefix armed   + unknown key       => disarm, swallow (no command, no
//     routing).
func Route(st State, key Key) Action {
	if st.PrefixArmed {
		return routeArmed(st, key)
	}

	// Prefix disarmed.
	if key.Type == KeyCtrlB {
		ns := st
		ns.PrefixArmed = true
		return Action{RouteTo: RouteNone, Command: CmdNone, NewState: ns}
	}
	return routeToFocus(st, key)
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

	// Unknown key while armed: disarm and swallow (no command, no routing).
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
