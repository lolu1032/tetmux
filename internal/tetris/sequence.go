package tetris

import "math/rand"

// PieceSequence returns the first n piece kinds that a game seeded with the
// given seed would spawn. This drives the deterministic-sequence tests and is
// independent of any board interaction.
func PieceSequence(seed int64, n int) []PieceKind {
	rng := rand.New(rand.NewSource(seed))
	out := make([]PieceKind, 0, n)
	var bag []PieceKind
	idx := 0
	for len(out) < n {
		if idx >= len(bag) {
			bag = append([]PieceKind(nil), AllKinds...)
			rng.Shuffle(len(bag), func(i, j int) {
				bag[i], bag[j] = bag[j], bag[i]
			})
			idx = 0
		}
		out = append(out, bag[idx])
		idx++
	}
	return out
}
