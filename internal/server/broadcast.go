package server

import "sync"

// broadcastHub fans out event messages to every WebSocket connection
// registered for a tournament. It's in-process memory only -- no
// external pub/sub -- matching the single-binary, offline-first
// architecture. A registrant that isn't reading fast enough (buffer
// full) has its message dropped rather than blocking the broadcaster,
// other connections, or the HTTP request that triggered the broadcast.
type broadcastHub struct {
	mu    sync.Mutex
	conns map[int64]map[chan []byte]struct{}
}

func newBroadcastHub() *broadcastHub {
	return &broadcastHub{conns: make(map[int64]map[chan []byte]struct{})}
}

func (h *broadcastHub) register(tournamentID int64) chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[tournamentID] == nil {
		h.conns[tournamentID] = make(map[chan []byte]struct{})
	}
	h.conns[tournamentID][ch] = struct{}{}
	return ch
}

func (h *broadcastHub) unregister(tournamentID int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[tournamentID], ch)
}

func (h *broadcastHub) broadcast(tournamentID int64, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.conns[tournamentID] {
		select {
		case ch <- msg:
		default:
		}
	}
}
