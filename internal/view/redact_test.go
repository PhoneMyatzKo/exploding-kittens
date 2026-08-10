package view

import (
	"encoding/json"
	"math/rand"
	"regexp"
	"strconv"
	"testing"

	"boardgame/kittens/internal/game"
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
func TestViewNeverLeaksHiddenCards(t *testing.T) {
	for seed := int64(0); seed < 60; seed++ {
		rng := rand.New(rand.NewSource(seed))
		n := 2 + int(seed)%4
		gs, ms := seats(n)
		s, err := game.NewGame(gs, rng)
		if err != nil {
			t.Fatal(err)
		}

		for step := 0; step < 400 && s.Phase != game.PhaseGameOver; step++ {
			if _, err := game.Apply(s, anyLegalMove(s, rng)); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}

			for _, m := range ms {
				v := For("ABCD", ms, s, m.ID, 0, nil)
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
	s, err := game.NewGame(gs, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	v := For("ABCD", ms, s, "p1", 0, nil)

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

// anyLegalMove mirrors the engine test's random driver, kept local so the two
// packages stay independent.
func anyLegalMove(s *game.State, rng *rand.Rand) game.Action {
	switch s.Phase {
	case game.PhaseDefuse:
		return game.Action{Kind: game.ActPlaceKitten, PlayerID: s.CurrentID(), Index: rng.Intn(s.DeckSize() + 1)}
	case game.PhaseFavor:
		id := s.AwaitingGiftFrom()
		p := s.Find(id)
		return game.Action{Kind: game.ActGiveCard, PlayerID: id, CardIDs: []int{p.Hand[rng.Intn(len(p.Hand))].ID}}
	case game.PhaseNope:
		return game.Action{Kind: game.ActNopeExpired}
	}

	p := s.Find(s.CurrentID())
	if rng.Intn(2) == 0 {
		for _, c := range p.Hand {
			switch c.Slug {
			case "skip", "attack", "shuffle", "future":
				return game.Action{Kind: game.ActPlay, PlayerID: p.ID, CardIDs: []int{c.ID}}
			}
		}
	}
	return game.Action{Kind: game.ActDraw, PlayerID: p.ID}
}
