package game

// The board: forty tiles, in order, starting at GO and running clockwise.
//
// This is the Myanmar edition. The *structure* is the original board's — which
// square is a property, which is a station, where the taxes and the corners
// fall, and what everything costs — because those numbers are eighty years
// balanced and changing them changes the game rather than the theme. What is
// Myanmar is the places on them, ordered the way the original orders its
// streets: the cheapest pair in the first corner, the crown jewels in the last.
//
// Prices are the original's, scaled by kyat below. Rents are the unimproved
// figures; the improved ones are here too so that houses, when they land, are a
// rules change and not a data-entry job.
//
// Every name is carried in both languages. It has to come from the server rather
// than a table in the client: what a square is called is game data, and everyone
// at the table has to be looking at the same board.

// kyat scales the original board's printed dollars onto amounts that read as
// money here — the cheapest property is K60,000 rather than K60. One constant so
// the whole board rescales together if these turn out to feel wrong.
const kyat = 1_000

// StartingCash and PassGo are the two amounts that are not a tile's.
const (
	StartingCash = 1_500 * kyat
	PassGo       = 200 * kyat
)

// TileKind is what landing on a square does.
type TileKind string

const (
	TileGo       TileKind = "go"
	TileProperty TileKind = "property"
	TileStation  TileKind = "station"
	TileUtility  TileKind = "utility"
	TileChance   TileKind = "chance"
	TileChest    TileKind = "chest"
	TileTax      TileKind = "tax"
	TileJail     TileKind = "jail"
	TileParking  TileKind = "parking"
	TileGoToJail TileKind = "gotojail"
)

// Buyable reports whether a square can be owned. The three that can are the ones
// that charge rent, which is the only reason ownership exists.
func (k TileKind) Buyable() bool {
	return k == TileProperty || k == TileStation || k == TileUtility
}

// Tile is one square.
type Tile struct {
	Kind   TileKind `json:"kind"`
	Name   string   `json:"name"`
	NameMy string   `json:"nameMy"`
	// Group ties a property to its colour set. Empty on everything else —
	// stations and utilities have their own rent rules and are grouped by kind.
	Group string `json:"group,omitempty"`
	Price int    `json:"price,omitempty"`
	// Rent is the unimproved rent followed by one, two, three, four houses and a
	// hotel. Only Rent[0] is reachable until building lands; the rest is data,
	// not dead code, and having it here means that change touches no numbers.
	Rent [6]int `json:"rent,omitempty"`
	// House is what one house costs on this property, which in the original
	// depends on the colour set rather than on the property.
	House int `json:"house,omitempty"`
	// Tax is what a tax square charges.
	Tax int `json:"tax,omitempty"`
}

// Colour groups, in board order. Named rather than numbered so a rule about "all
// three oranges" reads as one.
const (
	GroupBrown  = "brown"
	GroupLBlue  = "lightblue"
	GroupPink   = "pink"
	GroupOrange = "orange"
	GroupRed    = "red"
	GroupYellow = "yellow"
	GroupGreen  = "green"
	GroupDBlue  = "darkblue"
)

// BoardSize is the number of squares, and so the modulus every move is taken
// under.
const BoardSize = 40

// Jail positions.
const (
	JailTile     = 10
	GoToJailTile = 30
)

var board = [BoardSize]Tile{
	0:  {Kind: TileGo, Name: "GO", NameMy: "စတင်"},
	1:  {Kind: TileProperty, Name: "Myeik", NameMy: "မြိတ်", Group: GroupBrown, Price: 60 * kyat, Rent: [6]int{2 * kyat, 10 * kyat, 30 * kyat, 90 * kyat, 160 * kyat, 250 * kyat}, House: 50 * kyat},
	2:  {Kind: TileChest, Name: "Community Chest", NameMy: "ရပ်ရွာရန်ပုံငွေ"},
	3:  {Kind: TileProperty, Name: "Dawei", NameMy: "ထားဝယ်", Group: GroupBrown, Price: 60 * kyat, Rent: [6]int{4 * kyat, 20 * kyat, 60 * kyat, 180 * kyat, 320 * kyat, 450 * kyat}, House: 50 * kyat},
	4:  {Kind: TileTax, Name: "Income Tax", NameMy: "ဝင်ငွေခွန်", Tax: 200 * kyat},
	5:  {Kind: TileStation, Name: "Yangon Central Station", NameMy: "ရန်ကုန်ဘူတာကြီး", Price: 200 * kyat},
	6:  {Kind: TileProperty, Name: "Magway", NameMy: "မကွေး", Group: GroupLBlue, Price: 100 * kyat, Rent: [6]int{6 * kyat, 30 * kyat, 90 * kyat, 270 * kyat, 400 * kyat, 550 * kyat}, House: 50 * kyat},
	7:  {Kind: TileChance, Name: "Chance", NameMy: "ကံစမ်း"},
	8:  {Kind: TileProperty, Name: "Monywa", NameMy: "မုံရွာ", Group: GroupLBlue, Price: 100 * kyat, Rent: [6]int{6 * kyat, 30 * kyat, 90 * kyat, 270 * kyat, 400 * kyat, 550 * kyat}, House: 50 * kyat},
	9:  {Kind: TileProperty, Name: "Pathein", NameMy: "ပသိမ်", Group: GroupLBlue, Price: 120 * kyat, Rent: [6]int{8 * kyat, 40 * kyat, 100 * kyat, 300 * kyat, 450 * kyat, 600 * kyat}, House: 50 * kyat},
	10: {Kind: TileJail, Name: "Jail", NameMy: "အချုပ်"},
	11: {Kind: TileProperty, Name: "Mawlamyine", NameMy: "မော်လမြိုင်", Group: GroupPink, Price: 140 * kyat, Rent: [6]int{10 * kyat, 50 * kyat, 150 * kyat, 450 * kyat, 625 * kyat, 750 * kyat}, House: 100 * kyat},
	12: {Kind: TileUtility, Name: "Electricity Board", NameMy: "လျှပ်စစ်ဓာတ်အား", Price: 150 * kyat},
	13: {Kind: TileProperty, Name: "Sittwe", NameMy: "စစ်တွေ", Group: GroupPink, Price: 140 * kyat, Rent: [6]int{10 * kyat, 50 * kyat, 150 * kyat, 450 * kyat, 625 * kyat, 750 * kyat}, House: 100 * kyat},
	14: {Kind: TileProperty, Name: "Taunggyi", NameMy: "တောင်ကြီး", Group: GroupPink, Price: 160 * kyat, Rent: [6]int{12 * kyat, 60 * kyat, 180 * kyat, 500 * kyat, 700 * kyat, 900 * kyat}, House: 100 * kyat},
	15: {Kind: TileStation, Name: "Pyay Station", NameMy: "ပြည်ဘူတာ", Price: 200 * kyat},
	16: {Kind: TileProperty, Name: "Hpa-An", NameMy: "ဘားအံ", Group: GroupOrange, Price: 180 * kyat, Rent: [6]int{14 * kyat, 70 * kyat, 200 * kyat, 550 * kyat, 750 * kyat, 950 * kyat}, House: 100 * kyat},
	17: {Kind: TileChest, Name: "Community Chest", NameMy: "ရပ်ရွာရန်ပုံငွေ"},
	18: {Kind: TileProperty, Name: "Kalaw", NameMy: "ကလော", Group: GroupOrange, Price: 180 * kyat, Rent: [6]int{14 * kyat, 70 * kyat, 200 * kyat, 550 * kyat, 750 * kyat, 950 * kyat}, House: 100 * kyat},
	19: {Kind: TileProperty, Name: "Pyin Oo Lwin", NameMy: "ပြင်ဦးလွင်", Group: GroupOrange, Price: 200 * kyat, Rent: [6]int{16 * kyat, 80 * kyat, 220 * kyat, 600 * kyat, 800 * kyat, 1000 * kyat}, House: 100 * kyat},
	20: {Kind: TileParking, Name: "Free Parking", NameMy: "အခမဲ့ ရပ်နားရန်"},
	21: {Kind: TileProperty, Name: "Meiktila", NameMy: "မိတ္ထီလာ", Group: GroupRed, Price: 220 * kyat, Rent: [6]int{18 * kyat, 90 * kyat, 250 * kyat, 700 * kyat, 875 * kyat, 1050 * kyat}, House: 150 * kyat},
	22: {Kind: TileChance, Name: "Chance", NameMy: "ကံစမ်း"},
	23: {Kind: TileProperty, Name: "Bago", NameMy: "ပဲခူး", Group: GroupRed, Price: 220 * kyat, Rent: [6]int{18 * kyat, 90 * kyat, 250 * kyat, 700 * kyat, 875 * kyat, 1050 * kyat}, House: 150 * kyat},
	24: {Kind: TileProperty, Name: "Nay Pyi Taw", NameMy: "နေပြည်တော်", Group: GroupRed, Price: 240 * kyat, Rent: [6]int{20 * kyat, 100 * kyat, 300 * kyat, 750 * kyat, 925 * kyat, 1100 * kyat}, House: 150 * kyat},
	25: {Kind: TileStation, Name: "Thazi Junction", NameMy: "သာစည်ဘူတာ", Price: 200 * kyat},
	26: {Kind: TileProperty, Name: "Chaung Tha", NameMy: "ချောင်းသာ", Group: GroupYellow, Price: 260 * kyat, Rent: [6]int{22 * kyat, 110 * kyat, 330 * kyat, 800 * kyat, 975 * kyat, 1150 * kyat}, House: 150 * kyat},
	27: {Kind: TileProperty, Name: "Ngwe Saung", NameMy: "ငွေဆောင်", Group: GroupYellow, Price: 260 * kyat, Rent: [6]int{22 * kyat, 110 * kyat, 330 * kyat, 800 * kyat, 975 * kyat, 1150 * kyat}, House: 150 * kyat},
	28: {Kind: TileUtility, Name: "Water Supply", NameMy: "ရေပေးရေး", Price: 150 * kyat},
	29: {Kind: TileProperty, Name: "Ngapali", NameMy: "ငပလီ", Group: GroupYellow, Price: 280 * kyat, Rent: [6]int{24 * kyat, 120 * kyat, 360 * kyat, 850 * kyat, 1025 * kyat, 1200 * kyat}, House: 150 * kyat},
	30: {Kind: TileGoToJail, Name: "Go To Jail", NameMy: "အချုပ်သို့"},
	31: {Kind: TileProperty, Name: "Mandalay", NameMy: "မန္တလေး", Group: GroupGreen, Price: 300 * kyat, Rent: [6]int{26 * kyat, 130 * kyat, 390 * kyat, 900 * kyat, 1100 * kyat, 1275 * kyat}, House: 200 * kyat},
	32: {Kind: TileProperty, Name: "Kyaiktiyo", NameMy: "ကျိုက်ထီးရိုး", Group: GroupGreen, Price: 300 * kyat, Rent: [6]int{26 * kyat, 130 * kyat, 390 * kyat, 900 * kyat, 1100 * kyat, 1275 * kyat}, House: 200 * kyat},
	33: {Kind: TileChest, Name: "Community Chest", NameMy: "ရပ်ရွာရန်ပုံငွေ"},
	34: {Kind: TileProperty, Name: "Inle Lake", NameMy: "အင်းလေးကန်", Group: GroupGreen, Price: 320 * kyat, Rent: [6]int{28 * kyat, 150 * kyat, 450 * kyat, 1000 * kyat, 1200 * kyat, 1400 * kyat}, House: 200 * kyat},
	35: {Kind: TileStation, Name: "Mandalay Station", NameMy: "မန္တလေးဘူတာ", Price: 200 * kyat},
	36: {Kind: TileChance, Name: "Chance", NameMy: "ကံစမ်း"},
	37: {Kind: TileProperty, Name: "Bagan", NameMy: "ပုဂံ", Group: GroupDBlue, Price: 350 * kyat, Rent: [6]int{35 * kyat, 175 * kyat, 500 * kyat, 1100 * kyat, 1300 * kyat, 1500 * kyat}, House: 200 * kyat},
	38: {Kind: TileTax, Name: "Luxury Tax", NameMy: "ဇိမ်ခံခွန်", Tax: 100 * kyat},
	39: {Kind: TileProperty, Name: "Shwedagon Pagoda", NameMy: "ရွှေတိဂုံစေတီ", Group: GroupDBlue, Price: 400 * kyat, Rent: [6]int{50 * kyat, 200 * kyat, 600 * kyat, 1400 * kyat, 1700 * kyat, 2000 * kyat}, House: 200 * kyat},
}

// Board returns the whole board, for the client to draw and for tests to read.
// A copy: the board is the one thing in this package nothing may edit.
func Board() []Tile { return append([]Tile(nil), board[:]...) }

// TileAt is one square. Positions are always taken modulo the board, so this
// cannot be called out of range by a move.
func TileAt(pos int) Tile { return board[((pos%BoardSize)+BoardSize)%BoardSize] }

// groupSize is how many properties share a colour set — two for the first and
// last, three for the rest. Counted rather than written down, so it cannot fall
// out of step with the board above.
func groupSize(group string) int {
	n := 0
	for _, t := range board {
		if t.Group == group {
			n++
		}
	}
	return n
}
