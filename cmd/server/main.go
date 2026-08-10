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

	"boardgame/kittens/internal/room"
	"boardgame/kittens/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	mgr := room.NewManager()
	defer mgr.Shutdown()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(mgr),
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
