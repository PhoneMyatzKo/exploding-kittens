// Package web carries the browser client, compiled into the server binary so
// that running the game is a single executable with no asset paths to get wrong.
package web

import "embed"

//go:embed index.html app.js style.css
var Assets embed.FS
