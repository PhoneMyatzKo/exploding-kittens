package game

// The two decks: ကံစမ်း (Chance) and ရပ်ရွာရန်ပုံငွေ (Community Chest).
//
// Fifty cards, and every one of them is about living in Myanmar rather than
// about a board game: the power coming back, the bus being full, the checkpoint
// wanting a document you left at home, an aunt insisting you take money. That is
// the whole point of a localised edition — the board is places you know and the
// cards are days you have had.
//
// Split by character, the way the original splits its two decks. Chance is
// movement and luck: transport, weather, checkpoints, jail. Community Chest is
// money and social life: prices, food, family, shopping. Twenty-five each.
//
// On the amounts. The ideas came with figures between 1,000 and 20,000 kyat,
// which read right but are the wrong scale for this board: a property costs
// K60,000–400,000, you start with K1,500,000 and a lap of the board pays
// K200,000, so a K5,000 card is three tenths of one per cent of your money —
// invisible. They are multiplied by ten here, which puts them between 0.7% and
// 13% of starting cash. That is the proportion the original's $50–$200 cards have
// against its $1,500 start, and the relative ordering of the ideas is untouched.
//
// The Burmese is the author's own. The English beside it is a translation rather
// than a second joke, so the two decks say the same thing.

// Deck names which of the two piles a card belongs to.
type Deck string

const (
	DeckChance Deck = "chance"
	DeckChest  Deck = "chest"
)

// EffectKind is what a card does. A card may do two things — pay and move back —
// which is why Card carries a list.
type EffectKind string

const (
	// EffMoney pays you or takes from you.
	EffMoney EffectKind = "money"
	// EffMove walks you forward or back, resolving whatever you land on. Going
	// backwards past GO does not pay.
	EffMove EffectKind = "move"
	// EffSkip costs you your next turn, or turns.
	EffSkip EffectKind = "skip"
	// EffJail sends you straight to jail: no lap, no salary, and you are held
	// there rather than visiting.
	EffJail EffectKind = "jail"
	// EffPardon is kept rather than resolved — the one card you hold in your hand
	// and spend later.
	EffPardon EffectKind = "pardon"
	// EffRelease lets you straight out of jail, and does nothing if you are not
	// in it.
	EffRelease EffectKind = "release"
)

// Effect is one thing a card does. Each kind reads only its own field, which is
// the same bargain core.ClientMsg makes: a handful of scalars beats a union.
type Effect struct {
	Kind EffectKind `json:"kind"`
	// Money is kyat, negative to pay.
	Money int `json:"money,omitempty"`
	// Steps is squares, negative to go back.
	Steps int `json:"steps,omitempty"`
	// Turns is how many turns you miss.
	Turns int `json:"turns,omitempty"`
	// OrJail turns a payment you cannot cover into a trip to jail instead of
	// bankruptcy — the expired-licence card's rule.
	OrJail bool `json:"orJail,omitempty"`
}

// Card is one card, in both languages.
type Card struct {
	Deck  Deck   `json:"deck"`
	Emoji string `json:"emoji"`
	// Title is the headline; Flavour is the line underneath that carries the joke.
	Title     string   `json:"title"`
	TitleMy   string   `json:"titleMy"`
	Flavour   string   `json:"flavour"`
	FlavourMy string   `json:"flavourMy"`
	Effects   []Effect `json:"effects"`
}

// Held reports whether a card is kept in hand rather than resolved on the spot.
// Only the pardon is.
func (c Card) Held() bool {
	return len(c.Effects) == 1 && c.Effects[0].Kind == EffPardon
}

// Shorthands, so the table below reads as a list of cards rather than a list of
// struct literals. k is one card's worth of kyat at this board's scale.
const k = 10 * kyat // the ideas' 1,000 becomes 10,000

func owe(n int) Effect  { return Effect{Kind: EffMoney, Money: -n} }
func gain(n int) Effect { return Effect{Kind: EffMoney, Money: n} }
func fwd(n int) Effect  { return Effect{Kind: EffMove, Steps: n} }
func back(n int) Effect { return Effect{Kind: EffMove, Steps: -n} }
func miss(n int) Effect { return Effect{Kind: EffSkip, Turns: n} }

var toJail = Effect{Kind: EffJail}
var pardon = Effect{Kind: EffPardon}
var release = Effect{Kind: EffRelease}

// oweOrJail is the one conditional in the pack: cover it if you can, and if you
// cannot, you are not bankrupt — you are arrested.
func oweOrJail(n int) Effect { return Effect{Kind: EffMoney, Money: -n, OrJail: true} }

var cards = []Card{
	// ─────────────────────────── ကံစမ်း · Chance ───────────────────────────

	{DeckChance, "⚡", "The power is back!", "မီးပြန်လာပြီ!",
		"Charge everything, quickly.", "အားသွင်းစရာအကုန် မြန်မြန်သွင်း။",
		[]Effect{fwd(3)}},
	{DeckChance, "🕯️", "The power is out", "မီးပျက်ပြီ!",
		"And the phone is on 8%.", "ဖုန်းလည်း 8% ပဲကျန်တယ်။",
		[]Effect{miss(1)}},
	{DeckChance, "🌧️", "It starts raining", "ရုတ်တရက်မိုးရွာတယ်",
		"And the umbrella is at home.", "ထီးကလည်း အိမ်မှာ။",
		[]Effect{back(2)}},
	{DeckChance, "🌂", "You brought the umbrella today", "ဒီနေ့တော့ ထီးယူလာတယ်",
		"Rare preparation achievement unlocked.", "Rare preparation achievement unlocked.",
		[]Effect{fwd(2)}},
	{DeckChance, "🌊", "The road is a lake now", "လမ်းက ရေကြီးနေတယ်",
		"The shortcut is a swimming route.", "Shortcut က အခု swimming route ဖြစ်သွားတယ်။",
		[]Effect{back(3)}},
	{DeckChance, "🚌", "The bus comes straight away", "ဘတ်စ်ကား ချက်ချင်းရတယ်",
		"This kind of luck is rare.", "ဒီလိုကံကောင်းတာ ရှားတယ်။",
		[]Effect{fwd(4)}},
	{DeckChance, "🚌", "The bus is packed", "ဘတ်စ်ကား ပြည့်ကျပ်နေတယ်",
		"It came. You could not get on.", "ကားက လာတယ်၊ ကိုယ်မတက်နိုင်ဘူး။",
		[]Effect{miss(1)}},
	{DeckChance, "🚗", "Traffic jam", "Traffic Jam",
		"Five hundred metres in thirty minutes.", "မီတာ ၅၀၀ သွားတာ မိနစ် ၃၀။",
		[]Effect{back(2)}},
	{DeckChance, "🔋", "Power bank at 100%", "Power Bank 100%",
		"Let the power go out. See if I care.", "ဒီနေ့တော့ မီးပျက်လည်း အေးဆေး။",
		[]Effect{fwd(2)}},
	{DeckChance, "🪫", "Battery at 1%", "Battery 1%",
		"The charger is at home.", "Charger က အိမ်မှာကျန်ခဲ့တယ်။",
		[]Effect{miss(1)}},
	{DeckChance, "🚧", "You took the shortcut. It is closed", "Shortcut ဝင်လိုက်တာ လမ်းပိတ်နေတယ်",
		"The shortcut is now the longcut.", "Shortcut က Longcut ဖြစ်သွားပြီ။",
		[]Effect{back(3)}},
	{DeckChance, "🛵", "A motorbike shortcut", "ဆိုင်ကယ် Shortcut ရတယ်",
		"Straight past the traffic.", "Traffic ကို လွတ်သွားတယ်။",
		[]Effect{fwd(3)}},
	{DeckChance, "🛞", "A flat tyre", "တာယာပေါက်သွားတယ်",
		"So much for today going well.", "ဒီနေ့အဆင်ပြေနေတယ်ထင်တာ မှားတယ်။",
		[]Effect{owe(3 * k), back(1)}},
	{DeckChance, "🩴", "Your sandal snaps", "ဖိနပ်ပြတ်သွားတယ်",
		"In the middle of the road, as usual.", "လမ်းလယ်မှာ ဖြစ်တာကတော့ ပုံမှန်ပဲ။",
		[]Effect{back(1), owe(1 * k)}},
	{DeckChance, "🚨", "Your papers are not in order", "စစ်ဆေးရေးဂိတ်မှာ စာရွက်စာတမ်း မပြည့်စုံဘူး",
		"Left at home, on the one day it matters.", "လိုတဲ့နေ့မှ အိမ်မှာကျန်တယ်။",
		[]Effect{toJail}},
	{DeckChance, "🪪", "You cannot find your ID", "မှတ်ပုံတင်ရှာတာ မတွေ့ဘူး",
		"Everything comes out of the bag except the thing you need.",
		"အိတ်ထဲက ပစ္စည်းအကုန်ထွက်လာတယ်၊ လိုတာပဲမတွေ့ဘူး။",
		[]Effect{toJail, miss(1)}},
	{DeckChance, "🛵", "Your licence has expired", "လိုင်စင်သက်တမ်းကုန်နေတယ်",
		"Checked today, of all days.", "ဒီနေ့မှ စစ်တာနဲ့တည့်တယ်။",
		[]Effect{oweOrJail(5 * k)}},
	{DeckChance, "📄", "You filled the form in the wrong box", "Form ဖြည့်တာ နေရာမှားသွားတယ်",
		"Getting it right first time would not be Myanmar Monopoly.",
		"တစ်ခါတည်းအောင်မြင်ရင် Myanmar Monopoly မဟုတ်ဘူး။",
		[]Effect{miss(1)}},
	{DeckChance, "🚨", `"Just a quick check" is never quick`, `"ခဏစစ်မယ်" ဆိုတာ ခဏမဟုတ်ဘူး`,
		"No telling how long.", "အချိန်ဘယ်လောက်ကြာမလဲ မသိ။",
		[]Effect{toJail}},
	{DeckChance, "🚨", "Two checkpoints in a row", "စစ်ဆေးရေးဂိတ် နှစ်ခုဆက်တိုက်တွေ့တယ်",
		"Still celebrating the first one when the second appears.",
		"ပထမတစ်ခုလွတ်လို့ ပျော်နေတုန်း နောက်တစ်ခုရောက်လာတယ်။",
		[]Effect{toJail}},
	{DeckChance, "🎫", "Get out of jail free", "အချုပ်မှ အခမဲ့ထွက်ခွင့်",
		"Keep this. Spend it when you need it.", "ဒီကဒ်ကို သိမ်းထားပါ။ လိုအပ်တဲ့အခါ အသုံးပြုပါ။",
		[]Effect{pardon}},
	{DeckChance, "📞", "Someone who can help calls", "အကူအညီပေးမယ့်သူတစ်ယောက် ဆက်သွယ်လာတယ်",
		"And it is sorted.", "ကိစ္စက အဆင်ပြေသွားတယ်။",
		[]Effect{release}},
	{DeckChance, "⚡", "Perfect day", "Perfect Day",
		"Power on, good Wi-Fi, clear roads, no rain. Suspicious.",
		"မီးလာတယ်၊ Wi-Fi ကောင်းတယ်၊ ကားမပိတ်ဘူး၊ မိုးလည်းမရွာဘူး။ ဒီလောက်ကံကောင်းတာ သံသယဖြစ်စရာပဲ။",
		[]Effect{fwd(5)}},
	{DeckChance, "😭", "Everything goes wrong", "Everything Goes Wrong",
		"No power, jammed roads, rain, and 2% battery.",
		"မီးပျက် + ကားပိတ် + မိုးရွာ + ဖုန်းအား 2%။",
		[]Effect{back(4)}},
	{DeckChance, "🇲🇲", "Ultimate Myanmar luck", "ULTIMATE MYANMAR LUCK",
		"Cheap food, clear roads, good Wi-Fi, the power on. A legendary day.",
		"အစားအသောက်ဈေးတန်တယ်၊ လမ်းမပိတ်ဘူး၊ Wi-Fi ကောင်းတယ်၊ မီးလည်းလာတယ်။ ဒီနေ့က Legendary Day ပါ။",
		[]Effect{gain(20 * k), fwd(3)}},

	// ──────────────────── ရပ်ရွာရန်ပုံငွေ · Community Chest ────────────────────

	{DeckChest, "⛽", "Fuel is up again", "ဆီဈေးတက်ပြန်ပြီ",
		"Your wallet is quietly weeping.", "ပိုက်ဆံအိတ်က တိတ်တိတ်လေး ငိုနေတယ်။",
		[]Effect{owe(6 * k)}},
	{DeckChest, "⛽", "You filled up before the rise", "ဈေးမတက်ခင် ဆီအပြည့်ဖြည့်ထားတယ်",
		"Timing, for once.", "Timing ကောင်းသွားပြီ။",
		[]Effect{gain(6 * k)}},
	{DeckChest, "💸", "Prices are up again", "ကုန်ဈေးနှုန်းတက်ပြန်ပြီ",
		"Two thousand yesterday, three today.", "မနေ့က 2,000၊ ဒီနေ့ 3,000။",
		[]Effect{owe(3 * k)}},
	{DeckChest, "💱", "I pick up a gold ring", "ရွေလက်စွပ် ကောက်ရတယ်",
		"Sometime luck is on my side.", "တစ်ခါတလေတော့ ကံကောင်းဦးမှပေါ့။",
		[]Effect{gain(8 * k)}},
	{DeckChest, "📉", "The rate improved right after you changed money", "ငွေလဲပြီးမှ Rate ကောင်းသွားတယ်",
		"What is there to say.", "ဘာပြောရမလဲ…",
		[]Effect{owe(7 * k)}},
	{DeckChest, "☕", "A deal at the tea shop", "လက်ဖက်ရည်ဆိုင်မှာ Business Deal ရတယ်",
		"Talking turned into working.", "စကားပြောရင်းနဲ့ အလုပ်ရသွားတယ်။",
		[]Effect{gain(8 * k)}},
	{DeckChest, "☕", `"One more cup" took three hours`, `"နောက်တစ်ခွက်ပဲ" ဆိုပြီး ၃ နာရီကြာသွားတယ်`,
		"No time left to get home.", "အိမ်ပြန်ချိန်မရှိတော့ဘူး။",
		[]Effect{miss(1), owe(2 * k)}},
	{DeckChest, "🚕", "The taxi driver quotes a fair price", "Taxi ဆရာက ဈေးတန်တယ်",
		"An astonishing day.", "ဒီနေ့ အံ့ဩစရာနေ့ပဲ။",
		[]Effect{gain(4 * k)}},
	{DeckChest, "📶", "You find free Wi-Fi", "Free Wi-Fi တွေ့တယ်",
		"It does not even ask for a password.", "Password မတောင်းဘူး။",
		[]Effect{gain(3 * k)}},
	{DeckChest, "📱", "Your mobile data runs out", "Mobile Data ကုန်သွားတယ်",
		"At the worst possible moment.", "အရေးကြီးတဲ့အချိန်မှပဲ။",
		[]Effect{owe(2 * k)}},
	{DeckChest, "🍜", "A friend buys you mohinga", "သူငယ်ချင်းက မုန့်ဟင်းခါးရှင်းပေးတယ်",
		"Free breakfast.", "Free breakfast!",
		[]Effect{gain(3 * k)}},
	{DeckChest, "🍲", `You said "this one is on me"`, `"ဒီတစ်ခါ ငါရှင်းမယ်" လို့ပြောလိုက်တယ်`,
		`Everyone agreed immediately.`, `အားလုံးက "OK" လို့ ချက်ချင်းပြောတယ်။`,
		[]Effect{owe(5 * k)}},
	{DeckChest, "🛍️", "You haggled successfully", "ဈေးဆစ်တာ အောင်မြင်တယ်",
		`You have reached the "fine, take it" stage.`, `"ကဲ ယူယူ" ဆိုတဲ့အဆင့်ရောက်ပြီ။`,
		[]Effect{gain(4 * k)}},
	{DeckChest, "🛍️", "You bought without asking the price", "ဈေးမမေးဘဲ ဝယ်လိုက်မိတယ်",
		"The regret came after the payment.", "ပေးပြီးမှ နောင်တရတယ်။",
		[]Effect{owe(4 * k)}},
	{DeckChest, "💦", "Your phone survived Thingyan", "သင်္ကြန်နေ့ ဖုန်းမစိုဘူး",
		"Phone intact, person intact.", "ဖုန်းလည်းအကောင်း၊ လူလည်းအကောင်း။",
		[]Effect{gain(8 * k)}},
	{DeckChest, "📱", "Your phone got soaked at Thingyan", "သင်္ကြန်မှာ ဖုန်းရေဝင်သွားတယ်",
		"Not sure the bag of rice will save it.", "Rice bag က ကယ်နိုင်မလား မသေချာဘူး။",
		[]Effect{owe(9 * k)}},
	{DeckChest, "💌", "A wedding invitation arrives", "မင်္ဂလာဆောင်ဖိတ်စာရောက်ပြီ",
		"Another envelope.", "နောက်ထပ် စာအိတ်တစ်အိတ်။",
		[]Effect{owe(5 * k)}},
	{DeckChest, "👵", "Grandmother gives you pocket money", "အဘွားက မုန့်ဖိုးပေးတယ်",
		`"Take it, take it," she insists.`, `"ယူထား၊ ယူထား" လို့ အတင်းပေးတယ်။`,
		[]Effect{gain(8 * k)}},
	{DeckChest, "👨‍👩‍👧", "A family gathering", "မိသားစုဆုံပွဲရောက်တယ်",
		`"How is work?" "When are you getting married?"`,
		`"အလုပ်ဘယ်လိုလဲ?" "ဘယ်တော့မင်္ဂလာဆောင်မလဲ?"`,
		[]Effect{miss(1)}},
	{DeckChest, "🍚", "The price of rice is up", "ဆန်ဈေးတက်ပြီ",
		"The household budget is finished.", "အိမ်သုံးစရိတ် Budget ပျက်သွားတယ်။",
		[]Effect{owe(4 * k)}},
	{DeckChest, "🔧", `The mechanic says "there is one more thing"`, `စက်ပြင်ဆရာက "နောက်တစ်ခုရှိသေးတယ်" လို့ပြောတယ်`,
		"Your wallet went cold at those words.", "ဒီစကားကြားတာနဲ့ သေချင်သွားတယ်။",
		[]Effect{owe(8 * k)}},
	{DeckChest, "💰", "Money in an old Trouser", "Trouser အဟောင်းအိတ်ထဲ ပိုက်ဆံတွေ့တယ်",
		"Lucky, with your own money.", "ကိုယ့်ပိုက်ဆံနဲ့ကိုယ် ကံကောင်းနေတယ်။",
		[]Effect{gain(5 * k)}},
	{DeckChest, "📦", "Your online order arrives", "Online Order ရောက်လာတယ်",
		"And it looks like the picture!", "ပုံထဲကအတိုင်း တကယ်ရောက်လာတယ်!",
		[]Effect{gain(5 * k)}},
	{DeckChest, "📦", "Expectation versus reality", "Expectation vs Reality",
		"You ordered one thing. Something else arrived.", "မှာထားတာတစ်မျိုး၊ ရောက်လာတာတစ်မျိုး။",
		[]Effect{owe(3 * k)}},
	{DeckChest, "🧋", `A friend says "I will get this"`, `သူငယ်ချင်းက "ငါရှင်းမယ်" လို့ပြောတယ်`,
		"Do not argue.", "အငြင်းအခုံ မလုပ်ပါနဲ့။",
		[]Effect{gain(4 * k)}},
}

// Cards returns the whole pack, for the client to render and for tests to read.
// A copy: nothing may edit the pack.
func Cards() []Card { return append([]Card(nil), cards...) }

// deckOf is every card in one pile, as indices into the pack. Indices rather than
// copies, so a shuffled pile is a list of small integers that marshals cheaply
// and points at one source of truth for the text.
func deckOf(d Deck) []int {
	var out []int
	for i, c := range cards {
		if c.Deck == d {
			out = append(out, i)
		}
	}
	return out
}

// CardAt is one card of the pack. Panics on a bad index, which would be a bug in
// this package rather than anything a player can cause.
func CardAt(i int) Card { return cards[i] }
