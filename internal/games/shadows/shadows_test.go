package shadows

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/shadows/game"
)

func seats(n int) []core.Seat {
	out := make([]core.Seat, n)
	for i := range out {
		out[i] = core.Seat{ID: string(rune('a' + i)), Name: strings.ToUpper(string(rune('a' + i)))}
	}
	return out
}

func dealt(t *testing.T, n int) *Game {
	t.Helper()
	g := New(rand.New(rand.NewSource(11))).(*Game)
	if _, err := g.Deal(seats(n)); err != nil {
		t.Fatalf("deal: %v", err)
	}
	return g
}

func shell(viewer string, n int) core.Shell {
	sh := core.Shell{Code: "ABCD", Game: "shadows", ViewerID: viewer}
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		sh.Members = append(sh.Members, core.Membership{ID: id,
			Name: strings.ToUpper(id), Connected: true, Host: i == 0})
	}
	return sh
}

func view(t *testing.T, g *Game, viewer string, n int) *View {
	t.Helper()
	v, ok := g.View(shell(viewer, n)).(*View)
	if !ok {
		t.Fatal("view is not a *View")
	}
	return v
}

func TestLobbyViewIsRenderable(t *testing.T) {
	g := New(nil).(*Game)
	if g.Started() || g.Over() {
		t.Fatal("a fresh table has nothing dealt")
	}
	v := view(t, g, "a", 5)
	if v.Started || v.Phase != "lobby" || v.Game != "shadows" || len(v.Seats) != 5 {
		t.Fatalf("lobby view: %+v", v)
	}
	if !v.Me.Host || v.Me.ID != "a" {
		t.Fatalf("me = %+v", v.Me)
	}
	// The client renders off these, so absent is a crash in the browser where
	// empty is a quiet screen.
	if v.Me.Hand == nil || v.Me.Dossier == nil || v.Me.Offers == nil ||
		v.Feed == nil || v.Log == nil {
		t.Fatal("collections must be empty rather than absent")
	}
	if len(v.Secrets) == 0 || len(v.Powers) == 0 || v.Costs.Clean == 0 {
		t.Fatal("the lobby needs the catalogues so the rules panel works before the deal")
	}
}

func TestPlayerCountBounds(t *testing.T) {
	g := New(nil)
	if g.MinPlayers() != game.MinPlayers || g.MaxPlayers() != game.MaxPlayers {
		t.Fatal("bounds must come from the rules")
	}
	if _, err := g.Deal(seats(4)); err == nil {
		t.Fatal("four players should be refused")
	}
}

// The load-bearing test: nothing in a view may carry another player's secret,
// their hand, or who owns them.
func TestViewHidesEverythingPrivate(t *testing.T) {
	g := dealt(t, 6)
	// Put the board in the most leak-prone state there is: somebody owns
	// somebody, the catalyst is dealt, and there is intel about.
	s := g.state
	for s.Round < 3 {
		if _, err := g.apply(game.Action{Kind: game.ActPass, PlayerID: s.CurrentID()}); err != nil {
			t.Fatal(err)
		}
	}
	victim, owner := s.Players[4], s.Players[5]
	victim.MasterID = owner.ID

	v := view(t, g, "a", 6)
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	me := s.Find("a")
	for _, p := range s.Players {
		if p.ID == me.ID {
			continue
		}
		// Card ids are unique for the life of a game, and the public catalogues
		// use negative ones, so a real card's id appearing on the wire means that
		// card object was serialised. That is a sharper test than looking for
		// names: a Power card in the viewer's own hand legitimately names the
		// secret it answers.
		if !learnedAbout(me, p.ID) && strings.Contains(body, cardID(p.Weakness.ID)) {
			t.Fatalf("%s's secret card reached the wire", p.Name)
		}
		if p.MasterID == me.ID {
			continue // a Pawn's hand is the owner's to see
		}
		for _, c := range p.Hand {
			if strings.Contains(body, cardID(c.ID)) {
				t.Fatalf("%s's card %q reached the wire", p.Name, c.Name)
			}
		}
	}
	// Who owns whom is nobody else's business.
	for _, seat := range v.Seats {
		if seat.ID == victim.ID && seat.Pawn {
			t.Fatal("a pawn of somebody else was flagged as the viewer's")
		}
		if seat.MasterName != "" || seat.Coup || seat.AntiCoup || seat.Secret != nil {
			t.Fatalf("seat %s revealed mid-game: %+v", seat.ID, seat)
		}
	}
	if v.Me.Coup != me.Coup || v.Me.AntiCoup != me.AntiCoup {
		t.Fatal("the catalyst cards must be reported to their holder and nobody else")
	}
	// Every intel note the viewer holds is one they earned.
	if len(v.Me.Dossier) != len(me.Dossier) {
		t.Fatalf("dossier %d notes, own %d", len(v.Me.Dossier), len(me.Dossier))
	}
}

func cardID(id int) string { return fmt.Sprintf(`"id":%d`, id) }

// learnedAbout reports whether the viewer's own dossier names a seat's secret,
// which is the one case in which seeing it is correct.
func learnedAbout(viewer *game.Player, about string) bool {
	for _, in := range viewer.Dossier {
		if in.AboutID == about && in.Slug != "" {
			return true
		}
	}
	return false
}

func TestViewerSeesTheirOwnHalf(t *testing.T) {
	g := dealt(t, 5)
	v := view(t, g, "a", 5)
	me := g.state.Find("a")

	if v.Me.Secret == nil || v.Me.Secret.ID != me.Weakness.ID {
		t.Fatal("you always know your own secret")
	}
	if v.Me.RoleName != me.Role.String() || v.Me.Skill != me.Role.Skill() {
		t.Fatalf("me = %+v", v.Me)
	}
	if v.Round != 1 || v.Phase != string(game.PhaseColdWar) {
		t.Fatalf("round %d phase %s", v.Round, v.Phase)
	}
	if v.CoupTarget != 3 || v.FinalRound != 4 {
		t.Fatalf("target %d final %d", v.CoupTarget, v.FinalRound)
	}
	// Positions and influence are public: that is the premise of the game.
	for _, seat := range v.Seats {
		if seat.RoleName == "" || seat.Influence == 0 {
			t.Fatalf("seat %s should be public: %+v", seat.ID, seat)
		}
	}
}

func TestPawnHandsAreVisibleToTheirOwner(t *testing.T) {
	g := dealt(t, 5)
	s := g.state
	for s.Round < 2 {
		if _, err := g.apply(game.Action{Kind: game.ActPass, PlayerID: s.CurrentID()}); err != nil {
			t.Fatal(err)
		}
	}
	pawn := s.Find("b")
	pawn.MasterID = "a"

	v := view(t, g, "a", 5)
	if len(v.Me.Pawns) != 1 || v.Me.Pawns[0].ID != "b" {
		t.Fatalf("pawns = %+v", v.Me.Pawns)
	}
	if len(v.Me.Pawns[0].Hand) != len(pawn.Hand) {
		t.Fatal("owning somebody means seeing what they hold")
	}
	if v.Me.Held != 1 {
		t.Fatalf("held = %d", v.Me.Held)
	}
	// And the pawn is told they are compromised without being told by whom.
	pv := view(t, g, "b", 5)
	if !pv.Me.Compromised || pv.Me.MasterName != "" {
		t.Fatalf("pawn view = %+v", pv.Me)
	}
}

func TestFeedIsAnonymousUntilItIsOver(t *testing.T) {
	g := dealt(t, 5)
	if _, err := g.Submit("b", core.ClientMsg{Type: "news", Text: "the lawyer is bought"}); err != nil {
		t.Fatal(err)
	}
	v := view(t, g, "a", 5)
	if len(v.Feed) != 1 || v.Feed[0].Text == "" {
		t.Fatalf("feed = %+v", v.Feed)
	}
	if v.Feed[0].Author != "" {
		t.Fatal("a leak must be anonymous while it matters")
	}

	// End it, and the files open.
	s := g.state
	for s.Phase != game.PhaseGameOver {
		if _, err := g.apply(game.Action{Kind: game.ActPass, PlayerID: s.CurrentID()}); err != nil {
			t.Fatal(err)
		}
	}
	v = view(t, g, "a", 5)
	if v.Feed[0].Author != "B" {
		t.Fatalf("author = %q, want B", v.Feed[0].Author)
	}
	for _, seat := range v.Seats {
		if seat.Secret == nil {
			t.Fatalf("seat %s was not revealed", seat.ID)
		}
	}
	if v.Winner == "" || v.Reason == "" {
		t.Fatalf("winner %q reason %q", v.Winner, v.Reason)
	}
	if !g.Over() {
		t.Fatal("Over() should be true so the host gets the deal-again button")
	}
}

func TestSubmitMapsEveryClientAction(t *testing.T) {
	for _, typ := range []string{"skill", "power", "clean", "evidence", "seize",
		"accuse", "pass", "offer", "accept", "decline", "news", "leak", "pardon"} {
		if _, ok := toAction("a", core.ClientMsg{Type: typ}); !ok {
			t.Fatalf("%q is not wired up", typ)
		}
	}
	if _, ok := toAction("a", core.ClientMsg{Type: "nonsense"}); ok {
		t.Fatal("an unknown action must be refused")
	}
	// "advance" is the room's, never a client's.
	if _, ok := toAction("a", core.ClientMsg{Type: "advance"}); ok {
		t.Fatal("a client must not be able to force the round on")
	}
}

func TestSubmitCarriesTheProposalFields(t *testing.T) {
	g := dealt(t, 5)
	s := g.state
	for s.Round < 2 {
		if _, err := g.apply(game.Action{Kind: game.ActPass, PlayerID: s.CurrentID()}); err != nil {
			t.Fatal(err)
		}
	}
	mine := s.Find("a").Hand[0]
	theirs := s.Find("b").Hand[0]
	if _, err := g.Submit("a", core.ClientMsg{Type: "offer", TargetID: "b",
		CardIDs: []int{mine.ID}, WantIDs: []int{theirs.ID}, Amount: 1, Text: "deal"}); err != nil {
		t.Fatal(err)
	}
	v := view(t, g, "b", 5)
	if len(v.Me.Offers) != 1 {
		t.Fatalf("offers = %+v", v.Me.Offers)
	}
	o := v.Me.Offers[0]
	if o.Mine || len(o.Give) != 1 || len(o.Want) != 1 || o.Pay != 1 || o.Note != "deal" {
		t.Fatalf("offer = %+v", o)
	}
	if _, err := g.Submit("b", core.ClientMsg{Type: "accept", OfferID: o.ID}); err != nil {
		t.Fatal(err)
	}
	if !holds(s.Find("b"), mine.ID) {
		t.Fatal("the offered card did not reach the other side")
	}
	if !holds(s.Find("a"), theirs.ID) {
		t.Fatal("the wanted card did not come back")
	}
	if len(view(t, g, "b", 5).Me.Offers) != 0 {
		t.Fatal("a settled proposal should be off the screen")
	}
}

func holds(p *game.Player, id int) bool {
	for _, c := range p.Hand {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestBlockedOnAndAutoMovePass(t *testing.T) {
	g := dealt(t, 5)
	cur := g.BlockedOn()
	if cur == "" {
		t.Fatal("a live round is always waiting on somebody")
	}
	if _, err := g.AutoMove("somebody-else"); err != core.ErrNoMove {
		t.Fatalf("automove for the wrong player: %v", err)
	}
	if _, err := g.AutoMove(cur); err != nil {
		t.Fatal(err)
	}
	if g.BlockedOn() == cur {
		t.Fatal("the turn did not move on")
	}
	// AutoMove never spends a card or hands out information.
	for _, p := range g.state.Players {
		if len(p.Hand) != 0 || len(p.Dossier) != 1 {
			t.Fatalf("%s changed: hand %d dossier %d", p.Name, len(p.Hand), len(p.Dossier))
		}
	}
}

func TestNoWindow(t *testing.T) {
	g := dealt(t, 5)
	if _, _, open := g.Window(); open {
		t.Fatal("nothing in this game is interrupted")
	}
	if g.WindowExpired() != nil {
		t.Fatal("no window, no expiry")
	}
}

func TestResetAndRename(t *testing.T) {
	g := dealt(t, 5)
	g.Rename("a", "Ann")
	if g.state.Find("a").Name != "Ann" {
		t.Fatal("rename did not take")
	}
	g.Reset()
	if g.Started() || g.Over() {
		t.Fatal("reset should land back in the lobby")
	}
	g.Rename("a", "Nobody") // must not panic before a deal
	if _, err := g.Submit("a", core.ClientMsg{Type: "pass"}); err != core.ErrNotStarted {
		t.Fatalf("submit before the deal: %v", err)
	}
}

func TestSpectatorGetsThePublicBoardOnly(t *testing.T) {
	g := dealt(t, 5)
	v := view(t, g, "zz", 5)
	if v.Me.Secret != nil || len(v.Me.Hand) != 0 || v.Me.Can.Pass {
		t.Fatalf("spectator me = %+v", v.Me)
	}
	if len(v.Seats) != 5 {
		t.Fatal("a spectator still sees the table")
	}
}

func TestPermissionsMatchThePhase(t *testing.T) {
	g := dealt(t, 5)
	cur := g.state.CurrentID()
	v := view(t, g, cur, 5)
	if !v.Me.Can.Skill || !v.Me.Can.Pass {
		t.Fatalf("round one should offer a skill and a pass: %+v", v.Me.Can)
	}
	if v.Me.Can.Power || v.Me.Can.Clean || v.Me.Can.Accuse || v.Me.Can.Pardon {
		t.Fatalf("round one offers none of those: %+v", v.Me.Can)
	}
	// Off-turn, the free actions are still live and the main ones are not.
	other := "a"
	if other == cur {
		other = "b"
	}
	ov := view(t, g, other, 5)
	if ov.Me.Can.Skill || ov.Me.Can.Pass {
		t.Fatal("main actions are turn-gated")
	}
	if !ov.Me.Can.Offer || !ov.Me.Can.News {
		t.Fatalf("free actions are not: %+v", ov.Me.Can)
	}
}
