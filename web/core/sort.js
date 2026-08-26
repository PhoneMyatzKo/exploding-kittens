// groupByCategory arranges a hand so cards sharing a category sit next to each
// other, without inventing an order of its own: each group surfaces wherever
// its first card already was, and cards within a group keep their original
// relative order. A freshly drawn card of a category already in hand slots in
// right after its group rather than at the far end, because it ties with that
// group's cards on sort key and a stable sort leaves ties in input order.
//
// Display-only — the server's own order is the deal, and nothing here is sent
// back to it.
export function groupByCategory(items, categoryOf) {
  const firstSeenAt = new Map();
  items.forEach((item, i) => {
    const category = categoryOf(item);
    if (!firstSeenAt.has(category)) firstSeenAt.set(category, i);
  });
  return [...items].sort(
    (a, b) => firstSeenAt.get(categoryOf(a)) - firstSeenAt.get(categoryOf(b))
  );
}
