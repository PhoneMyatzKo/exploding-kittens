// Package prng is the randomness every game deals from.
//
// It exists for one reason: a game has to be able to stop and start again. A
// *math/rand.Rand cannot — its state is a large internal array with no way to
// write it down, so a game holding one can never be saved to disk and reloaded.
// That is the single obstacle between this server and surviving a restart, and
// it matters most for the longest games: losing ten minutes of Exploding Kittens
// to a deploy is a shrug, losing two hours of Monopoly is not.
//
// So: a counter-based generator. The whole state is two integers — the seed it
// started from and how many values it has produced — and the nth value is a hash
// of those two rather than a step in a chain. Two consequences:
//
//   - It marshals. A Source is a struct of two uint64s and nothing else, so a
//     game containing one is ordinary JSON.
//   - A bug report is two numbers. Given a seed and a count you can put the
//     generator back exactly where it was, or replay from the beginning and watch
//     the same game happen again.
//
// Deliberately *not* math/rand/v2's PCG, which also marshals: its state is an
// opaque sixteen-byte blob, which gives up the second property for nothing this
// codebase needs.
//
// Not safe for concurrent use, and it does not need to be — a game's randomness
// is only ever touched from its room's single goroutine.
package prng

import (
	cryptorand "crypto/rand"
	"encoding/binary"
)

// Source is a stream of pseudo-random values. The zero value works (seed 0) but
// New or NewSeeded is what callers want.
//
// Both fields are exported because being writable from outside is the point:
// restoring a saved game means setting them.
type Source struct {
	Seed  uint64 `json:"seed"`
	Count uint64 `json:"count"`
}

// New returns a Source that will always produce the same sequence.
func New(seed uint64) *Source { return &Source{Seed: seed} }

// NewSeeded returns a Source seeded unpredictably. crypto/rand rather than the
// clock: two rooms created in the same millisecond would otherwise deal the same
// cards, which is a real possibility when a test stands up several at once.
func NewSeeded() *Source {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any platform this runs on, and a game with
		// a predictable deck is worse than a game that refuses to start.
		panic("prng: no entropy available: " + err.Error())
	}
	return New(binary.LittleEndian.Uint64(b[:]))
}

// Clone is an independent copy, for a caller that wants to look ahead without
// disturbing the game's own stream.
func (s *Source) Clone() *Source { c := *s; return &c }

// next is the generator: the Count-th value is a hash of the seed and the
// counter, so nothing depends on the values before it.
//
// The multiplier is the golden-ratio constant that splitmix64 uses to walk its
// counter, and mix is splitmix64's finaliser — a well-studied bit mixer that
// passes the usual test suites. Neither is cryptographic and neither needs to be:
// this shuffles decks, it does not protect anything.
func (s *Source) next() uint64 {
	s.Count++
	return mix(s.Seed + s.Count*0x9E3779B97F4A7C15)
}

func mix(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Intn returns a value in [0, n). Panics for n <= 0, matching math/rand — a
// negative or zero bound is a caller's bug, and returning zero would hide it.
//
// Rejection rather than a plain modulo, so the low values are not very slightly
// more likely than the high ones. That makes the number of values consumed
// depend on what comes out, which would be fatal to a scheme that reconstructed
// the position by counting operations — and is harmless here, because Count is
// carried rather than inferred.
func (s *Source) Intn(n int) int {
	if n <= 0 {
		panic("prng: Intn requires a positive bound")
	}
	un := uint64(n)
	// The largest multiple of un that fits in a uint64. Anything at or above it
	// is in a partial block and would bias the result, so it is thrown away.
	limit := ^uint64(0) / un * un
	for {
		if v := s.next(); v < limit {
			return int(v % un)
		}
	}
}

// Shuffle permutes n items using the swap the caller provides. Fisher-Yates,
// written out rather than delegated, so that what a shuffle costs in values is
// this package's business and not the standard library's.
func (s *Source) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		swap(i, s.Intn(i+1))
	}
}

// Perm is the numbers [0, n) in a random order.
func (s *Source) Perm(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	s.Shuffle(n, func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
