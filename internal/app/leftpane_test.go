package app

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// TestMarkDirtyCoalesces verifies the redraw push is coalesced two ways: a burst
// of output before the UI consumes the dirty bit collapses into one push, and a
// fresh cycle within the same frame is deferred (frame-rate gated) rather than
// pushed synchronously — then fires once the frame boundary passes. n is atomic
// because the deferred push runs on a timer goroutine.
func TestMarkDirtyCoalesces(t *testing.T) {
	var n atomic.Int32
	lp := &leftPane{notify: func() { n.Add(1) }, exitCode: -1}

	// Fresh pane (lastNotify zero) => first dirty pushes immediately.
	lp.markDirty()
	if n.Load() != 1 || !lp.Dirty() {
		t.Fatalf("after first markDirty: n=%d dirty=%v", n.Load(), lp.Dirty())
	}
	lp.markDirty() // still dirty: coalesced, no second push
	if n.Load() != 1 {
		t.Fatalf("coalescing failed: n=%d want 1", n.Load())
	}
	lp.ConsumeDirty()
	if lp.Dirty() {
		t.Fatalf("dirty not cleared by ConsumeDirty")
	}

	// A fresh cycle within the same frame is deferred to the frame boundary.
	lp.markDirty()
	if got := n.Load(); got != 1 {
		t.Fatalf("within-frame redraw should defer: n=%d want 1", got)
	}
	// After the boundary the deferred push fires exactly once.
	deadline := time.Now().Add(time.Second)
	for n.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := n.Load(); got != 2 {
		t.Fatalf("deferred redraw never fired: n=%d want 2", got)
	}
}

func TestLeftPaneSpawnExitCode(t *testing.T) {
	lp, err := newLeftPane([]string{"sh", "-c", "exit 3"}, 10, 4, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	defer lp.Close()

	if !waitExited(lp, 3*time.Second) {
		t.Fatal("child did not exit in time")
	}
	if got := lp.ExitCode(); got != 3 {
		t.Errorf("exit code=%d want 3", got)
	}
	// Writing to an exited pane must be a no-op (no panic).
	lp.Write([]byte("ignored"))
	// Resize on an exited pane must be a no-op.
	lp.Resize(20, 20)
}

// TestLeftPaneConcurrent exercises the reader goroutine against concurrent
// RenderRows/Resize/Dirty/Exited calls. Run under `go test -race` it asserts
// the vt10x lock discipline holds.
func TestLeftPaneConcurrent(t *testing.T) {
	lp, err := newLeftPane(
		[]string{"sh", "-c", "i=0; while [ $i -lt 300 ]; do printf 'x%d\\n' $i; i=$((i+1)); done"},
		20, 8, nil,
	)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	defer lp.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = lp.RenderRows(20, 8, true)
				lp.Resize(20, 8)
				_ = lp.Dirty()
				_ = lp.Exited()
			}
		}
	}()

	waitExited(lp, 3*time.Second)
	close(done)
	wg.Wait() // ensure the worker fully stops before the deferred Close()
}

// TestResizeDoesNotNotify pins the invariant that Resize never pushes a redraw.
// Resize runs on the bubbletea Update goroutine, which already re-renders when
// it returns; a notify from there would re-enter Program.Send and deadlock. A
// quiet child produces no output (so notify stays at 0 on its own), and Resize
// must not bump it.
func TestResizeDoesNotNotify(t *testing.T) {
	var n atomic.Int32
	lp, err := newLeftPane([]string{"sleep", "1"}, 20, 8, func() { n.Add(1) })
	if err != nil {
		t.Skip("pty unavailable:", err)
	}
	defer lp.Close()

	time.Sleep(50 * time.Millisecond) // let the quiet child settle
	before := n.Load()
	for i := 0; i < 5; i++ {
		lp.Resize(20+i, 8)
	}
	time.Sleep(50 * time.Millisecond)
	if got := n.Load(); got != before {
		t.Errorf("Resize pushed %d redraws (before=%d); Resize must never notify", got-before, before)
	}
}

// TestCloseUnblocksBlockedWrite guards the shutdown deadlock: Write must not
// hold lp.mu across the blocking ptmx.Write, or Close() (which needs lp.mu)
// hangs forever behind a child that isn't draining stdin, orphaning it. With a
// non-draining child the PTY input buffer fills and Write blocks; Close must
// still return promptly (it closes the fd, unblocking the parked write).
func TestCloseUnblocksBlockedWrite(t *testing.T) {
	lp, err := newLeftPane([]string{"sh", "-c", "sleep 30"}, 80, 24, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}

	writing := make(chan struct{})
	go func() {
		close(writing)
		big := make([]byte, 1<<16)
		for i := 0; i < 256; i++ {
			lp.Write(big) // blocks once the PTY input buffer fills
		}
	}()
	<-writing
	time.Sleep(100 * time.Millisecond) // let the writer wedge in the write syscall

	done := make(chan struct{})
	go func() {
		lp.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close() deadlocked behind a blocked Write — shutdown would orphan the child")
	}
}

// TestConcurrentWriteAndClose runs Write/Resize concurrently with Close so
// `go test -race` catches any unsynchronized access to the closed flag / fd.
func TestConcurrentWriteAndClose(t *testing.T) {
	lp, err := newLeftPane([]string{"sh", "-c", "sleep 5"}, 20, 8, nil)
	if err != nil {
		t.Skip("pty unavailable:", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := []byte("hello\n")
			for {
				select {
				case <-stop:
					return
				default:
					lp.Write(b)
					lp.Resize(20, 8)
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	lp.Close() // concurrent with the writers; must not race or panic
	close(stop)
	wg.Wait()
}

func waitExited(lp *leftPane, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if lp.Exited() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return lp.Exited()
}

// splitRunes must never hand a partial multibyte rune to the emulator and must
// never lose bytes, no matter where a read boundary falls. This is the
// regression guard for the Hangul/CJK byte-loss bug (a 3-byte glyph split across
// the 4 KB PTY read boundary used to vanish).
func TestSplitRunesReassembles(t *testing.T) {
	full := []byte("가나다 abc 한글 ✓ 日本語\x1b[mx") // Hangul + ASCII + ESC + emoji + CJK

	// Every two-way split point: feed [:i] then [i:], threading carry as readLoop
	// does, and flush the final carry (the EOF path). Reassembly must be lossless.
	for i := 0; i <= len(full); i++ {
		var got, carry []byte
		for _, chunk := range [][]byte{full[:i], full[i:]} {
			var w []byte
			w, carry = splitRunes(carry, chunk)
			if len(w) > 0 && !utf8.Valid(w) {
				t.Fatalf("split %d: emitted a non-boundary slice %q", i, w)
			}
			got = append(got, w...)
		}
		got = append(got, carry...)
		if string(got) != string(full) {
			t.Fatalf("split at %d: reassembled %q, want %q", i, got, full)
		}
	}

	// Worst case: one byte at a time. Every emitted slice must be whole runes,
	// and the concatenation must equal the original.
	var got, carry []byte
	for i := 0; i < len(full); i++ {
		var w []byte
		w, carry = splitRunes(carry, full[i:i+1])
		if len(w) > 0 && !utf8.Valid(w) {
			t.Fatalf("byte %d: emitted a partial rune %q", i, w)
		}
		got = append(got, w...)
	}
	got = append(got, carry...)
	if string(got) != string(full) {
		t.Fatalf("byte-by-byte: reassembled %q, want %q", got, full)
	}
}
