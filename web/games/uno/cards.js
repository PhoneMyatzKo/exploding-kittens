// Every UNO card, drawn rather than downloaded.
//
// Exploding Kittens ships a JPEG per card because each one is a different
// illustration. UNO isn't: a card is a colour, a symbol and two corner markers,
// which is a shape a few dozen lines of SVG describe exactly and at any size. So
// nothing here is fetched — no /cards/ route, no sprite sheet, no 108 images to
// pay for on a phone connection, and no blank gaps while they load.
//
// The one thing this file must never do is invent card names. It is handed what
// the server dealt (`{colour, rank}`, both slugs from internal/games/uno/game)
// and draws that. If a slug arrives that it doesn't know, it draws a blank in the
// right colour rather than throwing, because a wrong-looking card is a bug you
// can see and an exception mid-render is a table that never appears.

// The four printed colours. Not tokens in style.css on purpose: these are the
// cards' own inks, fixed by the game, and they do not re-theme.
export const INKS = {
  red: "#ec2229",
  yellow: "#f8d92b",
  green: "#379f4b",
  blue: "#1272c4",
  wild: "#141414",
};

// The order the wild cards show their colours in, clockwise from the top left.
const WHEEL = ["red", "yellow", "green", "blue"];

const W = 140;
const H = 200;

// Cards are drawn at 140×200 and scaled by CSS, so this is the only place the
// proportions are written down.
export const ASPECT = W / H;

const NUMBERS = new Set(["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"]);

// numeral is the text a card wears, where it wears text: the ten digits, and the
// "+2" and "+4" the draw cards carry. Everything else is a glyph.
function numeral(rank) {
  if (NUMBERS.has(rank)) return rank;
  if (rank === "draw-two") return "+2";
  if (rank === "wild-draw-four") return "+4";
  return null;
}

// label is what a screen reader gets, and what a test can look for. The server
// already sends a name; this is the fallback for when it doesn't.
export function label(card) {
  if (card?.name) return card.name;
  const rank = (card?.rank ?? "").replace(/-/g, " ");
  const colour = card?.colour === "wild" ? "" : `${card?.colour ?? ""} `;
  return `${colour}${rank}`.trim() || "card";
}

// face returns the SVG for one card, as markup.
//
// Structure, outermost first: the white border, the coloured body, the tilted
// white oval every UNO card carries, the big central symbol, and two corner
// markers — the second rotated, because a card is read from either end.
export function face(card) {
  const colour = card?.colour ?? "wild";
  const rank = String(card?.rank ?? "");
  const ink = INKS[colour] ?? INKS.wild;
  const body = colour === "wild" ? INKS.wild : ink;

  return svg(
    label(card),
    `<rect x="0" y="0" width="${W}" height="${H}" rx="14" fill="#ffffff"/>
     <rect x="7" y="7" width="${W - 14}" height="${H - 14}" rx="9" fill="${body}"/>
     <ellipse cx="${W / 2}" cy="${H / 2}" rx="54" ry="31" fill="#ffffff"
              transform="rotate(-20 ${W / 2} ${H / 2})"/>
     ${centre(rank, ink)}
     ${corner(rank, 26, 30, 1)}
     ${corner(rank, W - 26, H - 30, -1)}`
  );
}

// back is the face-down card: the deck, and everybody else's hand.
export function back() {
  return svg(
    "face-down card",
    `<rect x="0" y="0" width="${W}" height="${H}" rx="14" fill="#ffffff"/>
     <rect x="7" y="7" width="${W - 14}" height="${H - 14}" rx="9" fill="${INKS.wild}"/>
     <ellipse cx="${W / 2}" cy="${H / 2}" rx="54" ry="31" fill="${INKS.red}"
              transform="rotate(-20 ${W / 2} ${H / 2})"/>
     <text x="${W / 2}" y="${H / 2}" text-anchor="middle" dominant-baseline="central"
           transform="rotate(-20 ${W / 2} ${H / 2})"
           font-size="34" font-weight="900" font-style="italic"
           font-family="Arial Black, Arial, Helvetica, sans-serif"
           fill="#ffffff" stroke="${INKS.wild}" stroke-width="1.5"
           paint-order="stroke" letter-spacing="1">UNO</text>`
  );
}

// element wraps either side of a card in a <div> the table can position, size
// and click. Nothing here knows what a hand looks like; that is the game
// module's business.
export function element(card, { faceDown = false } = {}) {
  const el = document.createElement("div");
  el.className = "uno-card";
  el.dataset.colour = faceDown ? "back" : card?.colour ?? "wild";
  el.dataset.rank = faceDown ? "back" : String(card?.rank ?? "");
  if (!faceDown && card?.id !== undefined) el.dataset.id = String(card.id);
  el.setAttribute("aria-label", faceDown ? "face-down card" : label(card));
  el.innerHTML = faceDown ? back() : face(card);
  return el;
}

// swatch draws just a colour, for the wild picker and for showing which colour
// is in force. Same inks as the cards, so the picker cannot drift from them.
export function swatch(colour) {
  const ink = INKS[colour] ?? INKS.wild;
  return svg(
    `${colour}`,
    `<rect x="0" y="0" width="${W}" height="${H}" rx="14" fill="#ffffff"/>
     <rect x="7" y="7" width="${W - 14}" height="${H - 14}" rx="9" fill="${ink}"/>`
  );
}

// ───────────────────────────────────────────────────────────── the symbols

// Every symbol is drawn in a 40×40 box centred on the origin, so the same
// definition serves the big one in the oval and the small ones in the corners:
// the only difference is the scale it is placed at.
function glyph(rank) {
  if (numeral(rank)) return null; // text, and the callers draw that themselves

  switch (rank) {
    case "skip":
      return `<circle cx="0" cy="0" r="14" fill="none" stroke="currentColor" stroke-width="5"/>
              <line x1="-10" y1="10" x2="10" y2="-10" stroke="currentColor" stroke-width="5"
                    stroke-linecap="round"/>`;

    case "reverse":
      // Two arrows passing each other. Filled rather than stroked so they stay
      // solid at corner size, where a 2px stroke would disappear.
      return `<path d="M-15,-12 H1 V-17 L15,-8 L1,1 V-4 H-15 Z" fill="currentColor"/>
              <path d="M15,4 H-1 V-1 L-15,8 L-1,17 V12 H15 Z" fill="currentColor"/>`;

    case "wild":
      // The four-colour wheel. Quadrant wedges rather than stripes: at corner
      // size stripes turn into mush, and a quarter each still reads as "any".
      return WHEEL.map((c, i) => {
        const a0 = (i * 90 - 90) * (Math.PI / 180);
        const a1 = a0 + Math.PI / 2;
        const r = 16;
        const x0 = (r * Math.cos(a0)).toFixed(2);
        const y0 = (r * Math.sin(a0)).toFixed(2);
        const x1 = (r * Math.cos(a1)).toFixed(2);
        const y1 = (r * Math.sin(a1)).toFixed(2);
        return `<path d="M0,0 L${x0},${y0} A${r},${r} 0 0 1 ${x1},${y1} Z" fill="${INKS[c]}"/>`;
      }).join("");
  }
  // The Wild Draw Four is not here: its middle is drawn by centre() and its
  // corners read "+4", so it never needs one symbol serving both sizes.
  return "";
}

// miniCards is the Wild Draw Four's symbol: four little cards, one per colour,
// in the same order as the wheel on the plain Wild.
function miniCards() {
  const w = 14;
  const h = 20;
  const at = [
    [-16, -22],
    [2, -22],
    [-16, 2],
    [2, 2],
  ];
  return WHEEL.map(
    (c, i) => `<rect x="${at[i][0]}" y="${at[i][1]}" width="${w}" height="${h}" rx="3"
                     fill="${INKS[c]}" stroke="#ffffff" stroke-width="2"/>`
  ).join("");
}

// centre is the big symbol in the oval. Numerals get a dark outline: a yellow 7
// on a white oval is authentic and unreadable, and the printed cards outline
// them too.
function centre(rank, ink) {
  // The printed card puts the four little cards in the middle and the +4 in the
  // corners, and the corners are where you read a card from in a fanned hand —
  // so that is exactly where each belongs.
  if (rank === "wild-draw-four") {
    return `<g transform="translate(${W / 2} ${H / 2}) rotate(-20) scale(1.45)">${miniCards()}</g>`;
  }
  const text = numeral(rank);
  if (text) {
    // Two characters need a smaller size than one, or "+2" runs off the oval.
    const size = text.length > 1 ? 58 : 82;
    return `<text x="${W / 2}" y="${H / 2 + 2}" text-anchor="middle" dominant-baseline="central"
                  font-size="${size}" font-weight="900"
                  font-family="Arial Black, Arial, Helvetica, sans-serif"
                  fill="${ink}" stroke="#1a1a1a" stroke-width="2.5" paint-order="stroke"
                  transform="rotate(-20 ${W / 2} ${H / 2})">${text}</text>`;
  }
  const g = glyph(rank);
  if (!g) return "";
  return `<g transform="translate(${W / 2} ${H / 2}) rotate(-20) scale(1.45)"
             color="${rank === "wild" ? INKS.wild : ink}">${g}</g>`;
}

// corner is the small marker, drawn twice: once each way up, so the card reads
// from either end the way a real one does. dir flips the second copy.
function corner(rank, x, y, dir) {
  const rot = dir === 1 ? 0 : 180;
  const text = numeral(rank);
  if (text) {
    return `<text x="${x}" y="${y}" text-anchor="middle" dominant-baseline="central"
                  transform="rotate(${rot} ${x} ${y})"
                  font-size="${text.length > 1 ? 22 : 30}" font-weight="900"
                  font-family="Arial Black, Arial, Helvetica, sans-serif"
                  fill="#ffffff">${text}</text>`;
  }
  const g = glyph(rank);
  if (!g) return "";
  return `<g transform="translate(${x} ${y}) rotate(${rot}) scale(0.5)" color="#ffffff">${g}</g>`;
}

function svg(title, body) {
  // focusable="false" and aria-hidden keep the inner SVG out of the tab order and
  // out of the accessibility tree — the wrapper element carries the label.
  return `<svg viewBox="0 0 ${W} ${H}" width="100%" height="100%" focusable="false"
               aria-hidden="true" role="img"><title>${escape(title)}</title>${body}</svg>`;
}

function escape(s) {
  return String(s).replace(/[<>&"]/g, (c) => `&#${c.charCodeAt(0)};`);
}
