package app

import (
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/router"
)

// toRouterKey maps a bubbletea KeyMsg onto the router's normalized Key. The
// Bytes field carries what should be forwarded to the PTY when the key routes
// to the left pane.
func toRouterKey(msg tea.KeyMsg) router.Key {
	switch msg.Type {
	case tea.KeyCtrlB:
		return router.Key{Type: router.KeyCtrlB, Bytes: []byte{0x02}}
	case tea.KeyTab:
		return router.Key{Type: router.KeyTab, Bytes: []byte{'\t'}}
	case tea.KeyEsc:
		return router.Key{Type: router.KeyEsc, Bytes: []byte{0x1b}}
	case tea.KeyCtrlC:
		return router.Key{Type: router.KeyCtrlC, Bytes: []byte{0x03}}
	case tea.KeyLeft:
		return router.Key{Type: router.KeyLeft, Bytes: []byte("\x1b[D")}
	case tea.KeyRight:
		return router.Key{Type: router.KeyRight, Bytes: []byte("\x1b[C")}
	case tea.KeyUp:
		return router.Key{Type: router.KeyUp, Bytes: []byte("\x1b[A")}
	case tea.KeyDown:
		return router.Key{Type: router.KeyDown, Bytes: []byte("\x1b[B")}
	case tea.KeyRunes:
		var r rune
		if len(msg.Runes) > 0 {
			r = msg.Runes[0]
		}
		return router.Key{Type: router.KeyRune, Rune: r, Bytes: []byte(string(msg.Runes))}
	default:
		// Special keys (enter, tab, esc, ctrl-*, backspace...). Forward the
		// raw bytes via String mapping where possible.
		return router.Key{Type: router.KeyOther, Bytes: specialBytes(msg)}
	}
}

// specialBytes returns the byte sequence to forward to the PTY for a special
// key that bubbletea has already decoded.
func specialBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyCtrlC:
		return []byte{0x03}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyCtrlZ:
		return []byte{0x1a}
	}
	// Fall back to the textual representation for anything else.
	s := msg.String()
	if len(s) == 1 {
		return []byte(s)
	}
	return nil
}

// Bracketed-paste markers (DEC private mode 2004). A child that enabled the mode
// expects pasted content delivered framed by these so it can treat a paste (a
// dragged-in file/image path, a multi-line block) differently from individually
// typed keys — without them an app like claude shows a pasted image path as
// literal text instead of attaching it.
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// wrapBracketedPaste frames b with the bracketed-paste markers so the child sees
// it as one paste. Only used when the child has enabled mode 2004 (see
// leftPane.BracketedPaste); feeding the markers to an app that didn't ask for
// them would surface them as garbage.
func wrapBracketedPaste(b []byte) []byte {
	out := make([]byte, 0, len(pasteStart)+len(b)+len(pasteEnd))
	out = append(out, pasteStart...)
	out = append(out, b...)
	out = append(out, pasteEnd...)
	return out
}

// joinRows joins rendered rows with newlines.
func joinRows(rows []string) string {
	return strings.Join(rows, "\n")
}

// clipPad clips or right-pads s to exactly width display columns (rune-based;
// adequate for the ASCII status text we emit).
func clipPad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > width {
		return string(r[:width])
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}

// itoa wraps strconv for readability in views.
func itoa(n int) string { return strconv.Itoa(n) }

// leftCmdName returns a short display name for the left-pane command.
func leftCmdName(argv []string) string {
	if len(argv) > 0 && argv[0] != "" {
		return argv[0]
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// leftErrorContent renders the spawn-failure message shown in the left pane
// when the command could not be started, padded into the pane height.
func leftErrorContent(argv []string, err error, w, h int) string {
	lines := []string{
		clipPad("failed to start \""+leftCmdName(argv)+"\":", w),
		clipPad("  "+err.Error(), w),
		clipPad("C-b q to quit", w),
	}
	for len(lines) < h {
		lines = append(lines, clipPad("", w))
	}
	return joinRows(lines[:maxInt(minInt(h, len(lines)), 1)])
}

// exitBanner renders the "[exited]" banner with the child exit code when known.
func exitBanner(code int) string {
	if code < 0 {
		return "[exited]"
	}
	return "[exited: code " + itoa(code) + "]"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
