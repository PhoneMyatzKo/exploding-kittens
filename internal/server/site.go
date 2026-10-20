package server

import (
	"net/http"
	"net/url"
)

// Shop is the owner's online store, advertised on the menu. The games stay
// free; the shop is how the site pays for itself. The zero value shows no
// shop at all.
type Shop struct {
	Name string
	// URL is where the banner and the top-bar button lead. Only http and https
	// are accepted, so a setting can never turn the link into a script.
	URL string
}

// siteHandler serves GET /api/site, the bits of the page that are settings of
// this server rather than part of any game.
func siteHandler(shop Shop) http.HandlerFunc {
	type shopJSON struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	var out struct {
		Shop *shopJSON `json:"shop"`
	}
	if u, err := url.Parse(shop.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		name := shop.Name
		if name == "" {
			name = "Our Shop"
		}
		out.Shop = &shopJSON{Name: name, URL: u.String()}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, out)
	}
}
