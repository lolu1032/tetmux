package app

import (
	"bytes"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/router"
)

// TestToRouterKeySpecials pins the exact key->PTY byte contract. These byte
// sequences are what the child shell/agent actually receives, so a regression
// here silently breaks every keystroke forwarded to the left pane.
func TestToRouterKeySpecials(t *testing.T) {
	cases := []struct {
		name      string
		msg       tea.KeyMsg
		wantType  router.KeyType
		wantBytes []byte
	}{
		{"ctrl-b", tea.KeyMsg{Type: tea.KeyCtrlB}, router.KeyCtrlB, []byte{0x02}},
		{"left", tea.KeyMsg{Type: tea.KeyLeft}, router.KeyLeft, []byte("\x1b[D")},
		{"right", tea.KeyMsg{Type: tea.KeyRight}, router.KeyRight, []byte("\x1b[C")},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, router.KeyUp, []byte("\x1b[A")},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, router.KeyDown, []byte("\x1b[B")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, router.KeyOther, []byte{'\r'}},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, router.KeyTab, []byte{'\t'}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, router.KeyEsc, []byte{0x1b}},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, router.KeyOther, []byte{0x7f}},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, router.KeyOther, []byte{' '}},
		{"ctrl-c", tea.KeyMsg{Type: tea.KeyCtrlC}, router.KeyCtrlC, []byte{0x03}},
		{"ctrl-d", tea.KeyMsg{Type: tea.KeyCtrlD}, router.KeyOther, []byte{0x04}},
		{"ctrl-z", tea.KeyMsg{Type: tea.KeyCtrlZ}, router.KeyOther, []byte{0x1a}},
	}
	for _, c := range cases {
		got := toRouterKey(c.msg)
		if got.Type != c.wantType {
			t.Errorf("%s: type=%v want %v", c.name, got.Type, c.wantType)
		}
		if !bytes.Equal(got.Bytes, c.wantBytes) {
			t.Errorf("%s: bytes=%v want %v", c.name, got.Bytes, c.wantBytes)
		}
	}
}

func TestToRouterKeyRunes(t *testing.T) {
	got := toRouterKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if got.Type != router.KeyRune {
		t.Errorf("type=%v want KeyRune", got.Type)
	}
	if got.Rune != 'a' {
		t.Errorf("rune=%q want 'a'", got.Rune)
	}
	if string(got.Bytes) != "a" {
		t.Errorf("bytes=%q want \"a\"", got.Bytes)
	}
}

func TestClipPad(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"hi", 0, ""},
		{"hi", -3, ""},
		{"abc", 3, "abc"},
		{"ab", 4, "ab  "},
		{"abcdef", 3, "abc"},
		{"", 2, "  "},
	}
	for _, c := range cases {
		if got := clipPad(c.in, c.width); got != c.want {
			t.Errorf("clipPad(%q,%d)=%q want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestExitBanner(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{-1, "[exited]"},
		{0, "[exited: code 0]"},
		{137, "[exited: code 137]"},
	}
	for _, c := range cases {
		if got := exitBanner(c.code); got != c.want {
			t.Errorf("exitBanner(%d)=%q want %q", c.code, got, c.want)
		}
	}
}

func TestLeftCmdName(t *testing.T) {
	if got := leftCmdName([]string{"claude", "--foo"}); got != "claude" {
		t.Errorf("leftCmdName argv = %q want claude", got)
	}
	t.Setenv("SHELL", "/bin/zsh")
	if got := leftCmdName(nil); got != "/bin/zsh" {
		t.Errorf("leftCmdName empty = %q want /bin/zsh", got)
	}
}
