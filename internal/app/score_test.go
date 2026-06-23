package app

import "testing"

func TestBestScoreRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got := loadBestScore(); got != 0 {
		t.Fatalf("fresh load=%d want 0", got)
	}
	saveBestScore(120)
	if got := loadBestScore(); got != 120 {
		t.Errorf("after save 120: load=%d", got)
	}
	saveBestScore(50) // lower than stored: must not overwrite
	if got := loadBestScore(); got != 120 {
		t.Errorf("lower save lowered best to %d, want 120", got)
	}
	saveBestScore(300)
	if got := loadBestScore(); got != 300 {
		t.Errorf("higher save: load=%d want 300", got)
	}
}

func TestSaveBestScoreIgnoresZero(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	saveBestScore(0)
	if got := loadBestScore(); got != 0 {
		t.Errorf("saving 0 should write nothing, load=%d", got)
	}
}
