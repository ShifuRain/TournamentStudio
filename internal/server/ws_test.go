package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"tournamentstudio/internal/auth"
)

// dialWS opens a WebSocket connection to the given tournament's live feed
// using httpServer's address, authenticating with token as the ?token=
// query parameter the way a real client must.
func dialWS(t *testing.T, httpServer *httptest.Server, tournamentID int64, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + httpServer.URL[len("http"):] + fmt.Sprintf("/api/tournaments/%d/ws?token=%s", tournamentID, token)
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func TestBroadcastReachesConnectedClientsOnlyForTheirTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)

	connA := dialWS(t, httpServer, tournamentA, token)
	connB := dialWS(t, httpServer, tournamentB, token)

	s.hub.broadcast(tournamentA, []byte(`{"type":"test"}`))

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := connA.Read(readCtx)
	if err != nil {
		t.Fatalf("connA should have received the broadcast: %v", err)
	}
	if string(msg) != `{"type":"test"}` {
		t.Fatalf("unexpected message: %s", msg)
	}

	shortCtx, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	if _, _, err := connB.Read(shortCtx); err == nil {
		t.Fatalf("connB should NOT have received tournamentA's broadcast")
	}
}

func TestWebSocketRejectsInvalidToken(t *testing.T) {
	s := newTestServer(t)
	_ = loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	tournamentID := int64(1)
	url := "ws" + httpServer.URL[len("http"):] + fmt.Sprintf("/api/tournaments/%d/ws?token=not-a-real-token", tournamentID)
	_, _, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("expected dial with an invalid token to fail")
	}
}

func TestWebSocketRejectsMissingToken(t *testing.T) {
	s := newTestServer(t)
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)

	url := "ws" + httpServer.URL[len("http"):] + "/api/tournaments/1/ws"
	_, _, err := websocket.Dial(context.Background(), url, nil)
	if err == nil {
		t.Fatalf("expected dial with no token to fail")
	}
}
