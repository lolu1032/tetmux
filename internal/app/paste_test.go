package app

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// waitBracketed polls until the child's bracketed-paste mode (DEC 2004) reaches
// want, or the deadline passes. Returns the final observed value.
func waitBracketed(lp *leftPane, want bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if lp.BracketedPaste() == want {
			return want
		}
		time.Sleep(5 * time.Millisecond)
	}
	return lp.BracketedPaste()
}

// pasteMsg is a bubbletea paste event (one KeyRunes msg with Paste set), exactly
// how a dragged-in image path or a clipboard paste is delivered.
func pasteMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

// TestBracketedPasteModeTracked pins that the vendored vt10x now tracks DEC
// private mode 2004 so the app can tell whether the child wants bracketed pastes.
func TestBracketedPasteModeTracked(t *testing.T) {
	lp, err := newLeftPane([]string{"sh", "-c", "printf '\\033[?2004h'; sleep 5"}, 20, 8, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	defer lp.Close()

	if !waitBracketed(lp, true, 2*time.Second) {
		t.Fatal("BracketedPaste() never became true after the child enabled mode 2004")
	}
}

// TestPasteWrappedWhenChildEnabledMode2004 is the end-to-end guard for the
// "pasted image path shows as literal text" bug: when the child enabled
// bracketed paste, a bubbletea paste forwarded to the PTY must arrive framed by
// ESC[200~ ... ESC[201~ so the child recognizes it as a paste.
func TestPasteWrappedWhenChildEnabledMode2004(t *testing.T) {
	got := captureChildStdin(t, true, "/tmp/shot.png")
	want := pasteStart + "/tmp/shot.png" + pasteEnd
	if got != want {
		t.Errorf("child received %q, want bracketed %q", got, want)
	}
}

// TestPasteRawWhenChildHasNoBracketedPaste pins the converse: a child that never
// enabled mode 2004 must get the raw bytes, never the markers (feeding ESC[200~
// to such an app would surface as garbage).
func TestPasteRawWhenChildHasNoBracketedPaste(t *testing.T) {
	got := captureChildStdin(t, false, "/tmp/shot.png")
	if got != "/tmp/shot.png" {
		t.Errorf("child received %q, want raw %q (no bracketed markers)", got, "/tmp/shot.png")
	}
}

// captureChildStdin spawns a child that (optionally) enables bracketed paste and
// then copies its stdin to a temp file, drives one paste of payload through the
// Model, and returns what the child actually received on its stdin. Raw tty mode
// makes the bytes flow immediately and byte-for-byte (no echo, no canonical
// line-buffering) so the assertion is deterministic.
func captureChildStdin(t *testing.T, enable2004 bool, payload string) string {
	t.Helper()
	out := t.TempDir() + "/got"
	script := "stty raw -echo 2>/dev/null; "
	if enable2004 {
		script = "printf '\\033[?2004h'; " + script
	}
	script += "cat > " + out

	m := New(nil, 1)
	lp, err := newLeftPane([]string{"sh", "-c", script}, 20, 8, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	m.windows = append(m.windows, lp)
	m.active = 0

	if waitBracketed(lp, enable2004, 2*time.Second) != enable2004 {
		t.Fatalf("bracketed-paste mode = %v, want %v before paste", lp.BracketedPaste(), enable2004)
	}

	m.handleKey(pasteMsg(payload))

	// Let the child read the paste, then close to EOF so cat flushes and exits.
	time.Sleep(150 * time.Millisecond)
	lp.Close()
	waitExited(lp, 2*time.Second)

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read child stdin capture: %v", err)
	}
	return string(b)
}
