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

	shop := shopFromEnv()

	an := openAnalytics(*trustProxy)
	defer an.close()

	mgr := room.NewManager()
	defer mgr.Shutdown()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(mgr, an.Analytics, shop),
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

// defaultShopURL is the owner's Odoo store. The games are free to play; the
// shop banner on the menu is how the site pays for itself.
const defaultShopURL = "https://shopping.boardgameroom.online"

// shopFromEnv reads the shop link shown on the menu.
//
//	SHOP_URL   where the link goes; set it empty to hide the shop
//	SHOP_NAME  the name on the banner and the button (default "Our Shop")
func shopFromEnv() server.Shop {
	shop := server.Shop{URL: defaultShopURL, Name: os.Getenv("SHOP_NAME")}
	if u, ok := os.LookupEnv("SHOP_URL"); ok {
		shop.URL = u
	}
	return shop
}

type analyticsSetup struct {
	server.Analytics
}

// openAnalytics switches recording on when DATABASE_URL is set. Analytics is a
// side concern: if the database is missing or down, the tables still play and
// only the report goes without.
//
//	DATABASE_URL  postgres://user:pass@localhost:5432/board-game-room
//	GEOIP_DB      path to a GeoLite2-Country or DB-IP Country Lite .mmdb file
//	REPORT_TOKEN  secret the Odoo module sends to read /api/report
func openAnalytics(trustProxy bool) analyticsSetup {
	setup := analyticsSetup{Analytics: server.Analytics{
		Token:      os.Getenv("REPORT_TOKEN"),
		TrustProxy: trustProxy,
	}}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Printf("analytics off: DATABASE_URL is not set")
		return setup
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := analytics.Open(ctx, url)
	if err != nil {
		log.Printf("analytics off: %v", err)
		return setup
	}
	geo, err := analytics.OpenLocator(os.Getenv("GEOIP_DB"))
	if err != nil {
		log.Printf("analytics: geolocation off: %v", err)
	} else if geo == nil {
		log.Printf("analytics: GEOIP_DB is not set, countries will show as Unknown")
	}
	if setup.Token == "" {
		log.Printf("analytics: REPORT_TOKEN is not set, /api/report will refuse everyone")
	}
	setup.Store = store
	setup.Recorder = store.NewRecorder(geo)
	setup.Geo = geo
	log.Printf("analytics on")
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
