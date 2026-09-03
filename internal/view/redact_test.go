package view

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"boardgame/kittens/internal/games/kittens/game"
	"boardgame/kittens/internal/prng"
)

// cardIDRe finds the numeric card IDs in a serialised view. Seat and player IDs
// are strings ("p1"), so this only ever matches actual cards.
var cardIDRe = regexp.MustCompile(`"id":(\d+)`)

func seats(n int) ([]game.Seat, []Membership) {
	var gs []game.Seat
	var ms []Membership
	for i := 0; i < n; i++ {
		id := "p" + strconv.Itoa(i)
		gs = append(gs, game.Seat{ID: id, Name: id})
		ms = append(ms, Membership{ID: id, Name: id, Connected: true, Host: i == 0})
	}
	return gs, ms
}

// TestViewNeverLeaksHiddenCards is the automated form of "watch the WebSocket
// frames and check nothing private goes out". It walks a game through many
// random states and asserts that the only card IDs a player can see are their
// own hand, the top of the discard, and the face-up cards of an open Nope
// window. Crucially the draw pile's *order* must never appear.
// Both decks are walked. The expansion is the one that puts the top of the draw
// pile in front of a player on purpose — Alter the Future — so it is exactly the
// deck where a leak would be easiest to introduce. Those faces carry no card IDs,
// which is what lets this scan stay as strict as it is.
func TestViewNeverLeaksHiddenCards(t *testing.T) {
	for _, variant := range []game.Variant{game.Original, game.Imploding} {
		t.Run(string(variant), func(t *testing.T) { leakScan(t, variant) })
	}
}

func leakScan(t *testing.T, variant game.Variant) {
	t.Helper()
	sawAlter := false
	for seed := int64(0); seed < 60; seed++ {
		rng := prng.New(uint64(seed))
		n := 2 + int(seed)%(game.MaxPlayersFor(variant)-1)
		gs, ms := seats(n)
		s, err := game.NewGame(gs, variant, rng)
		if err != nil {
			t.Fatal(err)
		}

		for step := 0; step < 400 && s.Phase != game.PhaseGameOver; step++ {
			if _, err := game.Apply(s, anyLegalMove(s, rng)); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if s.Phase == game.PhaseAlter {
				sawAlter = true
			}

			for _, m := range ms {
				v := For("ABCD", ms, s, m.ID, Countdown{}, nil)
				blob, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}

				allowed := map[int]bool{}
				if p := s.Find(m.ID); p != nil {
					for _, c := range p.Hand {
						allowed[c.ID] = true
					}
				}
				if top := s.DiscardTop(); top != nil {
					allowed[top.ID] = true
				}
				if s.Phase == game.PhaseNope && s.Pending != nil {
					for _, c := range s.Pending.Cards {
						allowed[c.ID] = true
					}
				}

				// The one card of the deck order anybody may be told about, and
				// only while it is face up on top. Checked here rather than only in
				// a focused test because this walk reaches states no hand-built one
				// would — the kitten buried, resurfacing, drawn again.
				if v.DeckTop != nil {
					top := s.TopFaceUp()
					if top == nil {
						t.Fatalf("seed %d step %d: the view names a deck top that is not face up\n%s",
							seed, step, blob)
					}
					if v.DeckTop.Slug != top.Slug {
						t.Fatalf("seed %d step %d: the view names %q on top, the deck has %q",
							seed, step, v.DeckTop.Slug, top.Slug)
					}
				} else if top := s.TopFaceUp(); top != nil {
					t.Fatalf("seed %d step %d: %q is face up on top and the view hides it",
						seed, step, top.Slug)
				}

				for _, match := range cardIDRe.FindAllStringSubmatch(string(blob), -1) {
					id, _ := strconv.Atoi(match[1])
					if !allowed[id] {
						t.Fatalf("seed %d step %d: view for %s exposes card %d it must not see\n%s",
							seed, step, m.ID, id, blob)
					}
				}
			}
		}
	}

	// A scan that never reaches the phase where the deck is shown to somebody
	// proves nothing about it, and would go quiet if the driver stopped playing
	// the card.
	if variant == game.Imploding && !sawAlter {
		t.Error("the scan never reached Alter the Future, so it did not test it")
	}
}

// The armed Imploding Kitten is disclosed only while it is the top card. Buried,
// the table is told it is armed and nothing more — knowing which card is second
// from the top would be a bigger edge than any card in the game gives.
func TestArmedKittenIsNamedOnlyOnTop(t *testing.T) {
	gs, ms := seats(3)
	s, err := game.NewGame(gs, game.Imploding, prng.New(uint64(11)))
	if err != nil {
		t.Fatal(err)
	}

	at := -1
	for i, c := range s.Draw {
		if c.Type == game.ImplodingKitten {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("the expansion deck was dealt without its Imploding Kitten")
	}
	s.Draw[at].FaceUp = true

	// Buried: second from the top, which is where it is most tempting to leak.
	s.Draw[0], s.Draw[at] = s.Draw[at], s.Draw[0]
	s.Draw[0], s.Draw[1] = s.Draw[1], s.Draw[0]

	v := For("ABCD", ms, s, "p1", Countdown{}, nil)
	if !v.ImplodingArmed {
		t.Error("a face-up kitten in the deck is not reported as armed")
	}
	if v.DeckTop != nil {
		t.Errorf("a buried kitten is named on the deck as %q", v.DeckTop.Slug)
	}

	// Surfaced.
	s.Draw[0], s.Draw[1] = s.Draw[1], s.Draw[0]

	v = For("ABCD", ms, s, "p1", Countdown{}, nil)
	if v.DeckTop == nil {
		t.Fatal("the kitten is face up on top and the deck says nothing")
	}
	if v.DeckTop.Slug != "imploding" {
		t.Errorf("the deck top is %q, want the imploding kitten", v.DeckTop.Slug)
	}
	// No ID, or the scan above would have something to catch.
	if strings.Contains(mustJSON(t, v.DeckTop), `"id"`) {
		t.Error("the deck top carries a card ID")
	}
}

func mustJSON(t *testing.T, x any) string {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLobbyViewHasNoCards(t *testing.T) {
	_, ms := seats(3)
	blob, err := json.Marshal(Lobby("ABCD", ms, "p0"))
	if err != nil {
		t.Fatal(err)
	}
	if cardIDRe.Match(blob) {
		t.Errorf("lobby view contains cards: %s", blob)
	}
}

func TestSeatsHideOtherHandsButKeepCounts(t *testing.T) {
	gs, ms := seats(3)
	s, err := game.NewGame(gs, game.Original, prng.New(uint64(7)))
	if err != nil {
		t.Fatal(err)
	}
	v := For("ABCD", ms, s, "p1", Countdown{}, nil)

	if len(v.Me.Hand) != 8 {
		t.Errorf("own hand = %d cards, want 8", len(v.Me.Hand))
	}
	for _, seat := range v.Seats {
		if seat.HandCount != 8 {
			t.Errorf("seat %s handCount = %d, want 8", seat.ID, seat.HandCount)
		}
	}
	if v.DeckCount != s.DeckSize() {
		t.Errorf("deckCount = %d, want %d", v.DeckCount, s.DeckSize())
	}
}

// The odds readout is only honest if the count tracks the deck, so check it at
// the deal and after an elimination has taken a Kitten out of the game.
func TestKittensLeftTracksTheDeck(t *testing.T) {
	gs, ms := seats(4)
	rng := prng.New(uint64(3))
	s, err := game.NewGame(gs, game.Original, rng)
	if err != nil {
		t.Fatal(err)
	}
	if got := For("ABCD", ms, s, "p0", Countdown{}, nil).KittensLeft; got != 3 {
		t.Fatalf("kittensLeft at the deal = %d, want players-1 = 3", got)
	}

	for step := 0; step < 400 && s.Phase != game.PhaseGameOver; step++ {
		if _, err := game.Apply(s, anyLegalMove(s, rng)); err != nil {
			t.Fatal(err)
		}
		v := For("ABCD", ms, s, "p0", Countdown{}, nil)
		dead := 0
		for _, p := range s.Players {
			if !p.Alive {
				dead++
			}
		}
		if want := 3 - dead; v.KittensLeft != want {
			t.Fatalf("step %d: kittensLeft = %d with %d eliminated, want %d",
				step, v.KittensLeft, dead, want)
		}
		if v.KittensLeft > v.DeckCount && s.PendingKitten == nil {
			t.Fatalf("step %d: %d kittens in a %d card deck", step, v.KittensLeft, v.DeckCount)
		}
	}
}

// anyLegalMove mirrors the engine test's random driver, kept local so the two
// packages stay independent.
func anyLegalMove(s *game.State, rng *prng.Source) game.Action {
	switch s.Phase {
	case game.PhaseDefuse:
		return game.Action{Kind: game.ActPlaceKitten, PlayerID: s.CurrentID(), Index: rng.Intn(s.DeckSize() + 1)}
	case game.PhaseFavor:
		id := s.AwaitingGiftFrom()
		p := s.Find(id)
		return game.Action{Kind: game.ActGiveCard, PlayerID: id, CardIDs: []int{p.Hand[rng.Intn(len(p.Hand))].ID}}
	case game.PhaseAlter:
		return game.Action{
			Kind: game.ActAlterFuture, PlayerID: s.AlteringID(),
			Order: rng.Perm(len(s.AlterFaces())),
		}
	case game.PhaseNope:
		return game.Action{Kind: game.ActNopeExpired}
	}

	p := s.Find(s.CurrentID())

	// A cat set, when one is held: a three-card demand is the only move that puts
	// a card name on the wire, so the leak scan must cover it.
	if a, ok := catSetMove(s, p, rng); ok {
		return a
	}
	if rng.Intn(2) == 0 {
		for _, c := range p.Hand {
			switch c.Slug {
			// "alter" is the one that matters here: it is the only card that puts
			// the deck in front of somebody, so the scan has to reach that phase.
			case "skip", "attack", "shuffle", "future", "reverse", "bottom", "alter":
				return game.Action{Kind: game.ActPlay, PlayerID: p.ID, CardIDs: []int{c.ID}}
			}
		}
	}
	return game.Action{Kind: game.ActDraw, PlayerID: p.ID}
}

func catSetMove(s *game.State, p *game.Player, rng *prng.Source) (game.Action, bool) {
	groups := map[string][]int{}
	for _, c := range p.Hand {
		if strings.HasPrefix(c.Slug, "cat-") {
			groups[c.Slug] = append(groups[c.Slug], c.ID)
		}
	}
	var target string
	for _, q := range s.Players {
		if q.Alive && q.ID != p.ID {
			target = q.ID
			break
		}
	}
	if target == "" {
		return game.Action{}, false
	}
	for _, ids := range groups {
		if len(ids) >= 3 {
			return game.Action{
				Kind: game.ActPlay, PlayerID: p.ID, CardIDs: ids[:3],
				TargetID: target, Named: "defuse",
			}, true
		}
		if len(ids) == 2 {
			return game.Action{
				Kind: game.ActPlay, PlayerID: p.ID, CardIDs: ids[:2], TargetID: target,
			}, true
		}
	}
	return game.Action{}, false
}
