// Command server hosts the Exploding Kittens table. Everything — rules, rooms
// and the browser client — ships inside this one binary.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"boardgame/kittens/internal/analytics"
	"boardgame/kittens/internal/room"
	"boardgame/kittens/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	trustProxy := flag.Bool("trust-proxy", false,
		"take the player's address from X-Forwarded-For (only behind a reverse proxy)")
	flag.Parse()

	an := openAnalytics(*trustProxy)
	defer an.close()

	mgr := room.NewManager()
	defer mgr.Shutdown()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(mgr, an.Analytics),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: it would guillotine long-lived WebSocket connections.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	printAddresses(*addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("listen: %v", err)
	}
	log.Println("bye")
}

type analyticsSetup struct {
	server.Analytics
}

// openAnalytics switches recording on. With ODOO_URL set, rooms, joins and
// suggestions are sent to the kittens_report Odoo module and nothing is kept
// here; otherwise, with DATABASE_URL set, they go to Postgres and Odoo can read
// them through /api/export. Analytics is a side concern: if neither is set, or
// the database is down, the tables still play and only the report goes without.
//
//	ODOO_URL      https://odoo.example.com (the Odoo with kittens_report installed)
//	DATABASE_URL  postgres://user:pass@localhost:5432/board-game-room
//	GEOIP_DB      path to a GeoLite2-Country or DB-IP Country Lite .mmdb file
//	REPORT_TOKEN  secret shared with the Odoo module (Settings > Board Games)
func openAnalytics(trustProxy bool) analyticsSetup {
	setup := analyticsSetup{Analytics: server.Analytics{
		Token:      os.Getenv("REPORT_TOKEN"),
		TrustProxy: trustProxy,
	}}
	odooURL := os.Getenv("ODOO_URL")
	dbURL := os.Getenv("DATABASE_URL")
	if odooURL == "" && dbURL == "" {
		log.Printf("analytics off: neither ODOO_URL nor DATABASE_URL is set")
		return setup
	}
	if setup.Token == "" {
		log.Printf("analytics: REPORT_TOKEN is not set, Odoo will refuse everything")
	}

	var sink analytics.Sink
	if odooURL != "" {
		odoo := analytics.NewOdoo(odooURL, setup.Token)
		sink = odoo
		setup.Suggestions = odoo
		log.Printf("analytics on: sending to Odoo at %s", odooURL)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		store, err := analytics.Open(ctx, dbURL)
		if err != nil {
			log.Printf("analytics off: %v", err)
			return setup
		}
		sink = store
		setup.Store = store
		setup.Suggestions = store
		log.Printf("analytics on: saving to Postgres")
	}

	geo, err := analytics.OpenLocator(os.Getenv("GEOIP_DB"))
	if err != nil {
		log.Printf("analytics: geolocation off: %v", err)
	} else if geo == nil {
		log.Printf("analytics: GEOIP_DB is not set, countries will show as Unknown")
	}
	setup.Recorder = analytics.NewRecorder(sink, geo)
	setup.Geo = geo
	return setup
}

// close flushes queued events before the pool they are written through goes.
func (a analyticsSetup) close() {
	a.Recorder.Close()
	a.Geo.Close()
	if a.Store != nil {
		a.Store.Close()
	}
}

// printAddresses lists the URLs guests on the same Wi-Fi should open, since the
// entire point is other people joining from their own phones.
func printAddresses(addr string) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "8080"
	}
	log.Printf("Exploding Kittens ready")
	log.Printf("  http://localhost:%s", port)

	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			log.Printf("  http://%s:%s  (%s)", ipnet.IP, port, iface.Name)
		}
	}
}
