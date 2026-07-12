package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/coder/websocket"
)

// Hub fans out server→client messages to all connected WebSocket clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	last    map[api.WSMessageType][]byte // last message per type, replayed on connect
}

type wsClient struct {
	send chan []byte
}

func newHub() *Hub {
	return &Hub{
		clients: map[*wsClient]struct{}{},
		last:    map[api.WSMessageType][]byte{},
	}
}

// Broadcast marshals and sends a message to every connected client, dropping
// slow clients rather than blocking.
func (h *Hub) Broadcast(msg api.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	if msg.Type == api.WSTypeStacks || msg.Type == api.WSTypeMetrics {
		h.last[msg.Type] = data
	}
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
		}
	}
}

func (h *Hub) add(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	replay := make([][]byte, 0, len(h.last))
	for _, d := range h.last {
		replay = append(replay, d)
	}
	h.mu.Unlock()
	for _, d := range replay {
		select {
		case c.send <- d:
		default:
		}
	}
}

func (h *Hub) remove(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	client := &wsClient{send: make(chan []byte, 32)}
	s.hub.add(client)
	defer s.hub.remove(client)

	ctx := r.Context()

	// Reader: drain client messages / detect disconnect.
	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-client.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
