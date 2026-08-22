package server

import (
	"net/http"
	"strconv"

	"github.com/coder/websocket"
)

// handleWebSocket upgrades the connection and streams every broadcast
// event for the tournament in the URL until the client disconnects.
// Auth is manual (not through requireRole) because browsers cannot set
// an Authorization header on a WebSocket handshake -- the token travels
// as a query parameter instead, validated the same way session tokens
// are validated everywhere else in this API.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	if _, err := s.sessions.Find(token); err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()

	// CloseRead spawns a goroutine that discards incoming reads (this
	// connection is server-push only) and handles control frames; its
	// returned context is canceled the moment the client disconnects.
	ctx := c.CloseRead(r.Context())

	ch := s.hub.register(tournamentID)
	defer s.hub.unregister(tournamentID, ch)

	for {
		select {
		case msg := <-ch:
			if err := c.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
