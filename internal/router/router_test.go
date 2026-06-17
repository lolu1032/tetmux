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
	act := Route(st, Key{Type: KeyRune, Rune: 'z', Bytes: []byte{'z'}})
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
