package app

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"

	"tetmux/internal/vtrender"
)

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

	notify func() // push a redraw to the bubbletea program (may be nil in tests)

	dirty    int32 // atomic: 1 when new output is pending a redraw (coalescing gate)
	exited   int32 // atomic: 1 when the child has exited
	exitCode int32 // atomic: child exit code, or -1 if unknown

	// mu serializes operations that touch the PTY fd lifecycle (Write/Resize
	// vs Close) so a Close from the signal handler goroutine cannot race a
	// Resize/Write on the bubbletea goroutine. Once closed is set, fd ops are
	// skipped.
	mu     sync.Mutex
	closed bool
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
		notify:   notify,
		exitCode: -1,
	}
	lp.grid = newVTGrid(term)

	go lp.readLoop()
	return lp, nil
}

// markDirty sets the dirty bit and pushes a redraw, but only on the 0->1
// transition so a burst of output collapses into a single pending redraw until
// the UI consumes it via ConsumeDirty.
func (lp *leftPane) markDirty() {
	if atomic.SwapInt32(&lp.dirty, 1) == 0 && lp.notify != nil {
		lp.notify()
	}
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
	for {
		n, err := lp.ptmx.Read(buf)
		if n > 0 {
			_, _ = lp.term.Write(buf[:n])
			lp.markDirty()
		}
		if err != nil {
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
func (lp *leftPane) Write(b []byte) {
	if lp.Exited() {
		return
	}
	lp.mu.Lock()
	if !lp.closed {
		_, _ = lp.ptmx.Write(b)
	}
	lp.mu.Unlock()
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
	if !lp.closed {
		_ = pty.Setsize(lp.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
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

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
}
