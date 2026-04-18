package realtime

import "sync"

type Event struct {
	UserID  string         `json:"userId"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

type Hub struct {
	mu           sync.RWMutex
	subscribers  map[string]int
	lastByUserID map[string]Event
}

func NewHub() *Hub {
	return &Hub{
		subscribers:  make(map[string]int),
		lastByUserID: make(map[string]Event),
	}
}

func (h *Hub) Subscribe(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[userID]++
}

func (h *Hub) Unsubscribe(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[userID] <= 1 {
		delete(h.subscribers, userID)
		return
	}
	h.subscribers[userID]--
}

func (h *Hub) Publish(evt Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastByUserID[evt.UserID] = evt
}

func (h *Hub) LastEvent(userID string) (Event, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	evt, ok := h.lastByUserID[userID]
	return evt, ok
}
