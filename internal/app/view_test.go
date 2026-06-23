package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"tetmux/internal/layout"
	"tetmux/internal/tetris"
)

// TestRenderRightPreviewGate sweeps the right pane inner width across the
// HOLD/NEXT side-panel threshold and asserts the panel appears exactly when it
// fits, while every rendered line stays inner-width + 2 border columns wide.
func TestRenderRightPreviewGate(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 120, 40
	const gate = tetris.Width*2 + previewGap + previewCols // 30
	for _, w := range []int{gate - 1, gate, gate + 1} {
		l := layout.Layout{RightInnerWidth: w, InnerHeight: tetris.Height + 2}
		out := m.renderRight(l)
		hasPanel := strings.Contains(out, "NEXT")
		if want := w >= gate; hasPanel != want {
			t.Errorf("w=%d: side panel present=%v want %v", w, hasPanel, want)
		}
		for i, line := range strings.Split(out, "\n") {
			if gw := lipgloss.Width(line); gw != w+2 {
				t.Errorf("w=%d line %d width=%d want %d", w, i, gw, w+2)
			}
		}
	}
}

// TestRenderPieceBoxWidths pins the preview box geometry so it can't drift past
// the gate it is sized against.
func TestRenderPieceBoxWidths(t *testing.T) {
	r := lipgloss.NewRenderer(os.Stdout)
	for _, tc := range []struct {
		name    string
		present bool
	}{{"filled", true}, {"empty", false}} {
		rows := renderPieceBox("NEXT", tetris.O, tc.present, r)
		if len(rows) != 5 {
			t.Fatalf("%s: rows=%d want 5 (label + 4 grid)", tc.name, len(rows))
		}
		if lw := lipgloss.Width(rows[0]); lw > previewCols {
			t.Errorf("%s: label width=%d > previewCols=%d", tc.name, lw, previewCols)
		}
		for i := 1; i < 5; i++ {
			if gw := lipgloss.Width(rows[i]); gw != previewCols {
				t.Errorf("%s: grid row %d width=%d want %d", tc.name, i, gw, previewCols)
			}
		}
	}
}

// TestViewSmoke exercises the full composition (including the spawn-error path)
// and asserts the frame is exactly terminal-height lines, the status bar spans
// the full width, and the focus indicator is present.
func TestViewSmoke(t *testing.T) {
	m := New([]string{"no-such-command-xyz"}, 1)
	m.width, m.height = 80, 24
	m.spawnErr = errors.New("boom")

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("View produced %d lines, want %d", len(lines), m.height)
	}
	last := lines[len(lines)-1]
	if lipgloss.Width(last) != m.width {
		t.Errorf("status line width=%d want %d", lipgloss.Width(last), m.width)
	}
	if !strings.Contains(last, "focus:LEFT") {
		t.Errorf("status line missing focus indicator: %q", last)
	}
}

// TestViewInitializing covers the pre-size guard.
func TestViewInitializing(t *testing.T) {
	m := New(nil, 1)
	if got := m.View(); got != "initializing tetmux..." {
		t.Errorf("zero-size View = %q", got)
	}
}
