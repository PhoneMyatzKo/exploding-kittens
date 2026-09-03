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
	// Games in progress are written here on the way out and read back on the way
	// in, so a restart does not end everybody's game. Set it to "" to turn that
	// off. On by default because the games worth losing least — a Monopoly session
	// runs for hours — are exactly the ones nobody would think to protect until
	// after it happened.
	statePath := flag.String("state", "boardgame-state.json",
		`file to save games in progress to across a restart ("" to disable)`)
	// How much a hard kill is allowed to cost, against how often the file is
	// rewritten. Thirty seconds of a board game is a turn or two.
	stateEvery := flag.Duration("state-every", 30*time.Second,
		"how often to checkpoint games in progress")
	flag.Parse()

	mgr := room.NewManager()

	if n, err := mgr.LoadFrom(*statePath); err != nil {
		// Not fatal. A state file that cannot be read costs those games, and
		// refusing to start costs everybody the server as well.
		log.Printf("could not restore games from %s: %v", *statePath, err)
	} else if n > 0 {
		log.Printf("restored %d room(s) — everybody reconnects with the link they already have", n)
	}
	// Checkpointed as well as saved on the way out: a crash, a power cut, or on
	// Windows anything other than Ctrl+C in this console, all skip the graceful
	// path entirely. Thirty seconds is the most a hard kill can cost.
	mgr.StartAutosave(*statePath, *stateEvery)

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

	// Saved before Shutdown, because Shutdown closes every room and a closed room
	// has nothing left to describe. This is the whole reason the shutdown path is
	// ordered by hand rather than left to a defer.
	if n, err := mgr.SaveTo(*statePath); err != nil {
		log.Printf("could not save games to %s: %v", *statePath, err)
	} else if n > 0 {
		log.Printf("saved %d room(s) to %s", n, *statePath)
	}
	mgr.Shutdown()
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
