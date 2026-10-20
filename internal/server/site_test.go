package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"boardgame/kittens/internal/room"
)

func TestSiteShop(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shop     Shop
		wantURL  string
		wantName string
	}{
		{"configured", Shop{Name: "Toys", URL: "https://shop.example.com"}, "https://shop.example.com", "Toys"},
		{"default name", Shop{URL: "https://shop.example.com/"}, "https://shop.example.com/", "Our Shop"},
		{"none", Shop{}, "", ""},
		{"script is refused", Shop{URL: "javascript:alert(1)"}, "", ""},
		{"no host", Shop{URL: "https://"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := room.NewManager()
			srv := httptest.NewServer(New(mgr, Analytics{}, tc.shop))
			t.Cleanup(func() { srv.Close(); mgr.Shutdown() })

			resp, err := http.Get(srv.URL + "/api/site")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var body struct {
				Shop *struct{ Name, URL string }
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if tc.wantURL == "" {
				if body.Shop != nil {
					t.Fatalf("shop = %+v, want none", body.Shop)
				}
				return
			}
			if body.Shop == nil || body.Shop.URL != tc.wantURL || body.Shop.Name != tc.wantName {
				t.Fatalf("shop = %+v, want %s at %s", body.Shop, tc.wantName, tc.wantURL)
			}
		})
	}
}
