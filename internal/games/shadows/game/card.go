// Package game holds the rules of "State of Shadows: Power & Paranoia", and
// nothing else.
//
// Like its Exploding Kittens and UNO siblings it has no I/O, no timers and no
// idea a browser exists: Apply is a pure reducer over State, so the whole
// rulebook is testable without a server.
//
// The game is social deduction rather than card-shedding, so the shape is
// different from the other two in one way worth knowing up front: almost
// nothing here is public. A player's Weakness, who controls whom, who holds the
// Coup card and what anybody has learned are all private, and the only public
// surfaces are the seat list, the influence counts, the anonymous newsfeed and a
// deliberately vague "someone moved against someone" line in the log. Every
// method that hands information out is in access.go, and the redaction rule is
// enforced there and in view.go rather than being sprinkled through the engine.
package game

import "fmt"

// Role is a player's public position. It is dealt face up and never changes: the
// secret in this game is never who you are, it is what can be done to you.
type Role string

const (
	President Role = "president"
	Police    Role = "police"
	Lawyer    Role = "lawyer"
	Hacker    Role = "hacker"
	Citizen   Role = "citizen"
)

var roleNames = map[Role]string{
	President: "President",
	Police:    "Chief of Police",
	Lawyer:    "Lawyer",
	Hacker:    "Hacker",
	Citizen:   "Citizen",
}

// roleSkills is the name of the once-a-round action each position grants. The
// effects are in engine.go; this is only what the button says.
var roleSkills = map[Role]string{
	President: "Decree",
	Police:    "Investigate",
	Lawyer:    "Inspect",
	Hacker:    "Peek",
	Citizen:   "Rumour",
}

var roleBlurbs = map[Role]string{
	President: "Force one player to publicly disclose the nature of their secret.",
	Police:    "Learn one player's exact Weakness.",
	Lawyer:    "Name a Weakness and learn how many people at this table hold it.",
	Hacker:    "Learn the category of a player's Weakness — twice a round.",
	Citizen:   "Hear a rumour: one random player's exact Weakness.",
}

func (r Role) String() string {
	if n, ok := roleNames[r]; ok {
		return n
	}
	return string(r)
}

// Skill is the display name of this role's action.
func (r Role) Skill() string { return roleSkills[r] }

// Blurb explains the skill in one line, for the client's own UI.
func (r Role) Blurb() string { return roleBlurbs[r] }

// SkillUses is how many times a round the role may act. The Hacker gets two
// because a category is worth much less than a name.
func (r Role) SkillUses() int {
	if r == Hacker {
		return 2
	}
	return 1
}

// roleOrder is the order positions are handed out in. Everybody past the fourth
// seat is a Citizen, which is the point: most of a state is bystanders.
var roleOrder = []Role{President, Police, Lawyer, Hacker}

// Category groups Weaknesses. It is the coarse fact a Hacker peek or a
// presidential decree exposes: enough to narrow the deck down, not enough to
// name the card somebody needs to hold.
type Category string

const (
	Financial Category = "financial"
	Criminal  Category = "criminal"
	Personal  Category = "personal"
	Fraud     Category = "fraud"
	Loyalty   Category = "loyalty"
)

var categoryNames = map[Category]string{
	Financial: "Financial",
	Criminal:  "Criminal",
	Personal:  "Personal",
	Fraud:     "Fraud",
	Loyalty:   "Loyalty",
}

func (c Category) String() string {
	if n, ok := categoryNames[c]; ok {
		return n
	}
	return string(c)
}

// Kind separates the four decks. A single Card type carries all of them because
// they all end up on the same wire and in the same log entries.
type Kind string

const (
	KindWeakness Kind = "weakness"
	KindPower    Kind = "power"
	KindCoup     Kind = "coup"
	KindAntiCoup Kind = "anti-coup"
)

// Card is one physical card. ID is unique for the life of a game so a client can
// point at one specific card without ambiguity.
//
// Exploits is set on Power cards only and names the Weakness slug the card
// destroys. The empty string on a Power card means a wildcard — Kompromat works
// on anybody — and that is deliberately not a separate Kind, so that the rule
// "does this card land?" has exactly one implementation.
type Card struct {
	ID   int      `json:"id"`
	Kind Kind     `json:"kind"`
	Slug string   `json:"slug"`
	Name string   `json:"name"`
	Cat  Category `json:"category,omitempty"`
	// Exploits is the Weakness slug this Power card matches, or "" for a wildcard.
	Exploits string `json:"exploits,omitempty"`
	// Wild marks the wildcard Power cards, so a client can style them without
	// having to know that an empty Exploits means anything at all.
	Wild bool `json:"wild,omitempty"`
	// Blurb is the flavour line the client prints on the face.
	Blurb string `json:"blurb,omitempty"`
}

// weaknessSpec is one entry in the Weakness catalogue.
type weaknessSpec struct {
	slug  string
	name  string
	cat   Category
	blurb string
}

// weaknesses is the ten secrets in the game. Two of each are dealt, so holding a
// Power card is never proof of who it is aimed at.
var weaknesses = []weaknessSpec{
	{"debt", "10M Debt", Financial, "Somebody, somewhere, is owed ten million."},
	{"offshore", "Offshore Accounts", Financial, "Three shells and a lawyer in a warm country."},
	{"corrupt-past", "Corrupt Past", Criminal, "The file was closed. It was not destroyed."},
	{"buried-body", "A Buried Body", Criminal, "One night, one shovel, one witness."},
	{"secret-family", "Secret Family", Personal, "A second address, and a school run you never miss."},
	{"affair", "Affair Tapes", Personal, "Forty minutes of audio you would pay anything for."},
	{"forged-degree", "Forged Degree", Fraud, "The university has no record of you."},
	{"rigged-tender", "Rigged Tender", Fraud, "The winning bid was written in your office."},
	{"leaked-source", "Leaked Source", Loyalty, "You gave a name to a journalist and someone died."},
	{"handler", "Foreign Handler", Loyalty, "You still take the calls."},
}

// powerSpec is one entry in the Power catalogue. exploits is the Weakness slug it
// answers, empty for a wildcard.
type powerSpec struct {
	slug     string
	name     string
	exploits string
	blurb    string
}

var powers = []powerSpec{
	{"bank-records", "Bank Records", "debt", "Every transfer, with dates."},
	{"tax-audit", "Tax Audit", "offshore", "An auditor who does not take calls."},
	{"indictment", "Sealed Indictment", "corrupt-past", "Signed. Not yet filed."},
	{"forensics", "Forensic File", "buried-body", "Soil samples, and a map."},
	{"detective", "Private Detective", "secret-family", "Photographs from across the street."},
	{"the-tapes", "The Tapes", "affair", "Reel one of three."},
	{"credentials", "Credential Check", "forged-degree", "A registrar's letter, on letterhead."},
	{"ledger", "Procurement Ledger", "rigged-tender", "The bid, in your handwriting."},
	{"counter-intel", "Counter-Intel Brief", "leaked-source", "Who told whom, and when."},
	{"wiretap", "Wiretap Transcript", "handler", "Six calls. All of them yours."},
}

// kompromat is the wildcard: it lands on whatever the target is hiding. Rare
// enough that holding one is worth extorting for.
var kompromat = powerSpec{"kompromat", "Kompromat", "", "It does not matter what you did. It matters that we have it."}

const (
	// weaknessCopies and powerCopies keep both decks ambiguous: two of everything
	// means a Power card in your hand never proves whose secret you can reach.
	weaknessCopies = 2
	powerCopies    = 2
	kompromatCount = 2
)

// WeaknessDeck builds the Weakness deck, unshuffled.
func WeaknessDeck(startID int) []Card {
	out := make([]Card, 0, len(weaknesses)*weaknessCopies)
	id := startID
	for i := 0; i < weaknessCopies; i++ {
		for _, w := range weaknesses {
			out = append(out, Card{ID: id, Kind: KindWeakness, Slug: w.slug,
				Name: w.name, Cat: w.cat, Blurb: w.blurb})
			id++
		}
	}
	return out
}

// PowerDeck builds the Power deck, unshuffled, with the wildcards last.
func PowerDeck(startID int) []Card {
	out := make([]Card, 0, len(powers)*powerCopies+kompromatCount)
	id := startID
	add := func(p powerSpec) {
		c := Card{ID: id, Kind: KindPower, Slug: p.slug, Name: p.name,
			Exploits: p.exploits, Blurb: p.blurb, Wild: p.exploits == ""}
		if p.exploits != "" {
			c.Cat = CategoryOf(p.exploits)
		}
		out = append(out, c)
		id++
	}
	for i := 0; i < powerCopies; i++ {
		for _, p := range powers {
			add(p)
		}
	}
	for i := 0; i < kompromatCount; i++ {
		add(kompromat)
	}
	return out
}

// coupCard and antiCoupCard are the two singletons dealt when the Catalyst
// arrives. They are cards rather than flags so that they can be shown, logged
// and revealed like anything else.
func coupCard(id int) Card {
	return Card{ID: id, Kind: KindCoup, Slug: "coup", Name: "Coup",
		Blurb: "Own six tenths of this room and the state is yours."}
}

func antiCoupCard(id int) Card {
	return Card{ID: id, Kind: KindAntiCoup, Slug: "anti-coup", Name: "Anti-Coup",
		Blurb: "Find the hand behind the throne. Cut it off."}
}

// CategoryOf is the category of a Weakness slug, or "" if there is no such
// Weakness.
func CategoryOf(slug string) Category {
	for _, w := range weaknesses {
		if w.slug == slug {
			return w.cat
		}
	}
	return ""
}

// WeaknessName is the printed name of a Weakness slug, falling back to the slug
// so an unknown one reads as odd rather than as blank.
func WeaknessName(slug string) string {
	for _, w := range weaknesses {
		if w.slug == slug {
			return w.name
		}
	}
	return slug
}

// KnownWeakness reports whether a slug names a real Weakness. The Lawyer's
// inspection takes a slug straight off the wire, so it has to be checked.
func KnownWeakness(slug string) bool { return CategoryOf(slug) != "" }

// WeaknessSlugs lists every Weakness in the game, in catalogue order. The client
// builds the Lawyer's dropdown from it, which is why it is exported.
func WeaknessSlugs() []string {
	out := make([]string, 0, len(weaknesses))
	for _, w := range weaknesses {
		out = append(out, w.slug)
	}
	return out
}

// Lands reports whether playing this Power card at somebody holding weakness
// puts them under control. A wildcard lands on anything; anything else has to
// match exactly.
func (c Card) Lands(weakness string) bool {
	if c.Kind != KindPower {
		return false
	}
	return c.Wild || c.Exploits == weakness
}

func (c Card) String() string { return fmt.Sprintf("%s(%d)", c.Name, c.ID) }

// WeaknessCatalogue is one of each Weakness, with ids that belong to no deal.
// The client builds the Lawyer's dropdown and the rules panel from it, so the
// ten secrets are written down exactly once.
func WeaknessCatalogue() []Card {
	out := make([]Card, 0, len(weaknesses))
	for i, w := range weaknesses {
		out = append(out, Card{ID: -1 - i, Kind: KindWeakness, Slug: w.slug,
			Name: w.name, Cat: w.cat, Blurb: w.blurb})
	}
	return out
}

// PowerCatalogue is one of each Power card, for the rules panel: which card
// answers which secret is public knowledge, and hiding it would only mean
// everybody had to learn it by wasting cards.
func PowerCatalogue() []Card {
	out := make([]Card, 0, len(powers)+1)
	id := -100
	for _, p := range append(append([]powerSpec(nil), powers...), kompromat) {
		c := Card{ID: id, Kind: KindPower, Slug: p.slug, Name: p.name,
			Exploits: p.exploits, Blurb: p.blurb, Wild: p.exploits == ""}
		if p.exploits != "" {
			c.Cat = CategoryOf(p.exploits)
		}
		out = append(out, c)
		id--
	}
	return out
}
