// The lookup the whole client is written on.
//
// Split out rather than repeated because the shell and every game module need it,
// and because having one place for it is what makes the rule visible: the shell
// looks up its own ids, a game looks up the ids in its own template, and neither
// reaches into the other's markup. A game's screens are shown and hidden through
// its module's render() and leaveTable(), never by id from outside.

export const $ = (id) => document.getElementById(id);

const scrollWatchers = new WeakMap();

// markScrollable flags a box that has more below the fold, so it can be drawn
// with a faded bottom edge. A row cut off mid-card is easy to read as the end of
// the list — or as a rendering fault — and these browsers paint no persistent
// scrollbar to say otherwise. That is exactly how a hand of eleven looked like a
// hand of six in the Favor picker, and the hand itself has the same shape.
//
// Shared because both the prompt's card list and the table's hand need it, and
// because a second copy of the observer bookkeeping is a leak waiting to happen.
export function markScrollable(el) {
  if (!el) return;
  const update = () => {
    el.classList.toggle("has-more", el.scrollHeight - el.scrollTop - el.clientHeight > 4);
  };
  el.onscroll = update;
  // Whether a box overflows depends on the window as much as on the contents:
  // turning a phone sideways, or dragging a desktop window shorter, changes the
  // answer with no scroll and no re-render to notice it.
  if (!scrollWatchers.has(el)) {
    const ro = new ResizeObserver(update);
    ro.observe(el);
    scrollWatchers.set(el, ro);
  }
  // The caller has only just filled it, so wait for layout before measuring.
  requestAnimationFrame(update);
}
