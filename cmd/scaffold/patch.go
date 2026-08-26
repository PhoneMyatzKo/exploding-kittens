package main

import (
	"fmt"
	"go/format"
	"os"
	"strings"
)

// patchCatalogue inserts a new game's tile into internal/games/catalogue.go,
// marked Playable: false — the registry switch in internal/games/registry.go
// is left for whoever implements the rules to wire up by hand (see run()'s
// printed instructions), since TestEveryPlayableGameHasRules requires that an
// unimplemented slug have no case there.
//
// It anchors on an exact substring of the file as it stands today rather than
// parsing Go, so a hand-edit to catalogue.go since the last game was added can
// make the anchor go missing. That is deliberate: run() treats a missing
// anchor as a hard error with the file path in it, never as "insert somewhere
// plausible" — a scaffold tool that silently mis-patches generated code is
// worse than one that stops and asks.

const catalogueConstAnchor = `UNO              = "uno"
)`

const catalogueEntryAnchor = `}

// All returns the catalogue.`

func patchCatalogue(g game) error {
	path := "internal/games/catalogue.go"
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(src)

	constLine := fmt.Sprintf("UNO              = \"uno\"\n\t%s = %q\n)", g.ConstName, g.Slug)
	if !strings.Contains(content, catalogueConstAnchor) {
		return fmt.Errorf("could not find the slug const block to extend")
	}
	content = strings.Replace(content, catalogueConstAnchor, constLine, 1)

	entry := fmt.Sprintf(`	{
		Slug: %s, Name: %q, Tagline: %q,
		Emoji: %q, Min: %d, Max: %d, Playable: false,
	},
}

// All returns the catalogue.`, g.ConstName, g.Name, g.Tagline, g.Emoji, g.Min, g.Max)
	if !strings.Contains(content, catalogueEntryAnchor) {
		return fmt.Errorf("could not find the end of the catalogue slice to extend")
	}
	content = strings.Replace(content, catalogueEntryAnchor, entry, 1)

	return writeFormatted(path, content)
}

func writeFormatted(path, content string) error {
	out, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("formatting patched file: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}
