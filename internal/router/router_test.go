package router

import (
	"bytes"
	"testing"
)

func TestNormalKeyLeftFocus(t *testing.T) {
	st := State{Focus: FocusLeft, PrefixArmed: false}
	key := Key{Type: KeyRune, Rune: 'a', Bytes: []byte{'a'}}
	act := Route(st, key)
	if act.RouteTo != RouteLeft {
		t.Errorf("expected RouteLeft, got %v", act.RouteTo)
	}
	if act.Command != CmdNone {
		t.Errorf("expected CmdNone, got %v", act.Command)
	}
	if act.NewState != st {
		t.Errorf("state should be unchanged: %+v vs %+v", act.NewState, st)
	}
	if !bytes.Equal(act.Bytes, []byte{'a'}) {
		t.Errorf("expected bytes [a], got %v", act.Bytes)
	}
}

func TestNormalKeyRightFocus(t *testing.T) {
	st := State{Focus: FocusRight, PrefixArmed: false}
	key := Key{Type: KeyDown}
	act := Route(st, key)
	if act.RouteTo != RouteRight {
		t.Errorf("expected RouteRight, got %v", act.RouteTo)
	}
	if act.Command != CmdNone {
		t.Errorf("expected CmdNone, got %v", act.Command)
	}
	if act.NewState != st {
		t.Errorf("state should be unchanged")
	}
}

func TestArmPrefix(t *testing.T) {
	st := State{Focus: FocusLeft, PrefixArmed: false}
	act := Route(st, Key{Type: KeyCtrlB})
	if act.RouteTo != RouteNone {
		t.Errorf("arming prefix should not route, got %v", act.RouteTo)
	}
	if act.Command != CmdNone {
		t.Errorf("arming prefix should issue no command, got %v", act.Command)
	}
	if !act.NewState.PrefixArmed {
		t.Errorf("prefix should be armed in new state")
	}
	if len(act.Bytes) != 0 {
		t.Errorf("arming prefix should route no bytes, got %v", act.Bytes)
	}
}

func TestPrefixLSwitchesRight(t *testing.T) {
	for _, key := range []Key{
		{Type: KeyRune, Rune: 'l'},
		{Type: KeyRight},
	} {
		st := State{Focus: FocusLeft, PrefixArmed: true}
		act := Route(st, key)
		if act.Command != CmdSwitchFocusRight {
			t.Errorf("key %+v: expected CmdSwitchFocusRight, got %v", key, act.Command)
		}
		if act.NewState.Focus != FocusRight {
			t.Errorf("key %+v: focus should be right", key)
		}
		if act.NewState.PrefixArmed {
			t.Errorf("key %+v: prefix should be disarmed", key)
		}
		if act.RouteTo != RouteNone {
			t.Errorf("key %+v: should not route to a pane", key)
		}
	}
}

func TestPrefixHSwitchesLeft(t *testing.T) {
	for _, key := range []Key{
		{Type: KeyRune, Rune: 'h'},
		{Type: KeyLeft},
	} {
		st := State{Focus: FocusRight, PrefixArmed: true}
		act := Route(st, key)
		if act.Command != CmdSwitchFocusLeft {
			t.Errorf("key %+v: expected CmdSwitchFocusLeft, got %v", key, act.Command)
		}
		if act.NewState.Focus != FocusLeft {
			t.Errorf("key %+v: focus should be left", key)
		}
		if act.NewState.PrefixArmed {
			t.Errorf("key %+v: prefix should be disarmed", key)
		}
	}
}

func TestPrefixQQuits(t *testing.T) {
	st := State{Focus: FocusLeft, PrefixArmed: true}
	act := Route(st, Key{Type: KeyRune, Rune: 'q'})
	if act.Command != CmdQuit {
		t.Errorf("expected CmdQuit, got %v", act.Command)
	}
	if act.NewState.PrefixArmed {
		t.Errorf("prefix should be disarmed after quit")
	}
}

func TestPrefixUnknownDisarmsAndSwallows(t *testing.T) {
	st := State{Focus: FocusLeft, PrefixArmed: true}
	// 'j' is not bound to any prefix command (unlike z=ratio, c/n/p/x=window...).
	act := Route(st, Key{Type: KeyRune, Rune: 'j', Bytes: []byte{'j'}})
	if act.NewState.PrefixArmed {
		t.Errorf("unknown key should disarm prefix")
	}
	if act.Command != CmdNone {
		t.Errorf("unknown key should issue no command, got %v", act.Command)
	}
	if act.RouteTo != RouteNone {
		t.Errorf("unknown prefix key should be swallowed, got route %v", act.RouteTo)
	}
}

func TestPrefixZCyclesSplitRatio(t *testing.T) {
	st := State{Focus: FocusLeft, PrefixArmed: true}
	act := Route(st, Key{Type: KeyRune, Rune: 'z', Bytes: []byte{'z'}})
	if act.Command != CmdSplitRatioCycle {
		t.Errorf("C-b z should cycle the split ratio, got %v", act.Command)
	}
	if act.RouteTo != RouteNone {
		t.Errorf("C-b z must be consumed by tetmux, got route %v", act.RouteTo)
	}
	if act.NewState.PrefixArmed {
		t.Errorf("prefix should disarm after C-b z")
	}
}

func TestDoubleCtrlBSendsLiteral(t *testing.T) {
	// Ctrl-b arms; second Ctrl-b sends exactly one literal 0x02 to the pane.
	st := State{Focus: FocusLeft, PrefixArmed: false}
	a1 := Route(st, Key{Type: KeyCtrlB})
	if !a1.NewState.PrefixArmed {
		t.Fatalf("first Ctrl-b should arm prefix")
	}
	a2 := Route(a1.NewState, Key{Type: KeyCtrlB})
	if a2.RouteTo != RouteLeft {
		t.Errorf("literal Ctrl-b should route to focused (left) pane, got %v", a2.RouteTo)
	}
	if !bytes.Equal(a2.Bytes, []byte{PrefixByte}) {
		t.Errorf("expected one literal Ctrl-b byte (0x02), got %v", a2.Bytes)
	}
	if a2.NewState.PrefixArmed {
		t.Errorf("prefix should be disarmed after literal Ctrl-b")
	}
}

func TestDoubleCtrlBRightFocus(t *testing.T) {
	st := State{Focus: FocusRight, PrefixArmed: true}
	act := Route(st, Key{Type: KeyCtrlB})
	if act.RouteTo != RouteRight {
		t.Errorf("literal Ctrl-b with right focus should route right, got %v", act.RouteTo)
	}
	if !bytes.Equal(act.Bytes, []byte{PrefixByte}) {
		t.Errorf("expected literal 0x02, got %v", act.Bytes)
	}
}

func TestPrefixResizeCommands(t *testing.T) {
	cases := []struct {
		rune rune
		want Command
	}{
		{'>', CmdSplitGrowLeft}, {'.', CmdSplitGrowLeft},
		{'<', CmdSplitShrinkLeft}, {',', CmdSplitShrinkLeft},
		{'=', CmdSplitReset},
	}
	for _, c := range cases {
		st := State{Focus: FocusLeft, PrefixArmed: true}
		act := Route(st, Key{Type: KeyRune, Rune: c.rune, Bytes: []byte(string(c.rune))})
		if act.Command != c.want {
			t.Errorf("prefix+%q: expected command %v, got %v", c.rune, c.want, act.Command)
		}
		if act.RouteTo != RouteNone {
			t.Errorf("prefix+%q: resize must not route to a pane, got %v", c.rune, act.RouteTo)
		}
		if act.NewState.PrefixArmed {
			t.Errorf("prefix+%q: prefix should be disarmed", c.rune)
		}
		if act.NewState.Focus != st.Focus {
			t.Errorf("prefix+%q: focus must not change on resize", c.rune)
		}
	}
}

func TestPrefixWindowCommands(t *testing.T) {
	cases := []struct {
		rune    rune
		want    Command
		wantArg int
	}{
		{'c', CmdNewWindow, 0},
		{'n', CmdNextWindow, 0},
		{'p', CmdPrevWindow, 0},
		{'x', CmdCloseWindow, 0},
		{'1', CmdSelectWindow, 1},
		{'9', CmdSelectWindow, 9},
	}
	for _, c := range cases {
		st := State{Focus: FocusLeft, PrefixArmed: true}
		act := Route(st, Key{Type: KeyRune, Rune: c.rune, Bytes: []byte{byte(c.rune)}})
		if act.Command != c.want {
			t.Errorf("prefix+%q: command=%v want %v", c.rune, act.Command, c.want)
		}
		if act.Arg != c.wantArg {
			t.Errorf("prefix+%q: arg=%d want %d", c.rune, act.Arg, c.wantArg)
		}
		if act.NewState.PrefixArmed {
			t.Errorf("prefix+%q: prefix should be disarmed", c.rune)
		}
	}
}

func TestTabTogglesFocus(t *testing.T) {
	// Tab flips focus both ways and is never routed to a pane.
	left := State{Focus: FocusLeft}
	a1 := Route(left, Key{Type: KeyTab, Bytes: []byte{'\t'}})
	if a1.Command != CmdToggleFocus || a1.NewState.Focus != FocusRight {
		t.Errorf("Tab from left: cmd=%v focus=%v want CmdToggleFocus/FocusRight", a1.Command, a1.NewState.Focus)
	}
	if a1.RouteTo != RouteNone {
		t.Errorf("Tab must not route to a pane, got %v", a1.RouteTo)
	}
	a2 := Route(a1.NewState, Key{Type: KeyTab, Bytes: []byte{'\t'}})
	if a2.NewState.Focus != FocusLeft {
		t.Errorf("Tab from right should return to left, got %v", a2.NewState.Focus)
	}
}

func TestEscRoutesToFocusedPane(t *testing.T) {
	// Esc is NOT a focus toggle; it routes to the focused pane (the game reads
	// it as pause; the command pane receives the literal Esc byte).
	r := Route(State{Focus: FocusRight}, Key{Type: KeyEsc, Bytes: []byte{0x1b}})
	if r.RouteTo != RouteRight || r.Command != CmdNone {
		t.Errorf("Esc (right focus): routeTo=%v cmd=%v want RouteRight/CmdNone", r.RouteTo, r.Command)
	}
	l := Route(State{Focus: FocusLeft}, Key{Type: KeyEsc, Bytes: []byte{0x1b}})
	if l.RouteTo != RouteLeft || !bytes.Equal(l.Bytes, []byte{0x1b}) {
		t.Errorf("Esc (left focus): routeTo=%v bytes=%v want RouteLeft/[27]", l.RouteTo, l.Bytes)
	}
}

func TestCtrlCQuitsFromGameForwardsFromCommand(t *testing.T) {
	// Game focused: Ctrl-C quits tetmux (back to the host terminal).
	r := Route(State{Focus: FocusRight}, Key{Type: KeyCtrlC, Bytes: []byte{0x03}})
	if r.Command != CmdQuit || r.RouteTo != RouteNone {
		t.Errorf("Ctrl-C (right focus): cmd=%v routeTo=%v want CmdQuit/RouteNone", r.Command, r.RouteTo)
	}
	// Command focused: Ctrl-C is forwarded to the child as a literal interrupt.
	l := Route(State{Focus: FocusLeft}, Key{Type: KeyCtrlC, Bytes: []byte{0x03}})
	if l.Command != CmdNone || l.RouteTo != RouteLeft || !bytes.Equal(l.Bytes, []byte{0x03}) {
		t.Errorf("Ctrl-C (left focus): cmd=%v routeTo=%v bytes=%v want CmdNone/RouteLeft/[3]", l.Command, l.RouteTo, l.Bytes)
	}
}

func TestPrefixTabSendsLiteralTab(t *testing.T) {
	// C-b then Tab delivers a real Tab to the focused pane (escape hatch for
	// shell completion, since a bare Tab toggles focus).
	st := State{Focus: FocusLeft, PrefixArmed: true}
	act := Route(st, Key{Type: KeyTab, Bytes: []byte{'\t'}})
	if act.RouteTo != RouteLeft {
		t.Errorf("C-b Tab should route to the focused pane, got %v", act.RouteTo)
	}
	if !bytes.Equal(act.Bytes, []byte{'\t'}) {
		t.Errorf("C-b Tab bytes=%v want [9]", act.Bytes)
	}
	if act.NewState.PrefixArmed {
		t.Errorf("prefix should be disarmed after C-b Tab")
	}
}

func TestPlainPRGoToFocusedPane(t *testing.T) {
	// Without the prefix, 'p'/'r' are ordinary keys (game keys / left input).
	for _, r := range []rune{'p', 'r'} {
		st := State{Focus: FocusRight, PrefixArmed: false}
		act := Route(st, Key{Type: KeyRune, Rune: r, Bytes: []byte{byte(r)}})
		if act.RouteTo != RouteRight || act.Command != CmdNone {
			t.Errorf("unprefixed %q: routeTo=%v cmd=%v want RouteRight/CmdNone", r, act.RouteTo, act.Command)
		}
	}
}

func TestResizeRunesAreNormalKeysWithoutPrefix(t *testing.T) {
	// Without the prefix armed, '>' etc. are ordinary keys for the focused pane.
	st := State{Focus: FocusLeft, PrefixArmed: false}
	act := Route(st, Key{Type: KeyRune, Rune: '>', Bytes: []byte{'>'}})
	if act.RouteTo != RouteLeft {
		t.Errorf("unprefixed '>' should route to focused pane, got %v", act.RouteTo)
	}
	if act.Command != CmdNone {
		t.Errorf("unprefixed '>' should issue no command, got %v", act.Command)
	}
}

func TestSwitchThenKeyPersistsFocus(t *testing.T) {
	// prefix + 'l' switches to RIGHT, then a normal key routes RIGHT.
	st := State{Focus: FocusLeft, PrefixArmed: true}
	a1 := Route(st, Key{Type: KeyRune, Rune: 'l'})
	if a1.NewState.Focus != FocusRight {
		t.Fatalf("expected focus right after switch")
	}
	a2 := Route(a1.NewState, Key{Type: KeyRune, Rune: 'x', Bytes: []byte{'x'}})
	if a2.RouteTo != RouteRight {
		t.Errorf("subsequent key should route to RIGHT (persisted focus), got %v", a2.RouteTo)
	}
}
