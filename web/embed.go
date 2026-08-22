// Package web carries the browser client, compiled into the server binary so
// that running the game is a single executable with no asset paths to get wrong.
package web

import "embed"

// Listed rather than swept with `all:.`, and that is not fussiness: web/testing
// sits in this directory too, and once its node_modules are installed a wildcard
// would compile a few thousand files of Playwright into the server.
//
// core/ is the shared client modules; games/ is one directory per game, holding
// the template and the module that mountGame() in app.js loads on demand.
//
//go:embed index.html app.js style.css core games
var Assets embed.FS
