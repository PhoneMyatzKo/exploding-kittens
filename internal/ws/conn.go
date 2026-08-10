// Package ws adapts browser WebSocket connections onto a room's command channel.
// It contains no game logic: read pumps only forward messages, and write pumps
// only forward bytes the room produced.
package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"boardgame/kittens/internal/room"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second // must stay comfortably below pongWait
	maxMessage = 4 << 10
	sendBuffer = 32
)

// Client is one browser socket. Send is safe to call from the room goroutine;
// the actual write happens on this client's own writer goroutine.
type Client struct {
	conn   *websocket.Conn
	out    chan []byte
	closed chan struct{}
}

func newClient(conn *websocket.Conn) *Client {
	return &Client{conn: conn, out: make(chan []byte, sendBuffer), closed: make(chan struct{})}
}

// Send queues a payload. A client that has stopped draining (a wedged tab) is
// disconnected rather than allowed to block the room goroutine.
func (c *Client) Send(b []byte) {
	select {
	case c.out <- b:
	case <-c.closed:
	default:
		c.Close()
	}
}

// Close is idempotent.
func (c *Client) Close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case b := <-c.out:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.closed:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

func (c *Client) readPump(r *room.Room, playerID string) {
	defer func() {
		r.Leave(playerID, c)
		c.Close()
	}()

	c.conn.SetReadLimit(maxMessage)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg room.ClientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue // ignore junk rather than dropping the player
		}
		r.Submit(playerID, msg)
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Same-origin is not enforced: this is meant to be opened from phones on the
	// host's LAN by IP, where Origin is unpredictable, and a room code is the
	// only thing worth protecting anyway.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Handler serves GET /ws?code=ABCD&name=Alice&token=…
//
// The token is the browser's claim on a seat; when it matches an existing member
// the player reconnects into their old hand instead of joining as someone new.
func Handler(mgr *room.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		rm, err := mgr.Get(q.Get("code"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return // Upgrade already wrote a response
		}
		c := newClient(conn)
		go c.writePump()

		playerID, token, err := rm.Join(q.Get("token"), q.Get("name"), c)
		if err != nil {
			b, _ := json.Marshal(map[string]string{"type": "fatal", "message": err.Error()})
			c.Send(b)
			// Give the writer a moment to flush the reason before hanging up.
			time.Sleep(200 * time.Millisecond)
			c.Close()
			return
		}

		hello, _ := json.Marshal(map[string]string{
			"type": "joined", "code": rm.Code, "playerId": playerID, "token": token,
		})
		c.Send(hello)

		c.readPump(rm, playerID)
		log.Printf("room %s: %s disconnected", rm.Code, playerID)
	}
}
