package app

import (
	"io"
	"os"
	"testing"

	"tetmux/internal/layout"
)

// hwCursor brackets a frame: anchor => hide-before + show+position-after; a
// hidden state keeps it hidden; an unmanaged (zero) cursor emits nothing.
func TestHWCursorFrameEscapes(t *testing.T) {
	var c hwCursor // zero value: unmanaged
	if c.framePre() != "" || c.framePost() != "" {
		t.Errorf("unmanaged hwCursor should emit nothing")
	}
	c.anchor(5, 12)
	if got, want := c.framePre(), "\x1b[?25l"; got != want {
		t.Errorf("framePre = %q, want %q", got, want)
	}
	if got, want := c.framePost(), "\x1b[?25h\x1b[5;12H"; got != want {
		t.Errorf("framePost = %q, want %q", got, want)
	}
	c.hideCursor() // managed but hidden (e.g. game pane focused)
	if got, want := c.framePre(), "\x1b[?25l"; got != want {
		t.Errorf("hidden framePre = %q, want %q", got, want)
	}
	if got := c.framePost(); got != "" {
		t.Errorf("hidden framePost should stay hidden, got %q", got)
	}
	c.disable() // TETMUX_NO_IME_CURSOR: hands cursor back to bubbletea
	if c.framePre() != "" || c.framePost() != "" {
		t.Errorf("disabled hwCursor should emit nothing")
	}
}

// cursorWriter brackets the frame with hide(before)+show/position(after), and
// reports n == len(payload) (not counting the bracketing bytes).
func TestCursorWriterBracketsFrame(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()

	cs := &hwCursor{}
	cs.anchor(3, 7)
	cw := &cursorWriter{File: pw, cs: cs}

	payload := []byte("FRAME")
	done := make(chan struct{})
	var got []byte
	go func() {
		defer close(done)
		got, _ = io.ReadAll(pr)
	}()

	n, err := cw.Write(payload)
	pw.Close()
	<-done
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(payload) {
		t.Errorf("n = %d, want %d (bracketing escapes must not be counted)", n, len(payload))
	}
	if want := "\x1b[?25lFRAME\x1b[?25h\x1b[3;7H"; string(got) != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// With an unmanaged cursor, cursorWriter is a transparent pass-through.
func TestCursorWriterPassThroughWhenUnmanaged(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()

	cw := &cursorWriter{File: pw, cs: &hwCursor{}} // unmanaged

	done := make(chan struct{})
	var got []byte
	go func() {
		defer close(done)
		got, _ = io.ReadAll(pr)
	}()
	if _, err := cw.Write([]byte("HELLO")); err != nil {
		t.Fatalf("write: %v", err)
	}
	pw.Close()
	<-done
	if string(got) != "HELLO" {
		t.Errorf("output = %q, want %q", got, "HELLO")
	}
}

// absCursor maps a pane cell to absolute 1-based screen coords and rejects
// cells outside the visible pane. Layout: tab row + top border push content
// down by 2 (1-based row = cy+3); left border pushes content right by 1
// (1-based col = cx+2).
func TestAbsCursor(t *testing.T) {
	l := layout.ComputeSplit(120, 40, layout.DefaultBorders(), 0)
	if l.LeftInnerWidth < 5 || l.InnerHeight < 5 {
		t.Fatalf("unexpectedly tiny layout: %+v", l)
	}

	row, col, active := absCursor(l, 0, 0)
	if !active || row != 3 || col != 2 {
		t.Errorf("origin -> row=%d col=%d active=%v, want 3,2,true", row, col, active)
	}

	row, col, active = absCursor(l, 4, 6)
	if !active || row != 9 || col != 6 {
		t.Errorf("(4,6) -> row=%d col=%d active=%v, want 9,6,true", row, col, active)
	}

	if _, _, active := absCursor(l, -1, 0); active {
		t.Error("negative column should be inactive")
	}
	if _, _, active := absCursor(l, l.LeftInnerWidth, 0); active {
		t.Error("column == width (right border) should be inactive")
	}
	if _, _, active := absCursor(l, 0, l.InnerHeight); active {
		t.Error("row == height (bottom border) should be inactive")
	}
}

// TETMUX_NO_IME_CURSOR (m.imeCursor=false) disables hardware-cursor anchoring, so
// no positioning escape is emitted (the escape hatch for terminals where it
// misplaces the IME preedit). With no window focused there is likewise no anchor.
func TestIMECursorToggleAndGating(t *testing.T) {
	m := New(nil, 1)
	m.imeCursor = false
	m.hw.anchor(5, 5) // pretend a previous frame anchored
	m.updateHWCursor(m.layout())
	// Disabled => fully unmanaged: emit nothing, let bubbletea own the cursor.
	if m.hw.framePre() != "" || m.hw.framePost() != "" {
		t.Errorf("disabled IME cursor must emit nothing")
	}

	m2 := New(nil, 1) // enabled by default, but no command window exists yet
	m2.hw.anchor(5, 5)
	m2.updateHWCursor(m2.layout())
	// No focused window => managed but hidden: no show/position escape.
	if got := m2.hw.framePost(); got != "" {
		t.Errorf("no focused window => cursor must not be shown/anchored, got %q", got)
	}
}
