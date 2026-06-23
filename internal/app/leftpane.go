package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"

	"tetmux/internal/vtrender"
)

// frameInterval caps how often the left pane pushes a redraw. A chatty child
// (streaming agent, fast build log) can emit many chunks per frame; without a
// gate that is one full re-render per chunk. Coalescing to ~60fps keeps a busy
// child smooth without affecting an idle one (no output => no timer => 0 CPU)
// or a single keystroke (if the last redraw was >frameInterval ago it fires
// immediately).
const frameInterval = 16 * time.Millisecond

// leftPane owns the PTY child process and its vt10x cell-grid emulator. A
// reader goroutine pumps PTY bytes into the terminal and, on each new chunk,
// pushes a redraw via notify (coalesced through the dirty bit) so the UI
// redraws only when output actually changes — an idle child costs zero redraws.
// When the child exits, exited is set so the View can show "[exited]" while the
// game keeps running.
type leftPane struct {
	cmd  *exec.Cmd
	ptmx *os.File
	term vt10x.Terminal
	grid *vtGridAdapter
	name string // short command name shown in the window tab list

	notify func() // push a redraw to the bubbletea program (may be nil in tests)

	dirty    int32 // atomic: 1 when new output is pending a redraw (coalescing gate)
	exited   int32 // atomic: 1 when the child has exited
	exitCode int32 // atomic: child exit code, or -1 if unknown

	// notifyMu guards the frame-rate redraw gate (lastNotify/notifyTimer) so a
	// burst of output pushes at most one redraw per frameInterval.
	notifyMu    sync.Mutex
	lastNotify  time.Time
	notifyTimer *time.Timer

	// mu serializes operations that touch the PTY fd lifecycle (Write/Resize
	// vs Close) so a Close from the signal handler goroutine cannot race a
	// Resize/Write on the bubbletea goroutine. Once closed is set, fd ops are
	// skipped. lastCols/lastRows cache the most recently applied PTY size so a
	// Resize to the same dimensions skips the pty.Setsize ioctl (and its
	// SIGWINCH/full-redraw storm) — at the drag clamp boundary many mouse columns
	// map to the same left width, so without this every motion respammed an
	// identical resize. -1 means "not yet sized".
	mu       sync.Mutex
	closed   bool
	lastCols int
	lastRows int

	// resizeCalls counts how many times Resize actually issued a pty.Setsize
	// (i.e. a real size change), so tests can assert the drag coalescing and the
	// unchanged-size early-return bound the ioctl count.
	resizeCalls int32
}

// newLeftPane spawns argv (or $SHELL when empty) under a PTY sized cols x rows
// and starts the reader goroutine. notify is invoked (off the bubbletea
// goroutine) whenever new output should trigger a redraw; pass nil to disable.
func newLeftPane(argv []string, cols, rows int, notify func()) (*leftPane, error) {
	if len(argv) == 0 {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		argv = []string{shell}
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}

	term := vt10x.New(vt10x.WithSize(cols, rows))

	lp := &leftPane{
		cmd:      cmd,
		ptmx:     ptmx,
		term:     term,
		name:     filepath.Base(argv[0]),
		notify:   notify,
		exitCode: -1,
		lastCols: cols,
		lastRows: rows,
	}
	lp.grid = newVTGrid(term)

	go lp.readLoop()
	return lp, nil
}

// markDirty sets the dirty bit and pushes a redraw, but only on the 0->1
// transition so a burst of output collapses into a single pending redraw until
// the UI consumes it via ConsumeDirty. The push itself is frame-rate gated (see
// scheduleNotify) so a steady stream renders ~once per frame, not per chunk.
func (lp *leftPane) markDirty() {
	if atomic.SwapInt32(&lp.dirty, 1) != 0 {
		return
	}
	if lp.notify == nil {
		return
	}
	lp.scheduleNotify()
}

// scheduleNotify pushes a redraw now if at least frameInterval has elapsed since
// the last one, otherwise arms a single timer to push at the frame boundary.
// Bursts within a frame share that one pending timer. notify() is always called
// with the lock released so a (potentially blocking) send can't stall the gate.
func (lp *leftPane) scheduleNotify() {
	lp.notifyMu.Lock()
	if lp.notifyTimer != nil { // a push is already pending this frame
		lp.notifyMu.Unlock()
		return
	}
	wait := frameInterval - time.Since(lp.lastNotify)
	if wait <= 0 {
		lp.lastNotify = time.Now()
		lp.notifyMu.Unlock()
		lp.notify()
		return
	}
	lp.notifyTimer = time.AfterFunc(wait, func() {
		lp.notifyMu.Lock()
		lp.lastNotify = time.Now()
		lp.notifyTimer = nil
		lp.notifyMu.Unlock()
		lp.notify()
	})
	lp.notifyMu.Unlock()
}

// readLoop drains the PTY into the vt10x terminal until EOF, pushing a redraw
// after each chunk. On EOF it records the exit code, sets exited, and pushes
// one final redraw (the "render once more after child EOF" rule).
//
// IMPORTANT: the blocking Read happens here, OUTSIDE the vt lock; only
// term.Write (which processes an in-memory slice and returns immediately) takes
// the lock. We must NOT use term.Parse, which keeps the vt lock held while it
// blocks on the next read — that would freeze the whole UI (RenderRows blocks
// on the same lock) whenever the child is idle, e.g. an agent "thinking".
func (lp *leftPane) readLoop() {
	buf := make([]byte, 4096)
	var carry []byte // bytes of an incomplete trailing UTF-8 rune held for the next read
	for {
		n, err := lp.ptmx.Read(buf)
		if n > 0 {
			// Reassemble a multibyte (CJK/Hangul) glyph split across this and the
			// previous read: prepend carried bytes and hold back any new incomplete
			// trailing rune, so vt10x.Write only ever sees whole runes. Without this
			// a Hangul syllable straddling the 4 KB read boundary is silently
			// dropped (vt10x.Write returns short on a mid-rune slice; the count was
			// discarded here, so those bytes were lost — the primary user types
			// Korean, so this was a real, intermittent corruption).
			var data []byte
			data, carry = splitRunes(carry, buf[:n])
			if len(data) > 0 {
				_, _ = lp.term.Write(data)
				lp.markDirty()
			}
		}
		if err != nil {
			// Flush any half-rune left at EOF rather than dropping it silently.
			if len(carry) > 0 {
				_, _ = lp.term.Write(carry)
				lp.markDirty()
			}
			break
		}
	}
	if err := lp.cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			atomic.StoreInt32(&lp.exitCode, int32(ee.ExitCode()))
		}
	} else {
		atomic.StoreInt32(&lp.exitCode, 0)
	}
	atomic.StoreInt32(&lp.exited, 1)
	lp.markDirty()
}

// splitRunes joins any carried partial-rune bytes with the freshly-read incoming
// bytes and splits the result into a prefix safe to feed to the emulator (it
// ends on a UTF-8 rune boundary) plus a new carry holding an incomplete trailing
// rune (≤ utf8.UTFMax-1 bytes) to prepend next time. The common path — carry
// empty and incoming ending on a boundary (ASCII, complete glyphs, ESC
// sequences, which are all single-byte runes) — returns the incoming slice as-is
// with no allocation. Only a genuine mid-rune split allocates the small carry.
func splitRunes(carry, incoming []byte) (write, newCarry []byte) {
	data := incoming
	if len(carry) > 0 {
		data = append(append([]byte(nil), carry...), incoming...)
	}
	if k := incompleteTrailingRune(data); k > 0 {
		return data[:len(data)-k], append([]byte(nil), data[len(data)-k:]...)
	}
	return data, nil
}

// incompleteTrailingRune returns how many bytes at the end of b begin a
// multibyte UTF-8 rune that is not complete yet, or 0 when b ends on a rune
// boundary (including when the trailing bytes are an already-complete, even if
// invalid, encoding). At most utf8.UTFMax-1.
func incompleteTrailingRune(b []byte) int {
	for i := 1; i <= utf8.UTFMax && i <= len(b); i++ {
		if utf8.RuneStart(b[len(b)-i]) {
			if utf8.FullRune(b[len(b)-i:]) {
				return 0
			}
			return i
		}
	}
	return 0
}

// Name returns the short command name shown in the window tab list.
func (lp *leftPane) Name() string { return lp.name }

// ConsumeDirty clears the dirty bit so the next chunk of output re-notifies.
// Called by the UI when it processes a ptyMsg.
func (lp *leftPane) ConsumeDirty() { atomic.StoreInt32(&lp.dirty, 0) }

// Dirty reports whether a redraw is pending (without clearing it).
func (lp *leftPane) Dirty() bool { return atomic.LoadInt32(&lp.dirty) == 1 }

// Exited reports whether the child process has exited.
func (lp *leftPane) Exited() bool { return atomic.LoadInt32(&lp.exited) == 1 }

// ExitCode returns the child's exit code once exited, or -1 if unknown.
func (lp *leftPane) ExitCode() int { return int(atomic.LoadInt32(&lp.exitCode)) }

// Write forwards raw bytes to the PTY stdin (the child echoes them).
//
// The blocking ptmx.Write happens OUTSIDE lp.mu: a child that isn't draining its
// stdin (paused process, a large paste) makes that write block, and holding the
// lock across it would wedge Close()/Resize() — the shutdown deadlock that
// orphans the child on quit. We snapshot the fd under the lock instead; if Close
// closes the fd concurrently, a parked or subsequent write just returns an error
// (which we discard), and closing the fd actively unblocks an in-flight write.
func (lp *leftPane) Write(b []byte) {
	if lp.Exited() {
		return
	}
	lp.mu.Lock()
	closed, ptmx := lp.closed, lp.ptmx
	lp.mu.Unlock()
	if closed || ptmx == nil {
		return
	}
	_, _ = ptmx.Write(b)
}

// Resize resizes both the PTY and the vt10x grid. It is a no-op once the child
// has exited (the fd may be closed and there is nothing to reflow), which lets
// the redraw loop stay quiescent per the near-zero-idle-CPU goal.
func (lp *leftPane) Resize(cols, rows int) {
	if lp.Exited() {
		return
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	lp.mu.Lock()
	// Skip the ioctl when the requested size matches the last applied one: an
	// identical Setsize still fires a SIGWINCH and a full child redraw, and at the
	// drag clamp boundary many mouse columns collapse to the same left width, so
	// without this an in-place drag spams redundant resizes.
	if lp.lastCols == cols && lp.lastRows == rows {
		lp.mu.Unlock()
		return
	}
	if !lp.closed {
		_ = pty.Setsize(lp.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
		atomic.AddInt32(&lp.resizeCalls, 1)
	}
	lp.lastCols = cols
	lp.lastRows = rows
	lp.mu.Unlock()
	// vt10x.Terminal.Resize self-locks the same (non-reentrant) mutex that
	// RenderRows takes via Lock(), so it must NOT be wrapped in another Lock()
	// or it self-deadlocks (which would freeze the UI on the first resize).
	lp.term.Resize(cols, rows)
	// NOTE: do NOT markDirty()/notify() here. Resize is only ever called from
	// the bubbletea Update goroutine (handleResize), which already triggers a
	// View render when it returns. Calling notify -> Program.Send from the
	// event-loop goroutine would block forever (the loop can't receive while
	// it is inside Update), deadlocking the UI. Push notifications come only
	// from the reader goroutine.
}

// ResizeCalls returns how many real pty.Setsize ioctls Resize has issued (size
// changes only; unchanged-size requests are skipped). Used by tests to assert
// the drag coalescing and the unchanged-size early-return bound the ioctl count.
func (lp *leftPane) ResizeCalls() int { return int(atomic.LoadInt32(&lp.resizeCalls)) }

// CursorPos returns the child's vt cursor cell (grid column/row) and whether it
// is currently shown. Snapshotted under the terminal lock, mirroring RenderRows.
// Grid cell index equals display column (the patched vt10x reserves a trailing
// cell after each wide glyph), so callers can treat x as an exact column offset.
func (lp *leftPane) CursorPos() (x, y int, visible bool) {
	lp.term.Lock()
	c := lp.term.Cursor()
	vis := lp.term.CursorVisible()
	lp.term.Unlock()
	return c.X, c.Y, vis
}

// BracketedPaste reports whether the child has enabled bracketed paste mode
// (DEC private 2004). When true, pasted input forwarded to the PTY must be
// wrapped in ESC[200~ ... ESC[201~ so the child can distinguish a paste (e.g. a
// dragged-in image path, or a multi-line block) from individually typed keys.
// Snapshotted under the terminal lock, mirroring CursorPos/RenderRows.
func (lp *leftPane) BracketedPaste() bool {
	lp.term.Lock()
	on := lp.term.Mode()&vt10x.ModeBracketedPaste != 0
	lp.term.Unlock()
	return on
}

// RenderRows snapshots the grid under lock and renders it into width x height
// clipped/padded string rows. focused controls cursor visibility.
func (lp *leftPane) RenderRows(width, height int, focused bool) []string {
	lp.term.Lock()
	rows := vtrender.Render(lp.grid, vtrender.Options{
		Width:      width,
		Height:     height,
		Styled:     true,
		ShowCursor: focused,
	})
	lp.term.Unlock()
	return rows
}

// Close terminates the child and closes the PTY. It is idempotent and safe to
// call concurrently with Write/Resize (it serializes via mu and the closed
// flag), so the signal handler and the main loop can both call it.
func (lp *leftPane) Close() {
	lp.mu.Lock()
	if lp.closed {
		lp.mu.Unlock()
		return
	}
	lp.closed = true
	cmd, ptmx := lp.cmd, lp.ptmx
	lp.mu.Unlock()

	// Cancel any pending frame-gated redraw so it cannot fire after shutdown.
	lp.notifyMu.Lock()
	if lp.notifyTimer != nil {
		lp.notifyTimer.Stop()
		lp.notifyTimer = nil
	}
	lp.notifyMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
}
