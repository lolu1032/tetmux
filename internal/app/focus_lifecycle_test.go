package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tetmux/internal/router"
	"tetmux/internal/tetris"
)

// pauseMenuMarker is text that appears ONLY in the interactive pause overlay
// (renderPauseMenu), never in the plain board, so its presence in a render means
// the modal menu is being shown.
const pauseMenuMarker = "계속하기"

// TestUserPauseNotAutoResumedAndNoStuckModal covers user-visible failure (a):
// a USER pause (Esc/p) must stay paused across a focus change (never auto-resumed
// like an auto-pause), and the interactive pause modal must NEVER be drawn while
// the command pane is focused — otherwise it is an unreachable, undismissable
// overlay whose Resume/Restart keys route to the child instead.
func TestUserPauseNotAutoResumedAndNoStuckModal(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	tab := tea.KeyMsg{Type: tea.KeyTab}

	// FocusLeft -> FocusRight on a Playing game (no auto-pause, no auto-resume).
	drive(m, tab)
	if m.rstate.Focus != router.FocusRight || m.game.State() != tetris.Playing {
		t.Fatalf("after Tab: focus=%v state=%v want FocusRight/Playing", m.rstate.Focus, m.game.State())
	}

	// User pause with 'p' (game focused). This is a USER pause, so it must NOT be
	// flagged as automatic.
	drive(m, runeKey('p'))
	if m.game.State() != tetris.Paused {
		t.Fatalf("'p' should pause, state=%v", m.game.State())
	}
	if m.autoPaused {
		t.Errorf("a user pause must not be flagged auto-paused")
	}

	// Tab back to the command pane while paused. Coherence: focus is LEFT and the
	// game is still paused — but the interactive modal must NOT be rendered, since
	// its keys now route to the child. Before the fix renderRight showed the modal
	// regardless of focus, so this assertion failed (the stuck-modal of (a)).
	drive(m, tab)
	if m.rstate.Focus != router.FocusLeft {
		t.Fatalf("Tab should return focus to the command pane, got %v", m.rstate.Focus)
	}
	if m.game.State() != tetris.Paused {
		t.Errorf("user pause should still be paused after focus change, state=%v", m.game.State())
	}
	out := m.renderRight(m.layout())
	if strings.Contains(out, pauseMenuMarker) {
		t.Errorf("interactive pause modal (%q) must NOT be shown while the command pane is focused; right pane:\n%s", pauseMenuMarker, out)
	}
	// ...but a paused board must NOT masquerade as a live one either: the static
	// 일시정지 banner has to be visible even with the command pane focused, so the
	// user can tell at a glance the game they paused is actually stopped.
	if !strings.Contains(out, "일시정지") {
		t.Errorf("a paused-but-unfocused board must still show the static 일시정지 indicator (not a plain live-looking board); right pane:\n%s", out)
	}

	// Tab back to the game: a USER pause must NOT be auto-resumed (only an
	// auto-pause is). It should stay Paused, and now (game focused) show the modal.
	drive(m, tab)
	if m.rstate.Focus != router.FocusRight {
		t.Fatalf("Tab should focus the game, got %v", m.rstate.Focus)
	}
	if m.game.State() != tetris.Paused {
		t.Errorf("a USER pause must not be auto-resumed by a focus change, state=%v", m.game.State())
	}
	if m.autoPaused {
		t.Errorf("user pause must still not be flagged auto-paused after a focus round-trip")
	}
	if out := m.renderRight(m.layout()); !strings.Contains(out, pauseMenuMarker) {
		t.Errorf("with the game focused and paused, the pause modal (%q) should be shown; right pane:\n%s", pauseMenuMarker, out)
	}
}

// TestAutoPauseOnFocusOutAndAutoResumeOnFocusIn covers user-visible failure (b):
// Tabbing away from a Playing game auto-pauses it and stops gravity, and Tabbing
// back auto-resumes it and restarts the gravity loop. It asserts the
// (focus, gameState, gravity-cmd) triple at each step, for BOTH the bare Tab
// toggle and the C-b l / C-b h focus switches.
func TestAutoPauseOnFocusOutAndAutoResumeOnFocusIn(t *testing.T) {
	m := New(nil, 1)
	m.width, m.height = 100, 40
	tab := tea.KeyMsg{Type: tea.KeyTab}

	// Step A: focus the game (still Playing); no auto-pause/resume should fire.
	_, _ = m.Update(tab)
	if m.rstate.Focus != router.FocusRight || m.game.State() != tetris.Playing {
		t.Fatalf("step A: focus=%v state=%v want FocusRight/Playing", m.rstate.Focus, m.game.State())
	}

	// Step B: Tab away -> auto-pause; gravity must stop (no live tick rescheduled).
	_, cmd := m.Update(tab)
	if m.rstate.Focus != router.FocusLeft {
		t.Fatalf("step B: focus=%v want FocusLeft", m.rstate.Focus)
	}
	if m.game.State() != tetris.Paused { // BEFORE fix: still Playing -> fails, reproducing (b)
		t.Errorf("step B: focus-out of a Playing game should auto-pause, state=%v", m.game.State())
	}
	if !m.autoPaused {
		t.Errorf("step B: focus-out auto-pause should be flagged autoPaused")
	}
	if cmd != nil {
		t.Errorf("step B: focus-out must not keep the gravity loop alive (cmd should be nil)")
	}
	if m.startGravity() != nil {
		t.Errorf("step B: a paused game must not start gravity")
	}

	// Step C: Tab back -> auto-resume + gravity restart.
	_, cmd = m.Update(tab)
	if m.rstate.Focus != router.FocusRight {
		t.Fatalf("step C: focus=%v want FocusRight", m.rstate.Focus)
	}
	if m.game.State() != tetris.Playing {
		t.Errorf("step C: focus-in of an auto-paused game should auto-resume, state=%v", m.game.State())
	}
	if m.autoPaused {
		t.Errorf("step C: auto-resume should clear the autoPaused flag")
	}
	if cmd == nil {
		t.Errorf("step C: auto-resume should restart the gravity loop (non-nil cmd)")
	}

	// Now exercise the C-b l / C-b h focus switches (CmdSwitchFocusRight/Left),
	// not just the bare Tab toggle. The game is Playing and FocusRight here.
	// C-b h -> FocusLeft: auto-pause.
	drive(m, ctrlB(), runeKey('h'))
	if m.rstate.Focus != router.FocusLeft {
		t.Fatalf("C-b h: focus=%v want FocusLeft", m.rstate.Focus)
	}
	if m.game.State() != tetris.Paused || !m.autoPaused {
		t.Errorf("C-b h focus-out should auto-pause, state=%v autoPaused=%v", m.game.State(), m.autoPaused)
	}
	if m.startGravity() != nil {
		t.Errorf("C-b h: a paused game must not start gravity")
	}

	// C-b l -> FocusRight: auto-resume + gravity restart (assert via the Update cmd).
	_, cmd = m.Update(ctrlB())
	if cmd != nil {
		t.Fatalf("C-b prefix arm should not produce a cmd")
	}
	_, cmd = m.Update(runeKey('l'))
	if m.rstate.Focus != router.FocusRight {
		t.Fatalf("C-b l: focus=%v want FocusRight", m.rstate.Focus)
	}
	if m.game.State() != tetris.Playing || m.autoPaused {
		t.Errorf("C-b l focus-in should auto-resume, state=%v autoPaused=%v", m.game.State(), m.autoPaused)
	}
	if cmd == nil {
		t.Errorf("C-b l auto-resume should restart the gravity loop (non-nil cmd)")
	}
}
