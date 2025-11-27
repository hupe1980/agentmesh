package viz

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512 KB
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// Message represents a WebSocket message
type Message struct {
	Type   string `json:"type"`
	RunID  string `json:"run_id,omitempty"`
	Data   any    `json:"data,omitempty"`
	Filter any    `json:"filter,omitempty"` // For subscription filters
}

// WebSocketHub manages WebSocket connections and broadcasts
type WebSocketHub struct {
	mu         sync.RWMutex
	clients    map[*WebSocketClient]bool
	broadcast  chan Message
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	shutdown   chan struct{}
	maxClients int
}

// NewWebSocketHub creates a new WebSocket hub
func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[*WebSocketClient]bool),
		broadcast:  make(chan Message, 256),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		shutdown:   make(chan struct{}),
		maxClients: 1000, // Prevent resource exhaustion
	}
}

// Run starts the WebSocket hub
func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if len(h.clients) >= h.maxClients {
				h.mu.Unlock()
				// Reject client if at capacity
				close(client.send)
				_ = client.conn.Close()
				log.Printf("WebSocket client limit reached (%d), rejecting connection", h.maxClients)
				continue
			}
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.broadcastToSubscribers(message)

		case <-h.shutdown:
			// Gracefully close all clients
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				_ = client.conn.Close()
			}
			h.clients = make(map[*WebSocketClient]bool)
			h.mu.Unlock()
			return
		}
	}
}

// BroadcastEvent broadcasts an event to all subscribed clients
func (h *WebSocketHub) BroadcastEvent(runID string, event ExecutionEvent) {
	h.broadcast <- Message{
		Type:  "event",
		RunID: runID,
		Data:  event,
	}
}

// BroadcastMessage broadcasts a generic message
func (h *WebSocketHub) BroadcastMessage(msg Message) {
	select {
	case h.broadcast <- msg:
	case <-h.shutdown:
		// Hub is shutting down, drop message
	}
}

// Stop gracefully stops the WebSocket hub
func (h *WebSocketHub) Stop() {
	close(h.shutdown)
}

// broadcastToSubscribers sends message to relevant subscribers
func (h *WebSocketHub) broadcastToSubscribers(message Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Check if client's subscription matches
		if !client.matchesSubscription(message) {
			continue
		}

		select {
		case client.send <- message:
		default:
			// Client's send buffer is full, skip
		}
	}
}

// Subscription represents a client's subscription to a run with filters
type Subscription struct {
	RunID  string
	Filter EventFilter
}

// WebSocketClient represents a single WebSocket connection
type WebSocketClient struct {
	hub           *WebSocketHub
	conn          *websocket.Conn
	send          chan Message
	mu            sync.RWMutex
	subscriptions map[string]*Subscription // map[runID]*Subscription
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(hub *WebSocketHub, conn *websocket.Conn) *WebSocketClient {
	return &WebSocketClient{
		hub:           hub,
		conn:          conn,
		send:          make(chan Message, 256),
		subscriptions: make(map[string]*Subscription),
	}
}

// subscribe subscribes the client to a run with optional filter
func (c *WebSocketClient) subscribe(runID string, filter *EventFilter) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &Subscription{RunID: runID}
	if filter != nil {
		sub.Filter = *filter
	}
	c.subscriptions[runID] = sub
}

// unsubscribe unsubscribes the client from a run
func (c *WebSocketClient) unsubscribe(runID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscriptions, runID)
}

// updateFilter updates the filter for an existing subscription
func (c *WebSocketClient) updateFilter(runID string, filter EventFilter) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if sub, ok := c.subscriptions[runID]; ok {
		sub.Filter = filter
	}
}

// matchesSubscription checks if an event matches any client subscription
func (c *WebSocketClient) matchesSubscription(message Message) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Non-event messages pass through
	if message.Type != "event" {
		return true
	}

	sub, ok := c.subscriptions[message.RunID]
	if !ok {
		return false // Not subscribed to this run
	}

	// No filter means accept all events for this run
	if len(sub.Filter.Types) == 0 && len(sub.Filter.Nodes) == 0 {
		return true
	}

	// Extract event from message data
	event, ok := message.Data.(ExecutionEvent)
	if !ok {
		return true // Can't filter, pass through
	}

	// Apply filter
	if len(sub.Filter.Types) > 0 {
		found := false
		for _, t := range sub.Filter.Types {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(sub.Filter.Nodes) > 0 {
		found := false
		for _, n := range sub.Filter.Nodes {
			if n == event.Node {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WebSocketClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle client messages
		c.handleMessage(msg)
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming client messages
func (c *WebSocketClient) handleMessage(msg Message) {
	switch msg.Type {
	case "subscribe":
		var filter *EventFilter
		if msg.Filter != nil {
			// Try to parse filter from message
			if filterMap, ok := msg.Filter.(map[string]any); ok {
				filter = parseEventFilter(filterMap)
			}
		}
		c.subscribe(msg.RunID, filter)

	case "unsubscribe":
		c.unsubscribe(msg.RunID)

	case "update_filter":
		if msg.Filter != nil {
			if filterMap, ok := msg.Filter.(map[string]any); ok {
				filter := parseEventFilter(filterMap)
				if filter != nil {
					c.updateFilter(msg.RunID, *filter)
				}
			}
		}

	case "control":
		// Handle execution control commands
		c.handleControlMessage(msg)

	case "breakpoint":
		// Handle breakpoint commands
		c.handleBreakpointMessage(msg)

	case "inspect":
		// Handle state inspection requests
		c.handleInspectMessage(msg)
	}
}

// handleControlMessage handles execution control commands
func (c *WebSocketClient) handleControlMessage(msg Message) {
	if msg.Data == nil {
		return
	}

	dataMap, ok := msg.Data.(map[string]any)
	if !ok {
		return
	}

	command, ok := dataMap["command"].(string)
	if !ok {
		return
	}

	// Send response back to client
	c.send <- Message{
		Type:  "control_response",
		RunID: msg.RunID,
		Data: map[string]any{
			"command": command,
			"status":  "received",
		},
	}
}

// handleBreakpointMessage handles breakpoint management commands
func (c *WebSocketClient) handleBreakpointMessage(msg Message) {
	if msg.Data == nil {
		return
	}

	dataMap, ok := msg.Data.(map[string]any)
	if !ok {
		return
	}

	action, ok := dataMap["action"].(string)
	if !ok {
		return
	}

	// Send response back to client
	c.send <- Message{
		Type:  "breakpoint_response",
		RunID: msg.RunID,
		Data: map[string]any{
			"action": action,
			"status": "received",
		},
	}
}

// handleInspectMessage handles state inspection requests
func (c *WebSocketClient) handleInspectMessage(msg Message) {
	if msg.Data == nil {
		return
	}

	// Send response back to client
	c.send <- Message{
		Type:  "inspect_response",
		RunID: msg.RunID,
		Data: map[string]any{
			"status": "received",
		},
	}
}

// parseEventFilter parses a filter from a map
func parseEventFilter(filterMap map[string]any) *EventFilter {
	filter := &EventFilter{}

	if types, ok := filterMap["types"].([]any); ok {
		for _, t := range types {
			if typeStr, ok := t.(string); ok {
				filter.Types = append(filter.Types, EventType(typeStr))
			}
		}
	}

	if nodes, ok := filterMap["nodes"].([]any); ok {
		for _, n := range nodes {
			if nodeStr, ok := n.(string); ok {
				filter.Nodes = append(filter.Nodes, nodeStr)
			}
		}
	}

	if searchText, ok := filterMap["search_text"].(string); ok {
		filter.SearchText = searchText
	}

	if limit, ok := filterMap["limit"].(float64); ok {
		filter.Limit = int(limit)
	}

	if offset, ok := filterMap["offset"].(float64); ok {
		filter.Offset = int(offset)
	}

	return filter
}

// ServeWS handles WebSocket requests from clients
func (h *WebSocketHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := NewWebSocketClient(h, conn)
	h.register <- client

	// Start pumps
	go client.writePump()
	go client.readPump()
}

// handleWebSocket is a wrapper for ServeWS
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.wsHub.ServeWS(w, r)
}
