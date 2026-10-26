// Package live is the in-memory hub behind the sync API's event stream: "something changed" signals after a push,
// and who is looking at what (presence). One process holds it all, which is right for SQLite and for a single
// PostgreSQL-backed instance; nothing here is stored.
package live

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// PresenceTTL: an entry goes away this long after its last heartbeat (the app re-posts every 30 s).
const PresenceTTL = 60 * time.Second

// Event is one server-sent event, its data already JSON.
type Event struct {
	Name string
	Data []byte
}

// Presence: a device (token) of a member looking at an item of a workspace (ItemID nil = the workspace itself).
type Presence struct {
	MemberID string  `json:"memberId"`
	Name     string  `json:"name"`
	Device   string  `json:"device"`
	ItemID   *string `json:"itemId"`
	At       int64   `json:"at"`
}

// Sub is one open event stream. C is closed when the hub drops it (too slow a reader, or shutdown).
type Sub struct {
	C         chan Event
	workspace string
	token     string
}

type Hub struct {
	mu       sync.Mutex
	closed   bool
	subs     map[string]map[*Sub]struct{}    // workspace → open streams
	streams  map[string]map[string]int       // workspace → token → open streams of that token
	presence map[string]map[string]*Presence // workspace → token → entry
}

func New() *Hub {
	h := &Hub{subs: map[string]map[*Sub]struct{}{}, streams: map[string]map[string]int{}, presence: map[string]map[string]*Presence{}}
	go h.sweep()
	return h
}

// Subscribe opens a stream on a workspace for a token and returns the presence list as of that moment, so the
// stream's first event and everything after it line up.
func (h *Hub) Subscribe(workspace, token string) (*Sub, []Presence) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := &Sub{C: make(chan Event, 32), workspace: workspace, token: token}
	if h.closed {
		close(sub.C)
		return sub, []Presence{}
	}
	if h.subs[workspace] == nil {
		h.subs[workspace] = map[*Sub]struct{}{}
		h.streams[workspace] = map[string]int{}
	}
	h.subs[workspace][sub] = struct{}{}
	h.streams[workspace][token]++
	return sub, h.list(workspace)
}

// Unsubscribe is called when a stream ends. When it was the token's last stream on the workspace, the token's
// presence entry goes too.
func (h *Hub) Unsubscribe(sub *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ws := sub.workspace
	delete(h.subs[ws], sub)
	if n := h.streams[ws][sub.token] - 1; n > 0 {
		h.streams[ws][sub.token] = n
	} else {
		delete(h.streams[ws], sub.token)
		if _, ok := h.presence[ws][sub.token]; ok {
			delete(h.presence[ws], sub.token)
			h.broadcastPresence(ws)
		}
	}
	if len(h.subs[ws]) == 0 {
		delete(h.subs, ws)
		delete(h.streams, ws)
	}
	if len(h.presence[ws]) == 0 {
		delete(h.presence, ws)
	}
}

// Publish sends an event to every stream of a workspace.
func (h *Hub) Publish(workspace, name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.send(workspace, Event{Name: name, Data: data})
}

// send never blocks: a stream that cannot keep up is dropped (its C closed); the app reconnects and starts over
// from the hello event, which is cheaper than holding up every other stream.
func (h *Hub) send(workspace string, ev Event) {
	for sub := range h.subs[workspace] {
		select {
		case sub.C <- ev:
		default:
			delete(h.subs[workspace], sub)
			close(sub.C)
		}
	}
}

// SetPresence records a heartbeat. The others hear about it only when the list changes (a device appears, goes,
// or moves to another item), not on every heartbeat.
func (h *Hub) SetPresence(workspace, token string, p Presence) {
	p.At = time.Now().UnixMilli()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.presence[workspace] == nil {
		h.presence[workspace] = map[string]*Presence{}
	}
	old := h.presence[workspace][token]
	h.presence[workspace][token] = &p
	if old == nil || !sameItem(old.ItemID, p.ItemID) || old.Name != p.Name || old.Device != p.Device {
		h.broadcastPresence(workspace)
	}
}

func sameItem(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// list is the workspace's presence, sorted for a stable order (caller holds mu).
func (h *Hub) list(workspace string) []Presence {
	out := []Presence{}
	for _, p := range h.presence[workspace] {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Device < out[j].Device
	})
	return out
}

func (h *Hub) broadcastPresence(workspace string) {
	data, _ := json.Marshal(map[string]any{"presence": h.list(workspace)})
	h.send(workspace, Event{Name: "presence", Data: data})
}

// sweep drops entries whose last heartbeat is older than PresenceTTL.
func (h *Hub) sweep() {
	for range time.Tick(5 * time.Second) {
		limit := time.Now().Add(-PresenceTTL).UnixMilli()
		h.mu.Lock()
		for ws, entries := range h.presence {
			changed := false
			for token, p := range entries {
				if p.At < limit {
					delete(entries, token)
					changed = true
				}
			}
			if changed {
				h.broadcastPresence(ws)
			}
			if len(entries) == 0 {
				delete(h.presence, ws)
			}
		}
		h.mu.Unlock()
	}
}

// Close ends every stream (on shutdown, so http.Server.Shutdown does not wait for them).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ws, subs := range h.subs {
		for sub := range subs {
			close(sub.C)
		}
		delete(h.subs, ws)
	}
}
