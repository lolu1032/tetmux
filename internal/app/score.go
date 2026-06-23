package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// scoreFileName is the high-score file under the state dir.
const scoreFileName = "tetmux/highscore"

// bestScorePath returns where the best score is stored, following XDG: it
// prefers $XDG_STATE_HOME, then ~/.local/state, and finally the OS cache dir.
// Returns "" if no suitable location can be determined (persistence is then a
// no-op — high scores are a nicety, never a hard dependency).
func bestScorePath() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, scoreFileName)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state", scoreFileName)
	}
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, scoreFileName)
	}
	return ""
}

// loadBestScore reads the persisted best score, or 0 if none/unreadable.
func loadBestScore() int {
	p := bestScorePath()
	if p == "" {
		return 0
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// saveBestScore writes n if it beats whatever is already stored. All errors are
// ignored: failing to persist a high score must never disrupt the program.
func saveBestScore(n int) {
	if n <= 0 {
		return
	}
	p := bestScorePath()
	if p == "" {
		return
	}
	if n <= loadBestScore() {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(strconv.Itoa(n)), 0o644)
}
