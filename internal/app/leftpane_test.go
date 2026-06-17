package app

import (
	"sync"
	"testing"
	"time"
)

// TestMarkDirtyCoalesces verifies the push notification is sent once per
// dirty->consumed cycle: a burst of output collapses into a single redraw, and
// a new redraw only fires after the UI consumes the prior one. This is what
// keeps an idle child from spinning the render loop.
func TestMarkDirtyCoalesces(t *testing.T) {
	n := 0
	lp := &leftPane{notify: func() { n++ }, exitCode: -1}

	lp.markDirty()
	if n != 1 || !lp.Dirty() {
		t.Fatalf("after first markDirty: n=%d dirty=%v", n, lp.Dirty())
	}
	lp.markDirty() // coalesced: no second notify
	if n != 1 {
		t.Fatalf("coalescing failed: n=%d want 1", n)
	}
	lp.ConsumeDirty()
	if lp.Dirty() {
		t.Fatalf("dirty not cleared by ConsumeDirty")
	}
	lp.markDirty() // fresh cycle: notifies again
	if n != 2 {
		t.Fatalf("after consume+markDirty: n=%d want 2", n)
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
