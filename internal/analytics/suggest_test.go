package analytics

import (
	"errors"
	"strings"
	"testing"
)

func TestSuggestionClean(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      SuggestionInput
		wantErr bool
		gamble  bool
	}{
		{"card game", SuggestionInput{GameName: " Sushi Go ", Category: "card"}, false, false},
		{"family game", SuggestionInput{GameName: "Ludo", Category: "family", Description: "for my kids"}, false, false},
		{"no name", SuggestionInput{GameName: "  ", Category: "card"}, true, false},
		{"bad category", SuggestionInput{GameName: "Chess", Category: "sport"}, true, false},
		{"name too long", SuggestionInput{GameName: strings.Repeat("a", maxGameName+1), Category: "card"}, true, false},
		{"poker", SuggestionInput{GameName: "Texas POKER", Category: "card"}, true, true},
		{"hold'em in text", SuggestionInput{GameName: "Cards", Category: "card", Description: "like Hold'em"}, true, true},
		{"blackjack", SuggestionInput{GameName: "Blackjack", Category: "card"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			err := in.Clean()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Clean() = %v, want error %v", err, tc.wantErr)
			}
			if errors.Is(err, ErrGambling) != tc.gamble {
				t.Fatalf("Clean() = %v, want gambling %v", err, tc.gamble)
			}
			if err == nil && in.GameName != strings.TrimSpace(tc.in.GameName) {
				t.Fatalf("name = %q, want it trimmed", in.GameName)
			}
		})
	}
}
