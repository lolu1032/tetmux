package vt10x

import (
	"testing"
	"time"
)

// fuzzSeeds are byte sequences most likely to misbehave when parsed: huge CSI
// repeat counts (the CHT/CBT CPU-DoS), the bounded-internally repeat ops, OSC
// queries, line-drawing/gfx mode, truecolor SGR, and CJK. They double as the
// corpus `go test` replays (so plain CI exercises them without -fuzz).
var fuzzSeeds = []string{
	"\x1b[999999999I",     // CHT huge repeat (must be clamped)
	"\x1b[999999999Z",     // CBT huge repeat (must be clamped)
	"\x1b[999999999@",     // ICH
	"\x1b[999999999P",     // DCH
	"\x1b[999999999S",     // SU
	"\x1b[999999999T",     // SD
	"\x1b[999999999X",     // ECH
	"\x1b[999999999L",     // IL
	"\x1b[999999999M",     // DL
	"\x1b]11;?\x07",       // OSC background-color query
	"\x1b(0qqqx\x1b(B",    // line-drawing (gfx) mode then back
	"\x1b[38;2;255;0;0mX", // truecolor SGR
	"안녕하세요",               // CJK (Hangul)
	"\x1b[1;1H\x1b[2J",    // cursor home + clear
}

// FuzzParse feeds arbitrary bytes to the emulator and asserts it neither panics
// nor leaves the grid in a state where reading a cell panics. Hangs are caught
// by the go fuzzer's own per-input deadline; the explicit clamp test below pins
// the CPU-DoS regression deterministically for plain CI.
func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		term := New(WithSize(80, 24))
		_, _ = term.Write(data)
		// Touch every cell: a write that corrupted bounds would panic here.
		cols, rows := term.Size()
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				_ = term.Cell(x, y)
			}
		}
	})
}

// TestCSIHugeRepeatDoesNotHang is the deterministic guard for the clamp in
// csi.go: an unbounded CHT/CBT (or other) repeat count must not spin the parser.
func TestCSIHugeRepeatDoesNotHang(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		term := New(WithSize(80, 24))
		_, _ = term.Write([]byte("\x1b[999999999I\x1b[999999999Z\x1b[999999999@\x1b[999999999P"))
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CSI with a huge repeat count hung — the DoS clamp is missing/ineffective")
	}
}
