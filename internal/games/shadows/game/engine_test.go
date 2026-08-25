package game

import (
	"math/rand"
	"strings"
	"testing"
)

// table deals a game with a fixed seed so a failure is reproducible.
func table(t *testing.T, n int) *State {
	t.Helper()
	seats := make([]Seat, 0, n)
	for i := 0; i < n; i++ {
		seats = append(seats, Seat{ID: string(rune('a' + i)), Name: strings.ToUpper(string(rune('a' + i)))})
	}
	s, _, err := NewGame(seats, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	return s
}

// passRound spends every remaining main action as a pass, which is the cheapest
// way to get a test to the round it is actually about.
func passRound(t *testing.T, s *State) {
	t.Helper()
	guard := 0
	round := s.Round
	for s.Round == round && s.Phase != PhaseGameOver {
		if guard++; guard > 4*MaxPlayers {
			t.Fatal("round never ended")
		}
		if _, err := Apply(s, Action{Kind: ActPass, PlayerID: s.CurrentID()}); err != nil {
			t.Fatalf("pass: %v", err)
		}
	}
}

func find(events []Event, kind EventKind) *Event {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func TestPlayerCount(t *testing.T) {
	for _, n := range []int{0, 1, 4, 9, 12} {
		seats := make([]Seat, n)
		for i := range seats {
			seats[i] = Seat{ID: string(rune('a' + i))}
		}
		if _, _, err := NewGame(seats, rand.New(rand.NewSource(1))); err == nil {
			t.Fatalf("%d players should be refused", n)
		}
	}
	for n := MinPlayers; n <= MaxPlayers; n++ {
		table(t, n)
	}
}

func TestDealGivesPositionsAndSecrets(t *testing.T) {
	s := table(t, 6)
	seen := map[Role]int{}
	for _, p := range s.Players {
		seen[p.Role]++
		if p.Weakness.Kind != KindWeakness {
			t.Fatalf("%s has no secret", p.Name)
		}
		if len(p.Hand) != 0 {
			t.Fatal("no Power cards before the Arms Race")
		}
		if p.Influence != influenceStart {
			t.Fatalf("influence = %d", p.Influence)
		}
	}
	for _, r := range roleOrder {
		if seen[r] != 1 {
			t.Fatalf("%s dealt %d times", r, seen[r])
		}
	}
	if seen[Citizen] != 6-len(roleOrder) {
		t.Fatalf("citizens = %d", seen[Citizen])
	}
}

func TestSecretsAreNotAllTheSame(t *testing.T) {
	// Two of each Weakness are in the deck, so a five-player table can never be
	// dealt three of one thing.
	s := table(t, 8)
	count := map[string]int{}
	for _, p := range s.Players {
		count[p.Weakness.Slug]++
	}
	for slug, n := range count {
		if n > weaknessCopies {
			t.Fatalf("%s dealt %d times", slug, n)
		}
	}
}

func TestTurnGating(t *testing.T) {
	s := table(t, 5)
	other := s.Players[(s.Current+1)%5].ID
	if _, err := Apply(s, Action{Kind: ActPass, PlayerID: other}); err != ErrNotYourTurn {
		t.Fatalf("off-turn pass: %v", err)
	}
	cur := s.CurrentID()
	if _, err := Apply(s, Action{Kind: ActPass, PlayerID: cur}); err != nil {
		t.Fatal(err)
	}
	if s.CurrentID() == cur {
		t.Fatal("the turn did not move")
	}
	// The turn has moved on, so acting again is refused as off-turn, and even
	// forcing the seat back is refused as already spent.
	s.Current = indexOf(s, cur)
	if _, err := Apply(s, Action{Kind: ActPass, PlayerID: cur}); err != ErrAlreadyActed {
		t.Fatalf("second action: %v", err)
	}
}

func indexOf(s *State, id string) int {
	for i, p := range s.Players {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func TestNoPowerCardsInTheColdWar(t *testing.T) {
	s := table(t, 5)
	p := s.current()
	p.Hand = append(p.Hand, PowerDeck(9000)[0])
	_, err := Apply(s, Action{Kind: ActPower, PlayerID: p.ID,
		TargetID: s.Players[(s.Current+1)%5].ID, CardID: p.Hand[0].ID})
	if err != ErrWrongPhase {
		t.Fatalf("power in round one: %v", err)
	}
}

func TestPoliceLearnsTheExactSecret(t *testing.T) {
	s := table(t, 5)
	cop := roleHolder(s, Police)
	s.Current = indexOf(s, cop.ID)
	target := s.Players[(s.Current+1)%5]

	events, err := Apply(s, Action{Kind: ActSkill, PlayerID: cop.ID, TargetID: target.ID})
	if err != nil {
		t.Fatal(err)
	}
	intel := find(events, EvIntel)
	if intel == nil || intel.OnlyFor != cop.ID {
		t.Fatal("the finding must be private to the investigator")
	}
	if !strings.Contains(intel.Text, target.Weakness.Name) {
		t.Fatalf("intel did not name the secret: %q", intel.Text)
	}
	// The public line says an investigation happened and nothing about what it
	// found.
	pub := find(events, EvSkill)
	if pub == nil || pub.OnlyFor != "" {
		t.Fatal("the table must be told a file was opened")
	}
	if strings.Contains(pub.Text, target.Weakness.Name) {
		t.Fatal("the public line leaked the secret")
	}
	if !cop.Acted {
		t.Fatal("a skill spends the turn")
	}
}

func TestSkillIsOncePerRoundButTheHackerGetsTwo(t *testing.T) {
	s := table(t, 5)
	cop := roleHolder(s, Police)
	s.Current = indexOf(s, cop.ID)
	other := s.Players[(s.Current+1)%5].ID
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: cop.ID, TargetID: other}); err != nil {
		t.Fatal(err)
	}
	s.Current = indexOf(s, cop.ID)
	cop.Acted = false
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: cop.ID, TargetID: other}); err != ErrSkillSpent {
		t.Fatalf("second investigation: %v", err)
	}

	s = table(t, 5)
	h := roleHolder(s, Hacker)
	s.Current = indexOf(s, h.ID)
	a := s.Players[(s.Current+1)%5].ID
	b := s.Players[(s.Current+2)%5].ID
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: h.ID, TargetID: a}); err != nil {
		t.Fatal(err)
	}
	if h.Acted {
		t.Fatal("the first peek must not end the hacker's turn")
	}
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: h.ID, TargetID: b}); err != nil {
		t.Fatal(err)
	}
	if !h.Acted {
		t.Fatal("the second peek should end it")
	}
}

func TestLawyerCountsAndRefusesNonsense(t *testing.T) {
	s := table(t, 6)
	l := roleHolder(s, Lawyer)
	s.Current = indexOf(s, l.ID)
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: l.ID, Slug: "not-a-secret"}); err != ErrNoSuchWeakness {
		t.Fatalf("bogus slug: %v", err)
	}
	slug := s.Players[0].Weakness.Slug
	want := 0
	for _, p := range s.Players {
		if p.Weakness.Slug == slug {
			want++
		}
	}
	events, err := Apply(s, Action{Kind: ActSkill, PlayerID: l.ID, Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	intel := find(events, EvIntel)
	if intel == nil || intel.Count != want {
		t.Fatalf("count = %v, want %d", intel, want)
	}
}

func TestPresidentialDecreeIsPublic(t *testing.T) {
	s := table(t, 5)
	pres := roleHolder(s, President)
	s.Current = indexOf(s, pres.ID)
	target := s.Players[(s.Current+1)%5]
	before := pres.Influence

	events, err := Apply(s, Action{Kind: ActSkill, PlayerID: pres.ID, TargetID: target.ID})
	if err != nil {
		t.Fatal(err)
	}
	d := find(events, EvDisclosed)
	if d == nil || d.OnlyFor != "" {
		t.Fatal("a decree is public")
	}
	if d.Text != string(target.Weakness.Cat) {
		t.Fatalf("disclosed %q, want %q", d.Text, target.Weakness.Cat)
	}
	if strings.Contains(d.Text, target.Weakness.Name) {
		t.Fatal("a decree exposes the category, not the card")
	}
	if pres.Influence != before+1 {
		t.Fatal("a decree pays the president")
	}
	// Everybody's dossier records it, because everybody heard it.
	for _, p := range s.Players {
		if len(p.Dossier) < 2 {
			t.Fatalf("%s did not hear the decree", p.Name)
		}
	}
}

func roleHolder(s *State, r Role) *Player {
	for _, p := range s.Players {
		if p.Role == r {
			return p
		}
	}
	return nil
}

func TestRoundTwoDealsPowerAndPaysInfluence(t *testing.T) {
	s := table(t, 5)
	before := s.Players[0].Influence
	passRound(t, s)

	if s.Round != RoundArmsRace || s.Phase != PhaseArmsRace {
		t.Fatalf("round %d phase %s", s.Round, s.Phase)
	}
	for _, p := range s.Players {
		if len(p.Hand) != handSize {
			t.Fatalf("%s holds %d cards", p.Name, len(p.Hand))
		}
		if p.Acted || p.SkillsUsed != 0 {
			t.Fatal("the round did not reset")
		}
		if p.Influence != before+influenceRound {
			t.Fatalf("influence %d", p.Influence)
		}
	}
}

// arms gets a table into round two with a known board: the current player holds
// a card that lands on the next player and one that cannot land on anybody.
func arms(t *testing.T, n int) (*State, *Player, *Player, Card, Card) {
	t.Helper()
	s := table(t, n)
	passRound(t, s)
	actor := s.current()
	victim := s.Players[(s.Current+1)%n]

	hit := Card{ID: 9001, Kind: KindPower, Slug: "hit", Name: "Hit",
		Exploits: victim.Weakness.Slug}
	miss := Card{ID: 9002, Kind: KindPower, Slug: "miss", Name: "Miss",
		Exploits: "no-such-secret"}
	actor.Hand = append(actor.Hand, hit, miss)
	return s, actor, victim, hit, miss
}

func TestBlackmailLandsPrivately(t *testing.T) {
	s, actor, victim, hit, _ := arms(t, 5)
	before := actor.Influence

	events, err := Apply(s, Action{Kind: ActPower, PlayerID: actor.ID,
		TargetID: victim.ID, CardID: hit.ID})
	if err != nil {
		t.Fatal(err)
	}
	if victim.MasterID != actor.ID {
		t.Fatalf("victim master = %q", victim.MasterID)
	}
	if actor.Influence != before+1 {
		t.Fatal("a landed card pays")
	}
	// The public line names both players and says nothing about the outcome.
	move := find(events, EvMove)
	if move == nil || move.OnlyFor != "" {
		t.Fatal("the table sees that a move was made")
	}
	if move.Text != "" || len(move.Cards) != 0 {
		t.Fatalf("the public move leaked something: %+v", move)
	}
	owned := find(events, EvOwned)
	if owned == nil || owned.OnlyFor == "" {
		t.Fatal("the result is private")
	}
	// Nobody but the two of them is told.
	for _, e := range events {
		if e.Kind == EvOwned && e.OnlyFor != actor.ID && e.OnlyFor != victim.ID {
			t.Fatalf("leaked to %s", e.OnlyFor)
		}
	}
}

func TestBurnedCardLooksTheSameFromOutside(t *testing.T) {
	s, actor, victim, _, miss := arms(t, 5)
	events, err := Apply(s, Action{Kind: ActPower, PlayerID: actor.ID,
		TargetID: victim.ID, CardID: miss.ID})
	if err != nil {
		t.Fatal(err)
	}
	if victim.MasterID != "" {
		t.Fatal("a miss must not take control")
	}
	if find(events, EvMove) == nil {
		t.Fatal("a miss is still a public move")
	}
	for _, e := range events {
		if e.Kind == EvBurned && e.OnlyFor == "" {
			t.Fatal("a burn is private")
		}
	}
	if _, ok := actor.findCard(miss.ID); ok {
		t.Fatal("the card was not spent")
	}
	if s.BurnedCount() == 0 {
		t.Fatal("the card did not reach the burn pile")
	}
}

func TestCannotRetakeYourOwnPawnOrTouchTheProtected(t *testing.T) {
	s, actor, victim, hit, _ := arms(t, 5)
	victim.MasterID = actor.ID
	if _, err := Apply(s, Action{Kind: ActPower, PlayerID: actor.ID,
		TargetID: victim.ID, CardID: hit.ID}); err != ErrAlreadyOwned {
		t.Fatalf("retaking own pawn: %v", err)
	}
	victim.MasterID = ""
	victim.Pardoned = true
	if _, err := Apply(s, Action{Kind: ActPower, PlayerID: actor.ID,
		TargetID: victim.ID, CardID: hit.ID}); err != ErrPardoned {
		t.Fatalf("hitting the pardoned: %v", err)
	}
}

func TestCleanRecordSwapsTheSecretAndIsBarredToPawns(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	p := s.current()
	p.Influence = costClean
	old := p.Weakness

	if _, err := Apply(s, Action{Kind: ActCleanRecord, PlayerID: p.ID}); err != nil {
		t.Fatal(err)
	}
	if p.Weakness.ID == old.ID {
		t.Fatal("the secret did not change")
	}
	if p.Influence != 0 {
		t.Fatalf("influence = %d", p.Influence)
	}

	s2 := table(t, 5)
	passRound(t, s2)
	q := s2.current()
	q.Influence = 99
	q.MasterID = s2.Players[(s2.Current+1)%5].ID
	if _, err := Apply(s2, Action{Kind: ActCleanRecord, PlayerID: q.ID}); err != ErrCleanNoOne {
		t.Fatalf("clean record while owned: %v", err)
	}

	s3 := table(t, 5)
	passRound(t, s3)
	r := s3.current()
	r.Influence = costClean - 1
	if _, err := Apply(s3, Action{Kind: ActCleanRecord, PlayerID: r.ID}); err != ErrInfluence {
		t.Fatalf("unaffordable clean record: %v", err)
	}
}

func TestEvidenceNamesThenFrees(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	pawn := s.current()
	master := s.Players[(s.Current+1)%5]
	pawn.MasterID = master.ID

	// First dig: nothing but a count.
	events, err := Apply(s, Action{Kind: ActEvidence, PlayerID: pawn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if pawn.KnowsMaster {
		t.Fatal("one piece of evidence is not a name")
	}
	if e := find(events, EvEvidence); e == nil || e.OnlyFor != pawn.ID {
		t.Fatal("digging is private")
	}

	// Second: the name.
	s.Current = indexOf(s, pawn.ID)
	pawn.Acted = false
	events, _ = Apply(s, Action{Kind: ActEvidence, PlayerID: pawn.ID})
	if !pawn.KnowsMaster {
		t.Fatal("two pieces should name the owner")
	}
	named := false
	for _, e := range events {
		if e.Kind == EvEvidence && strings.Contains(e.Text, master.Name) {
			named = true
		}
	}
	if !named {
		t.Fatalf("the second dig should name %s", master.Name)
	}

	// Third: the door. A mutiny is public, and protects them for the round.
	s.Current = indexOf(s, pawn.ID)
	pawn.Acted = false
	events, _ = Apply(s, Action{Kind: ActEvidence, PlayerID: pawn.ID})
	if pawn.MasterID != "" {
		t.Fatal("three pieces should free them")
	}
	if !pawn.Pardoned {
		t.Fatal("a mutiny protects for the round")
	}
	m := find(events, EvMutiny)
	if m == nil || m.OnlyFor != "" {
		t.Fatal("a mutiny is public")
	}
	if strings.Contains(m.Text, master.Name) {
		t.Fatal("a mutiny must not name the owner in public")
	}
}

func TestOnlyAPawnCanDig(t *testing.T) {
	s := table(t, 5)
	if _, err := Apply(s, Action{Kind: ActEvidence, PlayerID: s.CurrentID()}); err != ErrNotCompromised {
		t.Fatalf("digging while free: %v", err)
	}
}

func TestSeizeTakesFromYourOwnPawnOnly(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	owner := s.current()
	pawn := s.Players[(s.Current+1)%5]
	card := pawn.Hand[0]

	if _, err := Apply(s, Action{Kind: ActSeize, PlayerID: owner.ID,
		TargetID: pawn.ID, CardID: card.ID}); err != ErrNotPawn {
		t.Fatalf("seizing from a free player: %v", err)
	}
	pawn.MasterID = "somebody-else"
	if _, err := Apply(s, Action{Kind: ActSeize, PlayerID: owner.ID,
		TargetID: pawn.ID, CardID: card.ID}); err != ErrNotYourPawn {
		t.Fatalf("seizing somebody else's pawn: %v", err)
	}

	pawn.MasterID = owner.ID
	if _, err := Apply(s, Action{Kind: ActSeize, PlayerID: owner.ID,
		TargetID: pawn.ID, CardID: card.ID}); err != nil {
		t.Fatal(err)
	}
	if _, ok := pawn.findCard(card.ID); ok {
		t.Fatal("the card stayed with the pawn")
	}
	if _, ok := owner.findCard(card.ID); !ok {
		t.Fatal("the card never arrived")
	}
}

func TestTradeMovesCardsAndInfluence(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	a, b := s.Players[0], s.Players[1]
	mine, theirs := a.Hand[0], b.Hand[0]
	a.Influence, b.Influence = 5, 5

	// An offer is a free action: it does not need to be anybody's turn.
	events, err := Apply(s, Action{Kind: ActOffer, PlayerID: a.ID, TargetID: b.ID,
		CardIDs: []int{mine.ID}, WantIDs: []int{theirs.ID}, Amount: 2, Text: "final offer"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Acted {
		t.Fatal("an offer must not spend the turn")
	}
	sent := find(events, EvOffer)
	if sent == nil || sent.OnlyFor == "" {
		t.Fatal("an offer is private")
	}
	id := sent.Points

	if _, err := Apply(s, Action{Kind: ActAccept, PlayerID: a.ID, OfferID: id}); err != ErrNotYourOffer {
		t.Fatalf("accepting your own offer: %v", err)
	}
	if _, err := Apply(s, Action{Kind: ActAccept, PlayerID: b.ID, OfferID: id}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.findCard(mine.ID); !ok {
		t.Fatal("the offered card did not move")
	}
	if _, ok := a.findCard(theirs.ID); !ok {
		t.Fatal("the wanted card did not move")
	}
	if a.Influence != 3 || b.Influence != 7 {
		t.Fatalf("influence a=%d b=%d", a.Influence, b.Influence)
	}
	if len(s.Offers) != 0 {
		t.Fatal("a settled offer should be gone")
	}
	// The table learns that a deal happened, not what was in it.
	pub := find(events, EvOffer)
	if pub.OnlyFor == "" {
		t.Fatal("offer contents leaked")
	}
}

func TestExtortionIsJustAnOfferWithADemand(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	a, b := s.Players[0], s.Players[1]
	a.Influence, b.Influence = 0, 4

	events, err := Apply(s, Action{Kind: ActOffer, PlayerID: a.ID, TargetID: b.ID,
		Demand: 3, Text: "pay me or I use what I have"})
	if err != nil {
		t.Fatal(err)
	}
	id := find(events, EvOffer).Points
	if _, err := Apply(s, Action{Kind: ActAccept, PlayerID: b.ID, OfferID: id}); err != nil {
		t.Fatal(err)
	}
	if a.Influence != 3 || b.Influence != 1 {
		t.Fatalf("influence a=%d b=%d", a.Influence, b.Influence)
	}
}

func TestEmptyOfferAndUnaffordablePromiseRefused(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	a, b := s.Players[0], s.Players[1]
	if _, err := Apply(s, Action{Kind: ActOffer, PlayerID: a.ID, TargetID: b.ID}); err != ErrEmptyOffer {
		t.Fatalf("empty offer: %v", err)
	}
	a.Influence = 1
	if _, err := Apply(s, Action{Kind: ActOffer, PlayerID: a.ID, TargetID: b.ID,
		Amount: 9}); err != ErrInfluence {
		t.Fatalf("promising money you don't have: %v", err)
	}
}

func TestPlayingACardVoidsOffersThatNamedIt(t *testing.T) {
	s, actor, victim, hit, _ := arms(t, 5)
	third := s.Players[(indexOf(s, actor.ID)+2)%5]
	if _, err := Apply(s, Action{Kind: ActOffer, PlayerID: actor.ID, TargetID: third.ID,
		CardIDs: []int{hit.ID}, Demand: 1}); err != nil {
		t.Fatal(err)
	}
	if len(s.Offers) != 1 {
		t.Fatal("offer not recorded")
	}
	if _, err := Apply(s, Action{Kind: ActPower, PlayerID: actor.ID,
		TargetID: victim.ID, CardID: hit.ID}); err != nil {
		t.Fatal(err)
	}
	if len(s.Offers) != 0 {
		t.Fatal("an offer for a spent card must be withdrawn")
	}
}

func TestNewsCostsInfluenceAndIsAnonymous(t *testing.T) {
	s := table(t, 5)
	p := s.Players[0]
	before := p.Influence

	events, err := Apply(s, Action{Kind: ActNews, PlayerID: p.ID, Text: "the president has debts"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Influence != before-costNews {
		t.Fatal("a leak costs")
	}
	news := find(events, EvNews)
	if news == nil || news.OnlyFor != "" {
		t.Fatal("a leak is public")
	}
	if news.ActorID != "" {
		t.Fatalf("the leak named its author: %q", news.ActorID)
	}
	if len(s.Feed) != 1 || s.Feed[0].AuthorID != p.ID {
		t.Fatal("the author is recorded for the endgame reveal")
	}

	p.Influence = 0
	if _, err := Apply(s, Action{Kind: ActNews, PlayerID: p.ID, Text: "again"}); err != ErrInfluence {
		t.Fatalf("unaffordable leak: %v", err)
	}
	if _, err := Apply(s, Action{Kind: ActNews, PlayerID: s.Players[1].ID}); err != ErrNoText {
		t.Fatal("an empty leak should be refused")
	}
}

// catalyst gets a table to round three with the Coup and Anti-Coup in known
// hands, which is the only way to test either of them.
func catalyst(t *testing.T, n int) (*State, *Player, *Player) {
	t.Helper()
	s := table(t, n)
	passRound(t, s) // -> arms race
	passRound(t, s) // -> catalyst
	if s.Phase != PhaseCatalyst {
		t.Fatalf("phase %s", s.Phase)
	}
	m, r := s.mastermind(), s.resistance()
	if m == nil || r == nil || m.ID == r.ID {
		t.Fatalf("catalyst deal: master=%v rebel=%v", m, r)
	}
	return s, m, r
}

func TestCatalystDealsBothCardsSecretly(t *testing.T) {
	s := table(t, 6)
	passRound(t, s)
	passRound(t, s)

	m, r := s.mastermind(), s.resistance()
	if m == nil || r == nil {
		t.Fatal("both catalyst cards must be dealt")
	}
	if s.CoupTarget() != 4 {
		t.Fatalf("six players needs four pawns, got %d", s.CoupTarget())
	}
	// Everybody got an extra Power card, so nobody's hand size gives the
	// Mastermind away.
	sizes := map[int]bool{}
	for _, p := range s.Players {
		sizes[len(p.Hand)] = true
	}
	if len(sizes) != 1 {
		t.Fatalf("hand sizes differ: %v", sizes)
	}
}

func TestCoupTargetIsThreeFifthsRoundedUp(t *testing.T) {
	for n, want := range map[int]int{5: 3, 6: 4, 7: 5, 8: 5} {
		s := table(t, n)
		if got := s.CoupTarget(); got != want {
			t.Fatalf("%d players: target %d, want %d", n, got, want)
		}
	}
}

func TestMastermindWinsAtTheEndOfARound(t *testing.T) {
	s, m, _ := catalyst(t, 5)
	// Own the target, and hold it until the round closes.
	owned := 0
	for _, p := range s.Players {
		if p.ID == m.ID || owned >= s.CoupTarget() {
			continue
		}
		p.MasterID = m.ID
		owned++
	}
	if s.Phase == PhaseGameOver {
		t.Fatal("a majority mid-round is not yet a coup")
	}
	passRound(t, s)
	if s.Phase != PhaseGameOver || s.Winner != SideMastermind {
		t.Fatalf("phase %s winner %s", s.Phase, s.Winner)
	}
	if len(s.WinnerIDs) != 1 || s.WinnerIDs[0] != m.ID {
		t.Fatalf("winners %v", s.WinnerIDs)
	}
}

func TestCorrectAccusationEndsTheCoup(t *testing.T) {
	s, m, r := catalyst(t, 5)
	s.Current = indexOf(s, r.ID)
	r.Acted = false

	if _, err := Apply(s, Action{Kind: ActAccuse, PlayerID: r.ID, TargetID: m.ID}); err != nil {
		t.Fatal(err)
	}
	if s.Winner != SideResistance || s.WinnerIDs[0] != r.ID {
		t.Fatalf("winner %s %v", s.Winner, s.WinnerIDs)
	}
}

func TestWrongAccusationPaysTheMastermind(t *testing.T) {
	s, m, r := catalyst(t, 5)
	var wrong *Player
	for _, p := range s.Players {
		if p.ID != m.ID && p.ID != r.ID {
			wrong = p
			break
		}
	}
	s.Current = indexOf(s, r.ID)
	r.Acted = false
	before := m.Influence

	if _, err := Apply(s, Action{Kind: ActAccuse, PlayerID: r.ID, TargetID: wrong.ID}); err != nil {
		t.Fatal(err)
	}
	if s.Phase == PhaseGameOver {
		t.Fatal("a wrong accusation does not end the game")
	}
	if m.Influence != before+3 {
		t.Fatalf("mastermind influence %d", m.Influence)
	}
	s.Current = indexOf(s, r.ID)
	r.Acted = false
	if _, err := Apply(s, Action{Kind: ActAccuse, PlayerID: r.ID, TargetID: m.ID}); err != ErrAccusationUsed {
		t.Fatalf("second accusation: %v", err)
	}
}

func TestOnlyTheResistanceAccusesOrPardons(t *testing.T) {
	s, m, _ := catalyst(t, 5)
	s.Current = indexOf(s, m.ID)
	m.Acted = false
	if _, err := Apply(s, Action{Kind: ActAccuse, PlayerID: m.ID,
		TargetID: s.Players[0].ID}); err != ErrNotResistance {
		t.Fatalf("mastermind accusing: %v", err)
	}
	if _, err := Apply(s, Action{Kind: ActPardon, PlayerID: m.ID,
		TargetID: s.Players[0].ID}); err != ErrNotResistance {
		t.Fatalf("mastermind pardoning: %v", err)
	}
}

func TestPardonFreesAPawnAndProtectsThem(t *testing.T) {
	s, m, r := catalyst(t, 5)
	var pawn *Player
	for _, p := range s.Players {
		if p.ID != m.ID && p.ID != r.ID {
			pawn = p
			break
		}
	}
	pawn.MasterID = m.ID
	r.Influence = costPardon

	events, err := Apply(s, Action{Kind: ActPardon, PlayerID: r.ID, TargetID: pawn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if pawn.MasterID != "" || !pawn.Pardoned {
		t.Fatal("the pardon did not take")
	}
	if r.Influence != 0 {
		t.Fatalf("influence %d", r.Influence)
	}
	// Three private lines and nothing public: the Mastermind finds out by
	// noticing, not by being told in front of everybody.
	for _, e := range events {
		if e.OnlyFor == "" {
			t.Fatalf("a pardon leaked publicly: %+v", e)
		}
	}
	if find(events, EvPardon) == nil {
		t.Fatal("nobody was told")
	}

	r.Influence = costPardon - 1
	pawn2 := s.Players[(indexOf(s, pawn.ID)+1)%5]
	if pawn2.ID == r.ID {
		pawn2 = s.Players[(indexOf(s, pawn.ID)+2)%5]
	}
	pawn2.MasterID = m.ID
	if _, err := Apply(s, Action{Kind: ActPardon, PlayerID: r.ID, TargetID: pawn2.ID}); err != ErrInfluence {
		t.Fatalf("unaffordable pardon: %v", err)
	}
}

func TestWhistleblowerReachesTheResistanceOnly(t *testing.T) {
	s, m, r := catalyst(t, 5)
	var pawn *Player
	for _, p := range s.Players {
		if p.ID != m.ID && p.ID != r.ID {
			pawn = p
			break
		}
	}
	pawn.MasterID = m.ID

	events, err := Apply(s, Action{Kind: ActLeak, PlayerID: pawn.ID})
	if err != nil {
		t.Fatal(err)
	}
	var toRebel *Event
	for i, e := range events {
		if e.OnlyFor == "" {
			t.Fatal("a leak is never public")
		}
		if e.OnlyFor == r.ID {
			toRebel = &events[i]
		}
	}
	if toRebel == nil {
		t.Fatal("the resistance heard nothing")
	}
	// A pawn who has not dug cannot name their owner, so neither can the report.
	if strings.Contains(toRebel.Text, m.Name) {
		t.Fatalf("named the master too early: %q", toRebel.Text)
	}
	if _, err := Apply(s, Action{Kind: ActLeak, PlayerID: pawn.ID}); err != ErrLeakSpent {
		t.Fatalf("second leak: %v", err)
	}

	// Once they know, the report carries the name.
	pawn.LeakedThisRound = false
	pawn.KnowsMaster = true
	events, _ = Apply(s, Action{Kind: ActLeak, PlayerID: pawn.ID})
	for _, e := range events {
		if e.OnlyFor == r.ID && !strings.Contains(e.Text, m.Name) {
			t.Fatalf("report should name %s: %q", m.Name, e.Text)
		}
	}
}

func TestLeakBeforeTheCatalystGoesNowhere(t *testing.T) {
	s := table(t, 5)
	passRound(t, s)
	pawn := s.Players[0]
	pawn.MasterID = s.Players[1].ID
	events, err := Apply(s, Action{Kind: ActLeak, PlayerID: pawn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].OnlyFor != pawn.ID {
		t.Fatalf("events %+v", events)
	}
}

func TestTheStateWinsIfNobodyTakesTheRoom(t *testing.T) {
	s, _, _ := catalyst(t, 5)
	pawn := s.Players[0]
	if pawn.Coup {
		pawn = s.Players[1]
	}
	pawn.MasterID = "someone"

	passRound(t, s) // round 4
	if s.Phase == PhaseGameOver {
		t.Fatal("round three is not the end")
	}
	if s.Round != RoundFinal {
		t.Fatalf("round %d", s.Round)
	}
	passRound(t, s)
	if s.Phase != PhaseGameOver || s.Winner != SideState {
		t.Fatalf("phase %s winner %s", s.Phase, s.Winner)
	}
	// A Pawn at the final bell survived; they did not win.
	for _, id := range s.WinnerIDs {
		if id == pawn.ID {
			t.Fatal("a pawn cannot be among the winners")
		}
	}
	if len(s.WinnerIDs) != len(s.Players)-1 {
		t.Fatalf("winners %v", s.WinnerIDs)
	}
}

func TestNothingWorksOnceItIsOver(t *testing.T) {
	s, m, r := catalyst(t, 5)
	s.Current = indexOf(s, r.ID)
	r.Acted = false
	if _, err := Apply(s, Action{Kind: ActAccuse, PlayerID: r.ID, TargetID: m.ID}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []ActionKind{ActPass, ActNews, ActOffer, ActPower, ActLeak, ActPardon} {
		if _, err := Apply(s, Action{Kind: k, PlayerID: s.Players[0].ID}); err != ErrGameOver {
			t.Fatalf("%s after the end: %v", k, err)
		}
	}
}

func TestSelfTargetingAndUnknownPlayersRefused(t *testing.T) {
	s := table(t, 5)
	cur := s.current()
	if _, err := Apply(s, Action{Kind: ActSkill, PlayerID: cur.ID, TargetID: cur.ID}); err != ErrSelfTarget {
		// The Lawyer names a secret rather than a player, so this table's current
		// seat might legitimately not care about TargetID.
		if cur.Role != Lawyer && cur.Role != Citizen {
			t.Fatalf("self-target: %v", err)
		}
	}
	if _, err := Apply(s, Action{Kind: ActNews, PlayerID: "nobody", Text: "x"}); err != ErrNoSuchPlayer {
		t.Fatalf("unknown player: %v", err)
	}
}

func TestFirstSeatWalksBetweenRounds(t *testing.T) {
	s := table(t, 5)
	first := s.CurrentID()
	passRound(t, s)
	if s.CurrentID() == first {
		t.Fatal("the opening seat should move each round")
	}
}

func TestTextIsCapped(t *testing.T) {
	s := table(t, 5)
	long := strings.Repeat("x", newsLimit+50)
	if _, err := Apply(s, Action{Kind: ActNews, PlayerID: s.Players[0].ID, Text: long}); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(s.Feed[0].Text)); n != newsLimit {
		t.Fatalf("feed text is %d runes", n)
	}
}

func TestDecksAreTheRightSize(t *testing.T) {
	if n := len(WeaknessDeck(0)); n != len(weaknesses)*weaknessCopies {
		t.Fatalf("weakness deck %d", n)
	}
	if n := len(PowerDeck(0)); n != len(powers)*powerCopies+kompromatCount {
		t.Fatalf("power deck %d", n)
	}
	// Every Weakness has at least one Power card that answers it, or a secret
	// would be unexploitable and the player holding it could never be blackmailed.
	answered := map[string]bool{}
	for _, p := range powers {
		answered[p.exploits] = true
	}
	for _, w := range weaknesses {
		if !answered[w.slug] {
			t.Fatalf("%s has no counter", w.slug)
		}
	}
	// Ids are unique across both decks, because they share one namespace on the
	// wire.
	seen := map[int]bool{}
	for _, c := range append(WeaknessDeck(1000), PowerDeck(2000)...) {
		if seen[c.ID] {
			t.Fatalf("duplicate card id %d", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestKompromatLandsOnAnything(t *testing.T) {
	var k Card
	for _, c := range PowerDeck(0) {
		if c.Wild {
			k = c
			break
		}
	}
	if k.Name == "" {
		t.Fatal("no wildcard in the deck")
	}
	for _, w := range weaknesses {
		if !k.Lands(w.slug) {
			t.Fatalf("kompromat missed %s", w.slug)
		}
	}
	if (Card{Kind: KindWeakness, Wild: true}).Lands("debt") {
		t.Fatal("only Power cards land")
	}
}
