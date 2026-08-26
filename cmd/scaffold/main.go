// Command scaffold writes the boilerplate for a new game module: the Go
// adapter and rules-engine stub under internal/games, the catalogue and
// registry entries that wire it in, and the client stub under web/games.
//
// It does not invent architecture — internal/games/kittens and
// internal/games/uno already define the shape a game module takes (see
// internal/core/core.go and app.js's mountGame() doc comment). This only
// automates producing that shape so a new game starts from a compiling,
// playable-toggled-off skeleton instead of a blank directory.
//
// Usage:
//
//	go run ./cmd/scaffold <slug> "<Display Name>" <emoji> "<tagline>" <min> <max>
//
// Example:
//
//	go run ./cmd/scaffold chess "Chess" ♞ "Sixty-four squares, one winner." 2 2
package main

import (
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scaffold:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 6 {
		return fmt.Errorf(`usage: go run ./cmd/scaffold <slug> "<Display Name>" <emoji> "<tagline>" <min> <max>`)
	}
	slug, name, emoji, tagline := args[0], args[1], args[2], args[3]
	min, err := strconv.Atoi(args[4])
	if err != nil {
		return fmt.Errorf("min players: %w", err)
	}
	max, err := strconv.Atoi(args[5])
	if err != nil {
		return fmt.Errorf("max players: %w", err)
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase letters, digits and hyphens, matching what app.js's mountGame() accepts", slug)
	}

	pkg := strings.ReplaceAll(slug, "-", "")
	if pkg == "" || pkg[0] < 'a' || pkg[0] > 'z' {
		return fmt.Errorf("slug %q does not yield a usable Go package name", slug)
	}
	constName := exportedName(slug)

	g := game{
		Slug: slug, Pkg: pkg, ConstName: constName,
		Name: name, Emoji: emoji, Tagline: tagline,
		Min: min, Max: max,
	}

	for _, dir := range []string{
		"internal/games/" + slug,
		"internal/games/" + slug + "/game",
		"web/games/" + slug,
	} {
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("%s already exists", dir)
		}
	}

	files := map[string]string{
		"internal/games/" + slug + "/game/state.go":   stateGoTmpl(g),
		"internal/games/" + slug + "/game/engine.go":  engineGoTmpl(g),
		"internal/games/" + slug + "/" + slug + ".go": adapterGoTmpl(g),
		"web/games/" + slug + "/table.html":           tableHTMLTmpl(g),
		"web/games/" + slug + "/index.js":             indexJSTmpl(g),
	}
	for path, content := range files {
		if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}

	if err := patchCatalogue(g); err != nil {
		return fmt.Errorf("internal/games/catalogue.go: %w (files above were written; add the entry by hand)", err)
	}

	fmt.Printf("scaffolded %q as announced-but-not-playable, like uno-no-mercy.\n\n", slug)
	fmt.Println("internal/games/registry.go is deliberately left untouched: TestEveryPlayableGameHasRules")
	fmt.Println("requires that a slug with no rules yet also has no case in New()'s switch. Once")
	fmt.Println("internal/games/" + slug + "/game is implemented, wire it in and flip the tile on:")
	fmt.Println()
	fmt.Printf("  1. internal/games/registry.go: import %q,\n", "boardgame/kittens/internal/games/"+slug)
	fmt.Printf("     add `case %s: return %s.New(rng), nil` before the switch's closing brace.\n", g.ConstName, g.Pkg)
	fmt.Printf("  2. internal/games/catalogue.go: set %q's Playable to true.\n", slug)
	fmt.Println()
	fmt.Println("next: go build ./... && go test ./...")
	return nil
}

func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

// exportedName turns a slug into the CamelCase identifier catalogue.go's
// consts use: "uno-no-mercy" -> "UnoNoMercy".
func exportedName(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

type game struct {
	Slug, Pkg, ConstName string
	Name, Emoji, Tagline string
	Min, Max             int
}

// gofmtOrPanic formats generated Go source. A template that fails to gofmt is
// a bug in this file, not something a caller can fix, so it panics rather
// than shipping unformatted or (if the syntax is actually broken) silently
// wrong Go.
func gofmtOrPanic(src string) string {
	out, err := format.Source([]byte(src))
	if err != nil {
		panic(fmt.Sprintf("scaffold: generated invalid Go: %v\n%s", err, src))
	}
	return string(out)
}
