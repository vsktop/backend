// active conn map
package ws

import (
	"context"
	"sync"
	"sync/atomic"
)

const sendBufSize = 256

type Client struct {
	AccountID string
	DeviceID  string
	SessionID string
	send      chan []byte
	hub       *Hub

	seq uint64
}

func (c *Client) nextSeq() uint64 {
	return atomic.AddUint64(&c.seq, 1)
}

func (c *Client) enqueue(payload []byte) bool {
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[string]*Client),
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.AccountID] == nil {
		h.clients[c.AccountID] = make(map[string]*Client)
	}
	h.clients[c.AccountID][c.SessionID] = c
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sessions, ok := h.clients[c.AccountID]
	if !ok {
		return
	}
	delete(sessions, c.SessionID)
	if len(sessions) == 0 {
		delete(h.clients, c.AccountID)
	}
}

func (h *Hub) IsOnline(accountID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[accountID]) > 0
}

func (h *Hub) Send(accountID string, payload []byte) int {
	h.mu.RLock()
	sessions := h.clients[accountID]
	h.mu.RUnlock()

	delivered := 0
	for _, c := range sessions {
		if c.enqueue(payload) {
			delivered++
		}
	}
	return delivered
}

func (h *Hub) Broadcast(exclude *Client, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sessions := range h.clients {
		for _, c := range sessions {
			if c == exclude {
				continue
			}
			c.enqueue(payload)
		}
	}
}

func (h *Hub) OnlineAccounts() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) Run(_ context.Context) {}
