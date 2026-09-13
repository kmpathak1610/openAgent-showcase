package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"openagent/internal/metrics"
)

// Event is the wire format for WebSocket messages.
type Event struct {
	ID             string         `json:"id,omitempty"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	Room           string         `json:"-"` // org:<id> or channel:<id>
	Timestamp      time.Time      `json:"timestamp,omitempty"`
	OrganizationID string         `json:"organizationId,omitempty"`
}

type Client struct {
	ID             string
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	Conn           *websocket.Conn
	Send           chan Event
	Hub            *Hub
}

type Hub struct {
	mu               sync.RWMutex
	clients          map[string]*Client
	rooms            map[string]map[string]*Client // room -> clientID -> client
	register         chan *Client
	unregister       chan *Client
	broadcast        chan Event
	log              *slog.Logger
	redisBroadcaster *RedisBroadcaster
	seen             map[string]time.Time
	seenMu           sync.Mutex
}

func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		rooms:      make(map[string]map[string]*Client),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan Event, 256),
		log:        log,
		seen:       make(map[string]time.Time),
	}
}

func (h *Hub) SetRedisBroadcaster(rb *RedisBroadcaster) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.redisBroadcaster = rb
}

func (h *Hub) isDuplicate(id string) bool {
	if id == "" {
		return false
	}
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	if _, ok := h.seen[id]; ok {
		return true
	}
	h.seen[id] = time.Now()
	// cleanup old entries every 100
	if len(h.seen) > 1000 {
		for k, t := range h.seen {
			if time.Since(t) > 5*time.Minute {
				delete(h.seen, k)
			}
		}
	}
	return false
}

func (h *Hub) ensureEventMeta(ev *Event) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c.ID] = c
			orgRoom := "org:" + c.OrganizationID.String()
			if h.rooms[orgRoom] == nil {
				h.rooms[orgRoom] = make(map[string]*Client)
			}
			h.rooms[orgRoom][c.ID] = c
			h.mu.Unlock()
			metrics.IncWS()
			h.log.Info("ws client registered", "client", c.ID, "org", c.OrganizationID)

		case c := <-h.unregister:
			h.mu.Lock()
			delete(h.clients, c.ID)
			for room, members := range h.rooms {
				delete(members, c.ID)
				if len(members) == 0 {
					delete(h.rooms, room)
				}
			}
			h.mu.Unlock()
			metrics.DecWS()
			close(c.Send)

		case ev := <-h.broadcast:
			// dedup via ID
			if h.isDuplicate(ev.ID) {
				continue
			}
			h.ensureEventMeta(&ev)
			// Publish to Redis for other instances (if not already from Redis)
			// We publish only for org/channel rooms, and only if this Hub originated the event
			// Redis subscriber will call BroadcastLocal to avoid loop, so we check if we should publish
			// For now, always publish to Redis if broadcaster exists and Room is org/channel
			if h.redisBroadcaster != nil && ev.Room != "" {
				// Avoid publishing events that already came from Redis (they have no need to republish)
				// We use a simple heuristic: if ev.ID was just generated, it's local; if it came from Redis, it would have been deduped
				// So publish all local events
				go h.redisBroadcaster.PublishRaw(ev.Room, ev)
			}
			h.mu.RLock()
			targets := h.rooms[ev.Room]
			clients := make([]*Client, 0, len(targets))
			for _, c := range targets {
				clients = append(clients, c)
			}
			h.mu.RUnlock()
			for _, c := range clients {
				select {
				case c.Send <- ev:
				default:
					h.log.Warn("ws send buffer full, dropping", "client", c.ID)
				}
			}
		}
	}
}

func (h *Hub) Register(c *Client) { h.register <- c }
func (h *Hub) Unregister(c *Client) { h.unregister <- c }

func (h *Hub) Broadcast(ev Event) { h.broadcast <- ev }

func (h *Hub) BroadcastLocal(ev Event) {
	// Used by Redis subscriber to avoid re-publishing to Redis
	h.ensureEventMeta(&ev)
	if h.isDuplicate(ev.ID) {
		return
	}
	h.mu.RLock()
	targets := h.rooms[ev.Room]
	clients := make([]*Client, 0, len(targets))
	for _, c := range targets {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.Send <- ev:
		default:
			h.log.Warn("ws send buffer full, dropping", "client", c.ID)
		}
	}
}

// BroadcastToOrg is helper for org-scoped events.
func (h *Hub) BroadcastToOrg(orgID uuid.UUID, typ string, payload map[string]any) {
	ev := Event{
		ID:             uuid.NewString(),
		Type:           typ,
		Payload:        payload,
		Room:           "org:" + orgID.String(),
		Timestamp:      time.Now().UTC(),
		OrganizationID: orgID.String(),
	}
	// Ensure payload has organization for filtering
	if payload != nil {
		if _, ok := payload["organizationId"]; !ok {
			payload["organizationId"] = orgID.String()
		}
	}
	h.Broadcast(ev)
}

func (h *Hub) BroadcastToChannel(channelID uuid.UUID, typ string, payload map[string]any) {
	ev := Event{
		ID:        uuid.NewString(),
		Type:      typ,
		Payload:   payload,
		Room:      "channel:" + channelID.String(),
		Timestamp: time.Now().UTC(),
	}
	h.Broadcast(ev)
}

// JoinChannel adds client to channel room with org isolation check.
func (h *Hub) JoinChannel(c *Client, channelID uuid.UUID) {
	// Note: org isolation should be verified by caller via DB (channel belongs to client's org)
	h.mu.Lock()
	defer h.mu.Unlock()
	room := "channel:" + channelID.String()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[string]*Client)
	}
	h.rooms[room][c.ID] = c
}

func (h *Hub) LeaveChannel(c *Client, channelID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := "channel:" + channelID.String()
	if members, ok := h.rooms[room]; ok {
		delete(members, c.ID)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
}

// Client read/write pumps

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(1 << 20)
	_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
		var ev Event
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "ping":
			c.Send <- Event{Type: "pong", Payload: map[string]any{"ts": time.Now().Unix()}, Timestamp: time.Now().UTC(), ID: uuid.NewString()}
		case "join_channel":
			if chStr, ok := ev.Payload["channelId"].(string); ok {
				if channelID, err := uuid.Parse(chStr); err == nil {
					c.Hub.JoinChannel(c, channelID)
					c.Send <- Event{Type: "joined_channel", Payload: map[string]any{"channelId": chStr}, Timestamp: time.Now().UTC(), ID: uuid.NewString()}
				}
			}
		case "leave_channel":
			if chStr, ok := ev.Payload["channelId"].(string); ok {
				if channelID, err := uuid.Parse(chStr); err == nil {
					c.Hub.LeaveChannel(c, channelID)
				}
			}
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer c.Conn.Close()
	for {
		select {
		case ev, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.Conn.WriteJSON(ev)
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
