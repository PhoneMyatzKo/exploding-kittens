// State of Shadows — the client half of one game.
//
// The shell (../../app.js) owns the socket, the menu, the lobby and the screens
// every game shares. This module owns everything from the moment the table is
// dealt: the three information panels, the hand, every prompt, and the rules
// text. It never touches the socket directly and never reads the shell's state —
// both arrive through the ctx handed to mount().
//
// The server is authoritative for every rule. Nothing here decides whether a
// move is legal: `view.me.can` says which buttons are live, and everything else
// is sent as an intent and re-rendered from whatever comes back. That matters
// more in this game than in the card games, because a client that guessed at a
// rule would have to know things it is deliberately not told.
//
// Every prompt goes through the shared modal rather than through a picking mode
// with seat click handlers. One reason: a state can arrive at any moment (a
// proposal, a pardon, a round ending) and a half-finished selection living in
// the seat list would be repainted out from under the player's finger.

import { $ } from "../../core/dom.js";
import { logOpen, setStoredLogOpen } from "../../core/store.js";
import { openModal, closeModal, modalKind } from "../../core/modal.js";
import { renderLog, logPrivate, freshEvents, scrollLogToEnd } from "../../core/feed.js";

// The latest view. Kept because a modal built on one state has to be able to
// read the board when its button is finally pressed, and re-reading through ctx
// is what guarantees it is not acting on a board that has since moved.
let ctx = null;
const latest = () => ctx.view();

const nameOf = (id) => ctx.nameOf(id);
const send = (msg) => ctx.send(msg);

// ────────────────────────────────────────────────────────── the module

export default {
  // The cat portraits belong to Exploding Kittens. A cabinet meeting wearing
  // them is the wrong game.
  avatars: false,

  slots: ["sound-table"],

  mount(context) {
    ctx = context;

    $("sh-skill").onclick = () => useSkill();
    $("sh-power").onclick = () => choosePowerCard();
    $("sh-clean").onclick = () => confirmClean();
    $("sh-evidence").onclick = () => confirmEvidence();
    $("sh-seize").onclick = () => chooseSeizeTarget();
    $("sh-accuse").onclick = () => chooseAccused();
    $("sh-pass").onclick = () => send({ type: "pass" });

    $("sh-offer").onclick = () => composeOffer();
    $("sh-news").onclick = () => composeNews();
    $("sh-leak").onclick = () => send({ type: "leak" });
    $("sh-pardon").onclick = () => choosePardon();

    $("log-collapse").onclick = () => setLogOpen(!logOpen());
    $("rules-table").onclick = () => showRules(true);
    $("rules-close").onclick = () => showRules(false);
    $("lang-en").onclick = () => setRulesLang("en");
    $("lang-my").onclick = () => setRulesLang("my");
    setRulesLang(rulesLang());
    setLogOpen(logOpen());
  },

  unmount() {
    ctx = null;
  },

  render(v) {
    $("table").hidden = false;
    renderTable(v);
  },

  leaveTable() {
    $("table").hidden = true;
    $("sh-offers").hidden = true;
  },

  // Runs before render(), and — unlike render() — on every state including the
  // lobby's, which is the only reason it exists rather than living in
  // renderTable(): the rules sheet opens from the lobby, before the game the
  // rest of this module renders has even been dealt, and the catalogue table in
  // it needs the server's card list regardless.
  onState(v) {
    freshEvents(v);
    renderRulesCards(v);
  },

  onPrivate(e) { handlePrivate(e); },

  onEscape() {
    if (!$("rules-panel").hidden) { showRules(false); return true; }
    return false;
  },

  showRules,
};

// ────────────────────────────────────────────────────────── the table

function renderTable(v) {
  $("table-code").textContent = v.code;
  renderRound(v);
  renderTurnBanner(v);
  renderSeats(v);
  renderMePanel(v);
  renderFeed(v);
  renderDossier(v);
  renderHand(v);
  renderActions(v);
  renderOffers(v);
  renderLog(v, logLine);
  renderGameOver(v);
}

const PHASE_NAMES = {
  cold_war: "The Cold War",
  arms_race: "The Arms Race",
  catalyst: "The Catalyst",
  game_over: "Over",
};

function renderRound(v) {
  const phase = PHASE_NAMES[v.phase] || v.phase;
  $("sh-round").textContent = v.phase === "game_over"
    ? phase
    : `Round ${v.round}/${v.finalRound} · ${phase}`;
}

function renderTurnBanner(v) {
  const el = $("turn-banner");
  el.classList.toggle("mine", !!v.me.myTurn);
  if (v.phase === "game_over") {
    el.textContent = v.reason || "The files are open.";
    return;
  }
  if (v.me.myTurn) {
    el.textContent = v.me.acted ? "Your action is spent" : "Your move";
    return;
  }
  el.textContent = v.currentId ? `Waiting on ${nameOf(v.currentId)}` : "";
}

// A seat carries the public facts, plus two viewer-relative ones: whether they
// are your Pawn, and the sharpest thing your own dossier says about them.
function renderSeats(v) {
  const box = $("seats");
  box.replaceChildren(...v.seats.map((s) => seatEl(s, v)));
}

function seatEl(s, v) {
  const el = document.createElement("div");
  el.className = "seat";
  if (s.current) el.classList.add("current");

  const name = document.createElement("div");
  name.className = "seat-name";
  name.textContent = s.name + (s.id === v.me.id ? " (you)" : "");
  el.append(name);

  const meta = document.createElement("div");
  meta.className = "seat-meta";
  meta.textContent = `${s.roleName} · ${s.influence} inf`;
  el.append(meta);

  const badges = document.createElement("div");
  badges.className = "sh-badges";
  const add = (text, cls) => {
    const b = document.createElement("span");
    b.className = "sh-badge" + (cls ? " " + cls : "");
    b.textContent = text;
    badges.append(b);
  };
  add(`${s.handCount} card${s.handCount === 1 ? "" : "s"}`);
  if (s.pawn) add("yours", "pawn");
  if (s.knownSecret) add(s.knownSecret, s.cleared ? "known stale" : "known");
  else if (s.knownCategory) add(s.knownCategory, s.cleared ? "stale" : "");
  if (s.cleared) add("file is old", "stale");
  if (s.acted && v.phase !== "game_over") add("acted", "done");
  if (!s.connected) add("away", "done");
  // Once it is over, everything comes out.
  if (s.secret) {
    add(s.secret.name, "known");
    if (s.masterName) add(`owned by ${s.masterName}`, "pawn");
    if (s.coup) add("Mastermind", "pawn");
    if (s.antiCoup) add("Resistance", "known");
    if (s.won) add("won", "pawn");
  }
  el.append(badges);
  return el;
}

// Your own half of the world.
function renderMePanel(v) {
  const box = $("sh-me");
  const me = v.me;
  box.replaceChildren();

  const head = document.createElement("h3");
  head.className = "sh-panel-head";
  head.textContent = "Your position";
  box.append(head);

  if (!me.roleName) {
    box.append(el("p", "sh-empty", "You are watching this one."));
    return;
  }

  box.append(el("p", "sh-role", me.roleName));
  box.append(el("p", "sh-skill-blurb", `${me.skill}: ${me.skillBlurb}`));

  const facts = document.createElement("div");
  facts.className = "sh-facts";
  const fact = (text, cls) => facts.append(el("span", "sh-fact" + (cls ? " " + cls : ""), text));
  fact(`${me.influence} influence`);
  fact(`${me.skillLeft} use${me.skillLeft === 1 ? "" : "s"} left`);
  if (me.held > 0) fact(`${me.held} of ${v.coupTarget} owned`, "own");
  if (me.coup) fact("You hold the Coup", "own");
  if (me.antiCoup) fact("You hold the Anti-Coup", "own");
  if (me.compromised) {
    fact(me.masterName ? `Owned by ${me.masterName}` : "Compromised", "hot");
    fact(`Evidence ${me.evidence}/${v.costs.evidenceToFree}`, "hot");
  }
  if (me.protected) fact("Protected this round", "own");
  box.append(facts);

  if (me.secret) box.append(cardEl(me.secret, { className: "secret" }));

  // A Pawn's hand is the owner's to see. This is what control is actually worth
  // before the final count, so it is on screen rather than behind a button.
  for (const pawn of me.pawns || []) {
    box.append(el("p", "sh-panel-head", `${pawn.name}'s hand`));
    if (!pawn.hand.length) box.append(el("p", "sh-empty", "Nothing left to take."));
    for (const c of pawn.hand) box.append(cardEl(c));
  }
}

function renderFeed(v) {
  const box = $("sh-feed");
  if (!v.feed.length) {
    box.replaceChildren(el("li", "sh-empty", "Nobody has said anything out loud yet."));
    return;
  }
  box.replaceChildren(...v.feed.map((post) => {
    const li = document.createElement("li");
    li.append(el("span", "sh-meta",
      post.author ? `Round ${post.round} · ${post.author}` : `Round ${post.round} · anonymous`));
    li.append(document.createTextNode(post.text));
    return li;
  }));
}

function renderDossier(v) {
  const box = $("sh-dossier");
  const notes = v.me.dossier || [];
  if (!notes.length) {
    box.replaceChildren(el("li", "sh-empty", "You know nothing yet."));
    return;
  }
  // Newest first: the note you want is almost always the one you just earned.
  box.replaceChildren(...notes.slice().reverse().map((n) => {
    const li = document.createElement("li");
    li.append(el("span", "sh-meta", `Round ${n.round}`));
    li.append(document.createTextNode(n.text));
    return li;
  }));
}

function renderHand(v) {
  const box = $("hand");
  const hand = v.me.hand || [];
  if (!hand.length) {
    box.replaceChildren(el("p", "sh-empty",
      v.round < 2 ? "Power cards are dealt in round two." : "You are holding nothing."));
    return;
  }
  box.replaceChildren(...hand.map((c) => {
    const btn = cardEl(c, { button: true, dim: !v.me.can.power });
    btn.onclick = () => {
      if (!v.me.can.power) {
        ctx.toast(v.me.acted ? "Your action this round is spent." : "Not your turn yet.");
        return;
      }
      choosePowerTarget(c);
    };
    return btn;
  }));
}

function renderActions(v) {
  const can = v.me.can;
  const me = v.me;
  const skill = $("sh-skill");
  skill.textContent = me.skill ? `${me.skill}${me.skillLeft > 1 ? ` (${me.skillLeft})` : ""}` : "Use position";
  show(skill, !!me.roleName, can.skill);
  show($("sh-power"), true, can.power);
  show($("sh-clean"), true, can.clean, `Clean record (${v.costs.clean})`);
  show($("sh-evidence"), me.compromised, can.evidence);
  show($("sh-seize"), (me.pawns || []).length > 0, can.seize);
  show($("sh-accuse"), me.antiCoup, can.accuse);
  show($("sh-pass"), true, can.pass);

  show($("sh-offer"), true, can.offer);
  show($("sh-news"), true, can.news, `Leak to the feed (${v.costs.news})`);
  show($("sh-leak"), me.compromised, can.leak);
  show($("sh-pardon"), me.antiCoup, can.pardon, `Issue a pardon (${v.costs.pardon})`);

  $("hint").textContent = hint(v);
}

// show hides a button that will never be relevant to this player, and merely
// disables one that is not relevant right now. The difference matters: a greyed
// "Name the Mastermind" on every screen would tell the table how many people are
// even able to press it.
function show(btn, relevant, enabled, label) {
  btn.hidden = !relevant;
  btn.disabled = !enabled;
  if (label) btn.textContent = label;
}

function hint(v) {
  if (v.phase === "game_over") return "";
  const me = v.me;
  if (!me.roleName) return "Watching.";
  if (me.myTurn && !me.acted) return "One action, then the turn moves on.";
  if (me.compromised && !me.masterName) return "Somebody owns you. Dig, or find a friend.";
  if (me.myTurn && me.acted) return "Deals and leaks still work while you wait.";
  return "";
}

function renderOffers(v) {
  const box = $("sh-offers");
  const offers = v.me.offers || [];
  box.hidden = offers.length === 0;
  box.replaceChildren(...offers.map((o) => offerEl(o, v)));
}

function offerEl(o, v) {
  const wrap = document.createElement("div");
  wrap.className = "sh-offer";
  const other = o.mine ? o.toId : o.fromId;
  wrap.append(el("div", "sh-offer-head",
    o.mine ? `Your proposal to ${nameOf(other)}` : `${nameOf(other)} proposes`));

  const terms = [];
  if (o.give?.length) terms.push(`${o.mine ? "you give" : "you get"}: ${o.give.map((c) => c.name).join(", ")}`);
  if (o.want?.length) terms.push(`${o.mine ? "you get" : "you give"}: ${o.want.map((c) => c.name).join(", ")}`);
  if (o.pay) terms.push(`${o.mine ? "you pay" : "you get"} ${o.pay} influence`);
  if (o.demand) terms.push(`${o.mine ? "you get" : "you pay"} ${o.demand} influence`);
  wrap.append(el("p", "sh-offer-terms", terms.join(" · ") || "nothing at all"));
  if (o.note) wrap.append(el("p", "sh-offer-note", `“${o.note}”`));
  if (o.unfunded) wrap.append(el("p", "sh-offer-terms", "You cannot cover that."));

  const actions = document.createElement("div");
  actions.className = "sh-offer-actions";
  if (!o.mine) {
    actions.append(btn("Accept", "primary", () => send({ type: "accept", offerId: o.id }),
      o.unfunded));
    actions.append(btn("Counter", "", () => composeOffer(o.fromId)));
    actions.append(btn("Decline", "ghost", () => send({ type: "decline", offerId: o.id })));
  } else {
    actions.append(btn("Withdraw", "ghost", () => send({ type: "decline", offerId: o.id })));
  }
  wrap.append(actions);
  return wrap;
}

// The rules panel's table of which card answers which secret. Built from the
// catalogues the server sends, so the panel cannot drift out of step with the
// deck. Rebuilt only when the row count changes, which is never mid-game.
function renderRulesCards(v) {
  const box = $("sh-rules-cards");
  if (!box || box.childElementCount === (v.powers || []).length) return;
  box.replaceChildren(...(v.powers || []).map((p) => {
    const row = document.createElement("div");
    row.className = "sh-rules-row";
    row.append(el("b", "", p.name));
    row.append(el("span", "", p.wild ? "any secret at all" : secretName(v, p.exploits)));
    return row;
  }));
}

function secretName(v, slug) {
  const s = (v.secrets || []).find((c) => c.slug === slug);
  return s ? s.name : slug;
}

// ────────────────────────────────────────────────── prompts

// chooseSeat is every "who?" in the game. candidates is a filter over the seat
// list, so the caller says who is eligible and this says how it looks.
function chooseSeat(kind, { title, body, ok = "", candidates, onPick }) {
  const v = latest();
  const seats = v.seats.filter((s) => s.id !== v.me.id).filter(candidates || (() => true));
  if (!seats.length) {
    ctx.toast("There's nobody to aim that at.");
    return;
  }
  const rows = seats.map((s) => {
    const label = [s.name, s.roleName, s.knownSecret || s.knownCategory || null]
      .filter(Boolean).join(" · ");
    return btn(label, "", () => { closeModal(); onPick(s); });
  });
  openModal(kind, { title, body, extra: rows, alt: ok || "Cancel" });
}

// chooseCard is every "which card?". cards is a list of card objects.
function chooseCard(kind, { title, body, cards, onPick }) {
  if (!cards.length) {
    ctx.toast("You have nothing to play.");
    return;
  }
  const rows = cards.map((c) => {
    const b = cardEl(c, { button: true });
    b.onclick = () => { closeModal(); onPick(c); };
    return b;
  });
  openModal(kind, { title, body, extra: rows, alt: "Cancel" });
}

function useSkill() {
  const v = latest();
  const role = v.me.role;
  if (role === "lawyer") {
    const select = document.createElement("select");
    for (const c of v.secrets || []) {
      const opt = document.createElement("option");
      opt.value = c.slug;
      opt.textContent = `${c.name} — ${c.category}`;
      select.append(opt);
    }
    const form = document.createElement("div");
    form.className = "sh-form";
    form.append(el("label", "", "Which secret do you want counted?"), select);
    const { ok } = openModal("sh-skill", {
      title: "Go through the record books",
      body: "You will be told how many people at this table are carrying it. Not who.",
      extra: [form], ok: "Inspect", alt: "Cancel",
    });
    ok.onclick = () => { send({ type: "skill", slug: select.value }); closeModal(); };
    return;
  }
  if (role === "citizen") {
    const { ok } = openModal("sh-skill", {
      title: "Listen to the corridors",
      body: "You will hear one person's exact secret. Which person is not up to you.",
      ok: "Listen", alt: "Cancel",
    });
    ok.onclick = () => { send({ type: "skill" }); closeModal(); };
    return;
  }
  const copy = {
    police: {
      title: "Open a file",
      body: "You learn their exact secret. The table is told you opened a file on them, and nothing else.",
    },
    hacker: {
      title: "Get inside their machines",
      body: "You learn what kind of secret it is. You get two of these a round.",
    },
    president: {
      title: "Issue a decree",
      body: "Everybody learns what kind of secret they are keeping — including them. You take one influence for it.",
    },
  }[role] || { title: "Use your position", body: "" };
  chooseSeat("sh-skill", {
    ...copy,
    onPick: (s) => send({ type: "skill", targetId: s.id }),
  });
}

function choosePowerCard() {
  const v = latest();
  chooseCard("sh-power", {
    title: "Play a Power card",
    body: "It lands only if it matches the secret they are keeping. If it misses, it is gone.",
    cards: v.me.hand || [],
    onPick: choosePowerTarget,
  });
}

function choosePowerTarget(card) {
  chooseSeat("sh-power-target", {
    title: `${card.name} — at whom?`,
    body: card.wild
      ? "Kompromat lands on anybody. Spend it well."
      : `This one only lands on ${secretName(latest(), card.exploits)}. Everybody will see that you moved against them; only the two of you will learn whether it worked.`,
    onPick: (s) => send({ type: "power", cardId: card.id, targetId: s.id }),
  });
}

function confirmClean() {
  const v = latest();
  const { ok } = openModal("sh-clean", {
    title: "Buy a clean record",
    body: `Your secret is replaced with one nobody has been looking for. Costs ${v.costs.clean} influence, and everybody's notes about you quietly go stale.`,
    ok: "Pay up", alt: "Cancel",
  });
  ok.onclick = () => { send({ type: "clean" }); closeModal(); };
}

function confirmEvidence() {
  const v = latest();
  const me = v.me;
  const left = v.costs.evidenceToFree - me.evidence;
  const { ok } = openModal("sh-evidence", {
    title: "Dig",
    body: me.masterName
      ? `You know ${me.masterName} owns you. ${left} more and you walk out in a mutiny.`
      : `At ${v.costs.evidenceToName} you learn who owns you. At ${v.costs.evidenceToFree} you are out. You have ${me.evidence}.`,
    ok: "Dig", alt: "Cancel",
  });
  ok.onclick = () => { send({ type: "evidence" }); closeModal(); };
}

function chooseSeizeTarget() {
  const v = latest();
  const pawns = v.me.pawns || [];
  if (pawns.length === 1) return chooseSeizeCard(pawns[0]);
  chooseSeat("sh-seize", {
    title: "Take a card — from which of yours?",
    body: "They are not asked.",
    candidates: (s) => s.pawn,
    onPick: (s) => chooseSeizeCard(pawns.find((p) => p.id === s.id)),
  });
}

function chooseSeizeCard(pawn) {
  if (!pawn || !pawn.hand.length) {
    ctx.toast("They have nothing left to take.");
    return;
  }
  chooseCard("sh-seize-card", {
    title: `Take from ${pawn.name}`,
    body: "You can see their hand because you own them.",
    cards: pawn.hand,
    onPick: (c) => send({ type: "seize", targetId: pawn.id, cardId: c.id }),
  });
}

function chooseAccused() {
  chooseSeat("sh-accuse", {
    title: "Name the Mastermind",
    body: "One shot. Right, and the coup is over. Wrong, and they get three influence and the room learns who you are.",
    onPick: (s) => send({ type: "accuse", targetId: s.id }),
  });
}

function choosePardon() {
  const v = latest();
  chooseSeat("sh-pardon", {
    title: "Issue a pardon",
    body: `Costs ${v.costs.pardon} influence and frees them from whoever holds their secret — and protects them for the rest of the round. If they were nobody's Pawn, you will be told and it costs you nothing.`,
    onPick: (s) => send({ type: "pardon", targetId: s.id }),
  });
}

function composeNews() {
  const v = latest();
  const form = document.createElement("div");
  form.className = "sh-form";
  const box = document.createElement("textarea");
  box.maxLength = 180;
  box.placeholder = "The Chief of Police has offshore accounts…";
  form.append(el("label", "", "What does the room find out?"), box);

  const { ok } = openModal("sh-news", {
    title: "Leak to the feed",
    body: `Anonymous, and nobody checks it. Costs ${v.costs.news} influence.`,
    extra: [form], ok: "Post it", alt: "Cancel",
  });
  ok.onclick = () => {
    const text = box.value.trim();
    if (!text) { ctx.toast("Write something first."); return; }
    send({ type: "news", text });
    closeModal();
  };
  requestAnimationFrame(() => box.focus());
}

// The proposal composer. `toId` preselects the other side, which is how a
// counter-offer works: the same form, aimed back at whoever just proposed.
function composeOffer(toId) {
  const v = latest();
  const others = v.seats.filter((s) => s.id !== v.me.id);
  if (!others.length) return;

  const form = document.createElement("div");
  form.className = "sh-form";

  const who = document.createElement("select");
  for (const s of others) {
    const opt = document.createElement("option");
    opt.value = s.id;
    opt.textContent = `${s.name} — ${s.roleName}`;
    if (s.id === toId) opt.selected = true;
    who.append(opt);
  }
  form.append(el("label", "", "With whom?"), who);

  const give = pickList(v.me.hand || [], "You have nothing to offer.");
  form.append(el("label", "", "Cards you hand over"), give.el);

  // You can only name a card you can actually see, which is your own hand and
  // the hands of your own Pawns. Everything else is negotiated in the note and
  // settled by whoever can see what they are promising.
  const wantSource = () => {
    const pawn = (v.me.pawns || []).find((p) => p.id === who.value);
    return pawn ? pawn.hand : [];
  };
  const want = pickList(wantSource(), "");
  const wantLabel = el("label", "", "Cards you ask for");
  form.append(wantLabel, want.el);
  const syncWant = () => {
    const cards = wantSource();
    want.rebuild(cards);
    const visible = cards.length > 0;
    wantLabel.hidden = !visible;
    want.el.hidden = !visible;
  };
  who.onchange = syncWant;
  syncWant();

  const pay = numberRow("Influence you pay", 0, v.me.influence);
  const demand = numberRow("Influence you demand", 0, 99);
  form.append(pay.el, demand.el);

  const note = document.createElement("input");
  note.type = "text";
  note.maxLength = 120;
  note.placeholder = "Say what this is really about.";
  form.append(el("label", "", "Note"), note);

  const { ok } = openModal("sh-offer", {
    title: "Propose a deal",
    body: "Hand cards over, promise influence, demand it, or all three. Nothing here is binding beyond what it moves the moment they accept.",
    extra: [form], ok: "Send it", alt: "Cancel",
  });
  ok.onclick = () => {
    const msg = {
      type: "offer",
      targetId: who.value,
      cardIds: give.selected(),
      wantIds: want.selected(),
      amount: pay.value(),
      demand: demand.value(),
      text: note.value.trim(),
    };
    if (!msg.cardIds.length && !msg.wantIds.length && !msg.amount && !msg.demand) {
      ctx.toast("A proposal has to put something on the table.");
      return;
    }
    send(msg);
    closeModal();
  };
}

// pickList is a set of checkboxes over cards, returning the ids that are ticked.
function pickList(cards, emptyText) {
  const box = document.createElement("div");
  box.className = "sh-pick";
  const boxes = [];
  const build = (list) => {
    boxes.length = 0;
    box.replaceChildren();
    if (!list.length) {
      if (emptyText) box.append(el("p", "sh-empty", emptyText));
      return;
    }
    for (const c of list) {
      const label = document.createElement("label");
      label.className = "sh-check";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = String(c.id);
      label.append(input, document.createTextNode(`${c.name} — ${c.wild ? "any secret" : c.exploits || ""}`));
      box.append(label);
      boxes.push(input);
    }
  };
  build(cards);
  return {
    el: box,
    rebuild: build,
    selected: () => boxes.filter((b) => b.checked).map((b) => Number(b.value)),
  };
}

function numberRow(label, min, max) {
  const row = document.createElement("div");
  row.className = "sh-inline";
  const input = document.createElement("input");
  input.type = "number";
  input.min = String(min);
  input.max = String(max);
  input.value = "0";
  row.append(el("label", "", label), input);
  return { el: row, value: () => Math.max(0, Number(input.value) || 0) };
}

// ────────────────────────────────────────────────── the log

// One line per public event. Everything private is handled by handlePrivate()
// below and written straight into the log as a note only this player sees.
function logLine(e) {
  const li = document.createElement("li");
  const who = e.actorId ? nameOf(e.actorId) : "";
  const whom = e.targetId ? nameOf(e.targetId) : "";
  let text;
  let big = false;

  switch (e.kind) {
    case "started":  text = `${e.count} at the table. ${e.text}`; big = true; break;
    case "round":    text = `Round ${e.count} — ${e.text}`; big = true; break;
    case "catalyst": text = e.text; big = true; break;
    case "turn":     return null; // the banner is the whole story
    case "skill":    text = whom ? `${who} ${e.text} ${whom}` : `${who} ${e.text}`; break;
    case "disclosed":
      text = `By decree: ${whom}'s secret is ${e.text} in nature`;
      big = true;
      break;
    // The most important line in the game, and deliberately the least
    // informative: whether it landed is between the two of them.
    case "move":     text = `${who} moved against ${whom}`; break;
    case "traded":
      text = `${who} and ${whom} closed a deal` +
        (e.points ? ` — ${e.points} influence changed hands` : "");
      break;
    case "news":     text = `📰 ${e.text}`; big = true; break;
    case "clean":    text = `${who} bought a clean record for ${e.points}`; break;
    case "mutiny":   text = `${who} ${e.text}`; big = true; break;
    case "accused":
      text = e.text === "wrong"
        ? "That accusation was wrong"
        : `${who} accused ${whom}`;
      big = true;
      break;
    case "passed":   text = `${who} let the turn go by`; break;
    case "game_over": text = `🏁 ${e.text}`; big = true; break;
    case "revealed": return null; // the game-over card is the reveal
    case "auto":     text = `${who} was away — the table passed for them`; break;
    default:         return null; // private kinds arrive through onPrivate
  }

  li.textContent = text;
  if (big) li.classList.add("big");
  return li;
}

// Private entries reach one player only. Almost everything that actually happens
// in this game arrives here rather than in the shared log, so each one is both
// written into the log and — when it changes what the player should do next —
// put in front of them as a toast.
function handlePrivate(e) {
  const text = e.text || "";
  if (text) logPrivate(text);

  switch (e.kind) {
    case "owned":
      ctx.toast(text);
      break;
    case "pardon":
    case "seized":
    case "mutiny":
      ctx.toast(text);
      break;
    case "offer":
      // Your own copy is a receipt; theirs is a decision, and the strip above the
      // hand is already showing it.
      if (e.targetId && e.targetId !== ctx.me()) break;
      ctx.toast(`${nameOf(e.actorId)} has proposed something.`);
      break;
    case "leak":
      ctx.toast(text);
      break;
    case "declined":
      ctx.toast(text);
      break;
    default:
      break;
  }
}

// ────────────────────────────────────────────────── odds and ends

function renderGameOver(v) {
  if (v.phase !== "game_over") {
    if (modalKind() === "sh-over") closeModal();
    return;
  }
  if (modalKind() === "sh-over") return;

  const won = (v.winnerIds || []).includes(ctx.me());
  const titles = {
    mastermind: "The coup succeeded",
    resistance: "The coup was broken",
    state: "The state held",
  };
  const outcome = v.reason || "";
  const { ok, alt } = openModal("sh-over", {
    title: (won ? "🏆 " : "") + (titles[v.winner] || "It's over"),
    body: v.me.host
      ? outcome
      : `${outcome} Waiting for ${nameOf(hostId(v))} to deal again.`,
    ok: v.me.host ? "Deal again" : "Close",
    alt: v.me.host ? "Back to lobby" : "",
  });
  ok.onclick = () => {
    if (v.me.host) send({ type: "start" });
    closeModal();
  };
  // The lobby is the only way to pick up players who arrived mid-game: dealing
  // again seats whoever is present right now and drops everybody else.
  alt.onclick = () => {
    send({ type: "lobby" });
    closeModal();
  };
}

function hostId(v) {
  const h = v.seats.find((s) => s.host);
  return h ? h.id : "";
}

// cardEl draws one card as a slip of paper: its name, what it answers, and its
// flavour line. A Power card names the secret it destroys because that mapping
// is public — hiding it would only mean everybody had to learn it by wasting
// cards.
function cardEl(card, { button = false, dim = false, className = "" } = {}) {
  const node = document.createElement(button ? "button" : "div");
  if (button) node.type = "button";
  node.className = "sh-card" + (card.wild ? " wild" : "") + (dim ? " dim" : "") +
    (className ? " " + className : "");
  node.append(el("span", "sh-card-name", card.name));
  node.append(el("span", "sh-card-sub", cardSub(card)));
  if (card.blurb) node.append(el("span", "sh-card-blurb", card.blurb));
  return node;
}

function cardSub(card) {
  if (card.kind === "weakness") return `Your secret · ${card.category}`;
  if (card.wild) return "Lands on any secret";
  if (card.kind === "power") return `Answers ${secretName(latest() || {}, card.exploits)}`;
  return card.kind;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function btn(label, variant, onclick, disabled = false) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "btn" + (variant ? " " + variant : "") + " small";
  b.textContent = label;
  b.disabled = disabled;
  b.onclick = onclick;
  return b;
}

function showRules(on) {
  $("rules-panel").hidden = !on;
}

// ────────────────────────────────────────────────── rules language
//
// Burmese is offered for the same reason Exploding Kittens offers it: this is a
// party game played out loud, and the person reading the rules aloud is not
// always the person who set the server up. Only the how-to-play sheet is
// translated — everybody's own screen, buttons included, stays in English, which
// is why the sheet keeps Mastermind, Resistance, Coup, Pawn and the rest in
// English too: that is what a player reads a moment later on their own table.
const rulesLang = () => (localStorage.getItem("sh:lang") === "my" ? "my" : "en");

function setRulesLang(lang) {
  localStorage.setItem("sh:lang", lang);
  for (const s of document.querySelectorAll(".rules-lang")) {
    s.hidden = s.dataset.lang !== lang;
  }
  for (const [id, on] of [["lang-en", lang === "en"], ["lang-my", lang === "my"]]) {
    $(id).classList.toggle("on", on);
    $(id).setAttribute("aria-checked", String(on));
  }
  document.querySelector(".rules-title").textContent =
    lang === "my" ? "ကစားနည်း" : "How to play";
  $("rules-close").textContent = lang === "my" ? "ပိတ်" : "Close";
}

function setLogOpen(open) {
  setStoredLogOpen(open);
  const panel = $("log-panel");
  panel.classList.toggle("collapsed", !open);
  $("log-collapse").setAttribute("aria-expanded", String(open));
  $("log-caret").textContent = open ? "▾" : "▸";
  $("log").hidden = !open;
  if (open) scrollLogToEnd();
}
