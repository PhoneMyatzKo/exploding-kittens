package prng

import (
	"encoding/json"
	"math"
	"testing"
)

// The property the whole package exists for: a Source can be written down and
// picked up again, and the game carries on exactly as it would have.
func TestResumingFromAMarshalledSourceContinuesTheSequence(t *testing.T) {
	original := New(20260827)
	for i := 0; i < 500; i++ {
		original.Intn(52)
	}

	// Saved mid-stream, the way a room would be saved mid-game.
	blob, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored Source
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatal(err)
	}

	// Both carry on from here. If the restored one is even one value out, the
	// reloaded game deals a different card from the one it was about to.
	for i := 0; i < 500; i++ {
		want, got := original.Intn(52), restored.Intn(52)
		if want != got {
			t.Fatalf("value %d after restore is %d, want %d", i, got, want)
		}
	}
}

// And the state really is only those two numbers — a field added later that does
// not marshal would break resumption silently, so this pins the shape.
func TestSourceMarshalsAsSeedAndCount(t *testing.T) {
	s := New(7)
	for i := 0; i < 9; i++ {
		s.Intn(6)
	}
	blob, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(blob, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["seed"] == nil || fields["count"] == nil {
		t.Fatalf("a Source marshalled as %s — it must be exactly seed and count", blob)
	}
	if s.Count == 0 {
		t.Error("nine draws left the counter at zero")
	}
}

// Rebuilding from the seed and replaying reaches the same place. This is the
// "reproduce it from a bug report" property: two numbers are enough.
func TestReplayingFromTheSeedReachesTheSameState(t *testing.T) {
	live := New(99)
	for i := 0; i < 321; i++ {
		live.Intn(40)
	}

	replay := New(99)
	for replay.Count < live.Count {
		replay.Intn(40)
	}
	if *replay != *live {
		t.Fatalf("replay ended at %+v, live at %+v", *replay, *live)
	}
	// And jumping straight to the count, without replaying, is the same stream.
	jumped := &Source{Seed: 99, Count: live.Count}
	if a, b := jumped.Intn(40), live.Intn(40); a != b {
		t.Errorf("a jumped source gave %d where the live one gave %d", a, b)
	}
}

func TestSameSeedSameGame(t *testing.T) {
	a, b := New(5), New(5)
	for i := 0; i < 200; i++ {
		if x, y := a.Intn(13), b.Intn(13); x != y {
			t.Fatalf("two sources on seed 5 diverged at %d: %d vs %d", i, x, y)
		}
	}
}

func TestDifferentSeedsDifferentGames(t *testing.T) {
	a, b := New(1), New(2)
	same := 0
	for i := 0; i < 200; i++ {
		if a.Intn(1000) == b.Intn(1000) {
			same++
		}
	}
	// Some collisions are expected; agreement throughout would mean the seed is
	// being ignored.
	if same > 20 {
		t.Errorf("two different seeds agreed on %d of 200 values", same)
	}
}

func TestIntnStaysInRange(t *testing.T) {
	s := New(3)
	for _, n := range []int{1, 2, 3, 6, 7, 40, 52, 1000} {
		for i := 0; i < 2000; i++ {
			if v := s.Intn(n); v < 0 || v >= n {
				t.Fatalf("Intn(%d) returned %d", n, v)
			}
		}
	}
}

func TestIntnRejectsAnImpossibleBound(t *testing.T) {
	for _, n := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Intn(%d) did not panic", n)
				}
			}()
			New(1).Intn(n)
		}()
	}
}

// Even-handed enough to shuffle a deck with. Not a statistics suite — what this
// catches is a modulo that quietly favours the low values, which over a
// fifty-two card deck would put the same cards on top too often.
func TestIntnIsRoughlyUniform(t *testing.T) {
	const buckets, draws = 13, 130_000
	counts := make([]int, buckets)
	s := New(2026)
	for i := 0; i < draws; i++ {
		counts[s.Intn(buckets)]++
	}

	expected := float64(draws) / buckets
	for i, c := range counts {
		if off := math.Abs(float64(c)-expected) / expected; off > 0.05 {
			t.Errorf("bucket %d got %d, %.1f%% off an even %.0f", i, c, off*100, expected)
		}
	}
}

// A shuffle has to be a permutation: every card still there, none twice. The
// mistake this catches is an off-by-one in the Fisher-Yates bounds, which
// produces a shuffle that silently never moves the first or last item.
func TestShuffleIsAPermutation(t *testing.T) {
	s := New(11)
	for round := 0; round < 200; round++ {
		deck := make([]int, 52)
		for i := range deck {
			deck[i] = i
		}
		s.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

		seen := make([]bool, 52)
		for _, c := range deck {
			if c < 0 || c >= 52 || seen[c] {
				t.Fatalf("round %d: card %d is missing or duplicated: %v", round, c, deck)
			}
			seen[c] = true
		}
	}
}

// Every position has to be reachable by every card. A Fisher-Yates that loops
// `i > 0` but draws Intn(i) instead of Intn(i+1) leaves each card unable to stay
// where it started — a bias no permutation check would notice.
func TestShuffleCanMoveAnyCardAnywhere(t *testing.T) {
	const size = 8
	s := New(4)
	seen := make([][]bool, size)
	for i := range seen {
		seen[i] = make([]bool, size)
	}

	for round := 0; round < 20_000; round++ {
		deck := make([]int, size)
		for i := range deck {
			deck[i] = i
		}
		s.Shuffle(size, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		for pos, card := range deck {
			seen[card][pos] = true
		}
	}

	for card := range seen {
		for pos, ok := range seen[card] {
			if !ok {
				t.Errorf("card %d never landed in position %d", card, pos)
			}
		}
	}
}

func TestShuffleOfNothingIsSafe(t *testing.T) {
	s := New(1)
	s.Shuffle(0, func(i, j int) { t.Fatal("swapped in an empty shuffle") })
	s.Shuffle(1, func(i, j int) { t.Fatal("swapped in a one-item shuffle") })
}

func TestPermIsAPermutation(t *testing.T) {
	p := New(6).Perm(20)
	seen := make([]bool, 20)
	for _, v := range p {
		if v < 0 || v >= 20 || seen[v] {
			t.Fatalf("Perm returned %v", p)
		}
		seen[v] = true
	}
}

func TestCloneDoesNotDisturbTheOriginal(t *testing.T) {
	s := New(8)
	s.Intn(10)
	c := s.Clone()
	for i := 0; i < 50; i++ {
		c.Intn(10)
	}
	if want := uint64(1); s.Count != want {
		t.Errorf("the original advanced to %d, want %d", s.Count, want)
	}
}

func TestNewSeededIsNotAlwaysTheSameSeed(t *testing.T) {
	// Two rooms opened in the same instant must not deal the same cards, which
	// is what a clock-based seed would do.
	a, b := NewSeeded(), NewSeeded()
	if a.Seed == b.Seed {
		t.Error("two seeded sources got the same seed")
	}
}
