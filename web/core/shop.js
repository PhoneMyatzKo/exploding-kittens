// The owner's shop, on the menu: a button in the top bar and a banner under the
// games.
//
// The games are free; the shop is how the site pays for itself. Where it lives
// and what it is called are server settings (SHOP_URL, SHOP_NAME) read from
// /api/site, so moving the shop never needs a client change. With no shop set,
// or if the fetch fails, both stay hidden and the menu looks as it always did.

import { $ } from "./dom.js";

// Tagged so the shop's Odoo can tell visitors from the game site apart, and
// which of the two links they used.
function tracked(url, medium) {
  const u = new URL(url);
  u.searchParams.set("utm_source", "boardgameroom");
  u.searchParams.set("utm_medium", medium);
  u.searchParams.set("utm_campaign", "game_menu");
  return u.toString();
}

export async function mountShop() {
  let shop;
  try {
    const res = await fetch("/api/site");
    if (!res.ok) return;
    shop = (await res.json()).shop;
  } catch {
    return;
  }
  if (!shop) return;

  const top = $("shop-top");
  top.href = tracked(shop.url, "topbar");
  $("shop-top-name").textContent = shop.name;
  top.hidden = false;

  $("shop-banner-link").href = tracked(shop.url, "banner");
  $("shop-banner-name").textContent = shop.name;
  $("shop-banner").hidden = false;
}
