package analytics

import (
	"net"
	"net/http"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// Country is where a player connected from. Code is the ISO 3166-1 alpha-2
// code, which is also what Odoo's res.country is keyed on, so the report can be
// matched to Odoo countries without translating names.
type Country struct {
	Code string
	Name string
}

var (
	countryLAN     = Country{Code: "LAN", Name: "Local network"}
	countryUnknown = Country{Code: "??", Name: "Unknown"}
)

// Locator turns an IP address into a country using a local .mmdb file (MaxMind
// GeoLite2-Country or DB-IP Country Lite), so nothing is sent to a third party
// per player. A nil Locator still answers: LAN for private addresses, Unknown
// for the rest.
type Locator struct {
	db *geoip2.Reader
}

// OpenLocator opens the database at path. An empty path gives a nil Locator.
func OpenLocator(path string) (*Locator, error) {
	if path == "" {
		return nil, nil
	}
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &Locator{db: db}, nil
}

func (l *Locator) Close() {
	if l != nil {
		_ = l.db.Close()
	}
}

func (l *Locator) Country(ip net.IP) Country {
	if ip == nil {
		return countryUnknown
	}
	// Most tables are friends on the same Wi-Fi; no database knows where
	// 192.168.x.x is, and saying so is more useful than Unknown.
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return countryLAN
	}
	if l == nil {
		return countryUnknown
	}
	rec, err := l.db.Country(ip)
	if err != nil || rec.Country.IsoCode == "" {
		return countryUnknown
	}
	name := rec.Country.Names["en"]
	if name == "" {
		name = rec.Country.IsoCode
	}
	return Country{Code: rec.Country.IsoCode, Name: name}
}

// ClientIP is the address a request came from. X-Forwarded-For is only
// believed when trustProxy is set: without a proxy in front, any browser could
// write whatever country it liked into that header.
func ClientIP(req *http.Request, trustProxy bool) net.IP {
	if trustProxy {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	return net.ParseIP(host)
}
