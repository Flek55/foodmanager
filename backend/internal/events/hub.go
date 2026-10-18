// Package events is an in-process pub/sub: services publish "household X
// changed", and every open SSE connection of that household's members hears
// it. A single server instance is all we need; with several instances this
// would move to Postgres LISTEN/NOTIFY behind the same interface.
package events

import (
	"sync"

	"github.com/google/uuid"
)

const (
	// KindList: list data (items, products, categories, stores) changed.
	KindList = "list"
	// KindHousehold: name or members changed.
	KindHousehold = "household"
)

type Event struct {
	Kind    string `json:"kind"`
	Version int64  `json:"version,omitempty"`
}

// Publisher is what services depend on, so tests can pass a no-op.
type Publisher interface {
	Publish(householdID uuid.UUID, ev Event)
}

type Hub struct {
	mu   sync.Mutex
	subs map[uuid.UUID]map[*Subscription]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uuid.UUID]map[*Subscription]struct{})}
}

type Subscription struct {
	C           <-chan Event
	c           chan Event
	hub         *Hub
	householdID uuid.UUID
}

func (h *Hub) Subscribe(householdID uuid.UUID) *Subscription {
	c := make(chan Event, 16)
	s := &Subscription{C: c, c: c, hub: h, householdID: householdID}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[householdID] == nil {
		h.subs[householdID] = make(map[*Subscription]struct{})
	}
	h.subs[householdID][s] = struct{}{}
	return s
}

func (s *Subscription) Close() {
	h := s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[s.householdID], s)
	if len(h.subs[s.householdID]) == 0 {
		delete(h.subs, s.householdID)
	}
}

// Publish never blocks. If a subscriber's buffer is full, the event is
// dropped for it: the buffer already holds unread "something changed"
// events, and the client refetches everything on any of them anyway.
func (h *Hub) Publish(householdID uuid.UUID, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[householdID] {
		select {
		case s.c <- ev:
		default:
		}
	}
}

// Nop discards events; handy in tests that don't care about them.
type Nop struct{}

func (Nop) Publish(uuid.UUID, Event) {}
