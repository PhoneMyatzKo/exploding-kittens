package shadows

import (
	"boardgame/kittens/internal/core"
	"boardgame/kittens/internal/games/shadows/game"
)

// Seat is one player as the viewer is entitled to see them.
//
// Positions and influence are public — the spec's whole conceit is that the
// power structure is on the table and only the leverage is hidden. Everything
// viewer-relative on this struct is derived from the viewer's own dossier or
// from their own control, never from the target's private state, and the
// Revealed block is filled in only once the game is over.
type Seat struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`

	Role     string `json:"role,omitempty"`
	RoleName string `json:"roleName,omitempty"`
	Skill    string `json:"skill,omitempty"`
	// SkillLeft is how many uses of their position they have left this round.
	// Public because using one is public.
	SkillLeft int `json:"skillLeft"`

	Influence int  `json:"influence"`
	HandCount int  `json:"handCount"`
	Acted     bool `json:"acted"`

	Connected bool `json:"connected"`
	Host      bool `json:"host"`
	Current   bool `json:"current"`
	// Alive exists because the shell's seat list is shared with a game where
	// players are eliminated. Nobody is ever out here.
	Alive bool `json:"alive"`

	// Pawn is set when this seat is under the *viewer's* control. Whether they
	// are under somebody else's is not the viewer's business.
	Pawn bool `json:"pawn,omitempty"`
	// KnownSecret and KnownCategory are what the viewer has established about
	// this seat, and nothing more: a name if they have one, otherwise a category
	// if they have that.
	KnownSecret   string `json:"knownSecret,omitempty"`
	KnownCategory string `json:"knownCategory,omitempty"`
	// Cleared marks a seat whose secret has been replaced since the viewer last
	// learned anything about it, so a stale dossier reads as stale.
	Cleared bool `json:"cleared,omitempty"`

	// Revealed, once it is over.
	Secret     *game.Card `json:"secret,omitempty"`
	MasterName string     `json:"masterName,omitempty"`
	Coup       bool       `json:"coup,omitempty"`
	AntiCoup   bool       `json:"antiCoup,omitempty"`
	Won        bool       `json:"won,omitempty"`
}

// Pawn is one of the viewer's own Pawns, hand and all: owning somebody means
// seeing what they are holding, which is what makes control worth having before
// the final count.
type Pawn struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Hand []game.Card `json:"hand"`
}

// OfferView is one open proposal, with the faces of the cards on both sides
// resolved. Sent only to the two players party to it.
type OfferView struct {
	ID   int  `json:"id"`
	Mine bool `json:"mine"` // true if the viewer made it
	// FromID and ToID are kept so the client can name both ends without
	// re-deriving the direction.
	FromID   string      `json:"fromId"`
	ToID     string      `json:"toId"`
	Give     []game.Card `json:"give,omitempty"`
	Want     []game.Card `json:"want,omitempty"`
	Pay      int         `json:"pay,omitempty"`
	Demand   int         `json:"demand,omitempty"`
	Note     string      `json:"note,omitempty"`
	Round    int         `json:"round"`
	Unfunded bool        `json:"unfunded,omitempty"` // the viewer cannot meet the demand
}

// FeedPost is one line of the anonymous board. Author is empty until the game is
// over, which is the only redaction on this type and the reason it exists rather
// than the engine's Post going straight onto the wire.
type FeedPost struct {
	ID     int    `json:"id"`
	Text   string `json:"text"`
	Round  int    `json:"round"`
	Author string `json:"author,omitempty"`
}

// Can is what the viewer's buttons should be doing. Computed here so the rule for
// each one lives in one language rather than two.
type Can struct {
	Skill    bool `json:"skill"`
	Power    bool `json:"power"`
	Clean    bool `json:"clean"`
	Evidence bool `json:"evidence"`
	Seize    bool `json:"seize"`
	Accuse   bool `json:"accuse"`
	Pass     bool `json:"pass"`
	// Free actions. Available whether or not it is the viewer's turn.
	Offer  bool `json:"offer"`
	News   bool `json:"news"`
	Leak   bool `json:"leak"`
	Pardon bool `json:"pardon"`
}

// Me is the viewer's private half of the world.
type Me struct {
	ID       string `json:"id"`
	Host     bool   `json:"host"`
	Alive    bool   `json:"alive"`
	Role     string `json:"role,omitempty"`
	RoleName string `json:"roleName,omitempty"`
	Skill    string `json:"skill,omitempty"`
	// SkillBlurb is the one-line description of the position's action.
	SkillBlurb string `json:"skillBlurb,omitempty"`
	SkillLeft  int    `json:"skillLeft"`

	Secret    *game.Card  `json:"secret,omitempty"`
	Hand      []game.Card `json:"hand"`
	Influence int         `json:"influence"`

	MyTurn bool `json:"myTurn"`
	Acted  bool `json:"acted"`

	// Compromised is set while somebody owns the viewer. MasterName is filled in
	// only once they have dug for it.
	Compromised bool   `json:"compromised,omitempty"`
	MasterName  string `json:"masterName,omitempty"`
	Evidence    int    `json:"evidence"`
	Protected   bool   `json:"protected,omitempty"`

	Coup     bool `json:"coup,omitempty"`
	AntiCoup bool `json:"antiCoup,omitempty"`
	Accused  bool `json:"accused,omitempty"`
	// Pawns is who the viewer owns, hands included, and Held is how many that is
	// against the Coup's target.
	Pawns []Pawn `json:"pawns,omitempty"`
	Held  int    `json:"held"`

	Dossier []game.Intel `json:"dossier"`
	Offers  []OfferView  `json:"offers"`
	Can     Can          `json:"can"`
}

// Costs is the price list, sent rather than hard-coded in the client so that the
// numbers cannot drift apart.
type Costs struct {
	News           int `json:"news"`
	Clean          int `json:"clean"`
	Pardon         int `json:"pardon"`
	EvidenceToName int `json:"evidenceToName"`
	EvidenceToFree int `json:"evidenceToFree"`
}

// View is the complete payload one client receives.
type View struct {
	Type    string `json:"type"` // always "state"
	Code    string `json:"code"`
	Started bool   `json:"started"`
	Phase   string `json:"phase"`
	Round   int    `json:"round"`
	// FinalRound and CoupTarget are the two numbers the whole table plays
	// against, so both are public.
	FinalRound int `json:"finalRound"`
	CoupTarget int `json:"coupTarget"`

	Seats     []Seat `json:"seats"`
	Me        Me     `json:"me"`
	CurrentID string `json:"currentId,omitempty"`

	Feed      []FeedPost `json:"feed"`
	PowerDeck int        `json:"powerDeck"`
	Burned    int        `json:"burned"`

	// Winner is the side that took it, WinnerIDs who personally won, and Reason
	// the line the game-over card prints.
	Winner    string   `json:"winner,omitempty"`
	WinnerIDs []string `json:"winnerIds,omitempty"`
	Reason    string   `json:"reason,omitempty"`

	// Secrets and Powers are the catalogues: which secrets exist and which card
	// answers which. Public information, and the client needs both to build the
	// Lawyer's dropdown and the rules panel.
	Secrets []game.Card `json:"secrets"`
	Powers  []game.Card `json:"powers"`
	Costs   Costs       `json:"costs"`

	// Public is the room's visibility and Game the catalogue slug. Both filled in
	// by the room, which owns those facts.
	Public bool   `json:"public"`
	Game   string `json:"game"`

	Log []core.Entry `json:"log"`
}

func costs() Costs {
	return Costs{
		News: game.CostNews(), Clean: game.CostClean(), Pardon: game.CostPardon(),
		EvidenceToName: game.EvidenceToName(), EvidenceToFree: game.EvidenceToFree(),
	}
}

// lobby renders a room that has not been dealt yet: the same shape as a live
// view with everything empty, so the shell's lobby code cannot tell them apart.
func lobby(sh core.Shell) *View {
	v := &View{Type: "state", Code: sh.Code, Phase: "lobby",
		FinalRound: game.FinalRound(), Costs: costs(),
		Secrets: game.WeaknessCatalogue(), Powers: game.PowerCatalogue(),
		Feed: []FeedPost{}, Log: []core.Entry{},
		Me: Me{Hand: []game.Card{}, Alive: true, Dossier: []game.Intel{}, Offers: []OfferView{}},
	}
	for _, m := range sh.Members {
		v.Seats = append(v.Seats, Seat{ID: m.ID, Name: m.Name, Avatar: m.Avatar,
			Connected: m.Connected, Host: m.Host, Alive: true})
		if m.ID == sh.ViewerID {
			v.Me.ID = m.ID
			v.Me.Host = m.Host
		}
	}
	return v
}

// render projects a live game from one seat. Every line in here is either public
// information, information the viewer earned, or information the game being over
// has released.
func render(s *game.State, sh core.Shell) *View {
	over := s.Phase == game.PhaseGameOver
	v := &View{
		Type: "state", Code: sh.Code, Started: true,
		Phase: string(s.Phase), Round: s.Round,
		FinalRound: game.FinalRound(), CoupTarget: s.CoupTarget(),
		CurrentID: s.CurrentID(),
		PowerDeck: s.PowerDeckCount(), Burned: s.BurnedCount(),
		Winner: string(s.Winner), WinnerIDs: s.WinnerIDs, Reason: s.Reason,
		Secrets: game.WeaknessCatalogue(), Powers: game.PowerCatalogue(),
		Costs: costs(), Log: sh.Log,
	}
	if v.Log == nil {
		v.Log = []core.Entry{}
	}

	v.Feed = make([]FeedPost, 0, len(s.Feed))
	for _, post := range s.Feed {
		fp := FeedPost{ID: post.ID, Text: post.Text, Round: post.Round}
		// The author is the last secret to go. Until then a leak is a leak.
		if over {
			if a := s.Find(post.AuthorID); a != nil {
				fp.Author = a.Name
			}
		}
		v.Feed = append(v.Feed, fp)
	}

	meta := map[string]core.Membership{}
	for _, m := range sh.Members {
		meta[m.ID] = m
	}
	viewer := s.Find(sh.ViewerID)
	known := dossierIndex(viewer)
	winners := map[string]bool{}
	for _, id := range s.WinnerIDs {
		winners[id] = true
	}

	for _, p := range s.Players {
		m := meta[p.ID]
		seat := Seat{
			ID: p.ID, Name: p.Name, Avatar: m.Avatar,
			Role: string(p.Role), RoleName: p.Role.String(), Skill: p.Role.Skill(),
			SkillLeft: p.Role.SkillUses() - p.SkillsUsed,
			Influence: p.Influence, HandCount: len(p.Hand), Acted: p.Acted,
			Connected: m.Connected, Host: m.Host,
			Current: v.CurrentID == p.ID, Alive: true,
		}
		if viewer != nil && p.ID != viewer.ID {
			seat.Pawn = p.MasterID == viewer.ID
			if k, ok := known[p.ID]; ok {
				seat.KnownSecret = k.secret
				seat.KnownCategory = k.category
				seat.Cleared = k.round < p.CleanedOn
			}
		}
		if over {
			secret := p.Weakness
			seat.Secret = &secret
			seat.Coup, seat.AntiCoup, seat.Won = p.Coup, p.AntiCoup, winners[p.ID]
			if master := s.Find(p.MasterID); master != nil {
				seat.MasterName = master.Name
			}
		}
		v.Seats = append(v.Seats, seat)
	}

	if viewer == nil {
		// A spectator, or somebody whose seat was dealt out from under them. They
		// get the public board and nothing else.
		v.Me = Me{ID: sh.ViewerID, Alive: true, Hand: []game.Card{},
			Dossier: []game.Intel{}, Offers: []OfferView{}}
		return v
	}
	v.Me = renderMe(s, viewer, meta[viewer.ID], v.CurrentID == viewer.ID)
	return v
}

func renderMe(s *game.State, p *game.Player, m core.Membership, myTurn bool) Me {
	secret := p.Weakness
	me := Me{
		ID: p.ID, Host: m.Host, Alive: true,
		Role: string(p.Role), RoleName: p.Role.String(),
		Skill: p.Role.Skill(), SkillBlurb: p.Role.Blurb(),
		SkillLeft: p.Role.SkillUses() - p.SkillsUsed,
		Secret:    &secret,
		Hand:      append([]game.Card{}, p.Hand...),
		Influence: p.Influence,
		MyTurn:    myTurn, Acted: p.Acted,
		Compromised: p.MasterID != "",
		Evidence:    p.Evidence,
		Protected:   p.Pardoned,
		Coup:        p.Coup, AntiCoup: p.AntiCoup, Accused: p.Accused,
		Held:    s.PawnsOf(p.ID),
		Dossier: append([]game.Intel{}, p.Dossier...),
		Offers:  []OfferView{},
	}
	if me.Dossier == nil {
		me.Dossier = []game.Intel{}
	}
	// The name of whoever owns you is the reward for digging, so it is gated on
	// having dug rather than on being owned.
	if p.KnowsMaster {
		if master := s.Find(p.MasterID); master != nil {
			me.MasterName = master.Name
		}
	}
	for _, id := range s.PawnIDs(p.ID) {
		q := s.Find(id)
		if q == nil {
			continue
		}
		me.Pawns = append(me.Pawns, Pawn{ID: q.ID, Name: q.Name,
			Hand: append([]game.Card{}, q.Hand...)})
	}
	for _, o := range s.OffersFor(p.ID) {
		me.Offers = append(me.Offers, renderOffer(s, p, o))
	}
	me.Can = permissions(s, p, myTurn)
	return me
}

func renderOffer(s *game.State, viewer *game.Player, o game.Offer) OfferView {
	out := OfferView{ID: o.ID, Mine: o.FromID == viewer.ID,
		FromID: o.FromID, ToID: o.ToID,
		Pay: o.Pay, Demand: o.Demand, Note: o.Note, Round: o.Round}
	for _, id := range o.Give {
		if c, ok := s.CardByID(viewer.ID, id); ok {
			out.Give = append(out.Give, c)
		}
	}
	for _, id := range o.Want {
		if c, ok := s.CardByID(viewer.ID, id); ok {
			out.Want = append(out.Want, c)
		}
	}
	if !out.Mine && o.Demand > viewer.Influence {
		out.Unfunded = true
	}
	return out
}

// permissions is every button, and the reason each one is or is not live. It
// mirrors the engine's own checks rather than guessing at them: a button that is
// live when the action would be refused is worse than one that is greyed out.
func permissions(s *game.State, p *game.Player, myTurn bool) Can {
	live := s.Phase != game.PhaseGameOver
	main := live && myTurn && !p.Acted
	return Can{
		Skill:    main && p.SkillsUsed < p.Role.SkillUses(),
		Power:    main && s.CanPlayPower() && len(p.Hand) > 0,
		Clean:    main && s.CanCleanRecord(p.ID),
		Evidence: main && p.MasterID != "",
		Seize:    main && s.PawnsOf(p.ID) > 0,
		Accuse:   main && p.AntiCoup && !p.Accused && s.Phase == game.PhaseCatalyst,
		Pass:     main,

		Offer:  live && (len(p.Hand) > 0 || p.Influence > 0),
		News:   live && p.Influence >= game.CostNews(),
		Leak:   live && p.MasterID != "" && !p.LeakedThisRound,
		Pardon: live && p.AntiCoup && p.Influence >= game.CostPardon(),
	}
}

// knownAbout is what the viewer's dossier establishes about one seat: the most
// precise thing they have, and when they learned it.
type knownAbout struct {
	secret   string
	category string
	round    int
}

// dossierIndex folds the viewer's notes into one entry per seat, keeping the
// sharpest fact about each. A named secret beats a category, and a later note
// beats an earlier one — which is what makes a clean record detectable as a
// stale file rather than silently wrong.
func dossierIndex(viewer *game.Player) map[string]knownAbout {
	out := map[string]knownAbout{}
	if viewer == nil {
		return out
	}
	for _, in := range viewer.Dossier {
		if in.AboutID == "" {
			continue
		}
		cur := out[in.AboutID]
		if in.Slug != "" {
			cur.secret = game.WeaknessName(in.Slug)
			cur.category = string(game.CategoryOf(in.Slug))
			cur.round = in.Round
		} else if in.Cat != "" && cur.secret == "" {
			cur.category = string(in.Cat)
			cur.round = in.Round
		}
		out[in.AboutID] = cur
	}
	return out
}
