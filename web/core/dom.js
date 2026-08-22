// The lookup the whole client is written on.
//
// Split out rather than repeated because the shell and every game module need it,
// and because having one place for it is what makes the rule visible: the shell
// looks up its own ids, a game looks up the ids in its own template, and neither
// reaches into the other's markup. A game's screens are shown and hidden through
// its module's render() and leaveTable(), never by id from outside.

export const $ = (id) => document.getElementById(id);
