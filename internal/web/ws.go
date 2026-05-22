package web

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/horizon6666/max-team/internal/bus"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
	Time int64  `json:"time"`
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	done       chan struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[ws] client connected, total: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[ws] client disconnected, total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					go func(c *Client) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()

		case <-h.done:
			return
		}
	}
}

func (h *Hub) Stop() {
	close(h.done)
}

func (h *Hub) BroadcastMessage(msg bus.Message) {
	evt := WSEvent{
		Type: msgTypeToEventType(msg.Type),
		Time: time.Now().UnixMilli(),
		Data: map[string]any{
			"id":      msg.ID,
			"from":    msg.From,
			"to":      msg.To,
			"type":    int(msg.Type),
			"payload": formatPayload(msg.Payload),
		},
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- data:
	default:
	}
}

func (h *Hub) BroadcastEvent(eventType string, data any) {
	evt := WSEvent{
		Type: eventType,
		Time: time.Now().UnixMilli(),
		Data: data,
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- b:
	default:
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}
	s.hub.register <- client

	go client.writePump()
	go client.readPump(s.hub)
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

func msgTypeToEventType(t bus.MessageType) string {
	switch t {
	case bus.MsgUserInput:
		return "user_input"
	case bus.MsgUserReply:
		return "agent_reply"
	case bus.MsgTaskAssign:
		return "task_assign"
	case bus.MsgTaskResult:
		return "task_result"
	case bus.MsgTaskFailed:
		return "task_failed"
	case bus.MsgProgress:
		return "progress"
	case bus.MsgApprovalReq:
		return "approval_request"
	case bus.MsgApprovalResp:
		return "approval_response"
	case bus.MsgAllTasksDone:
		return "all_tasks_done"
	case bus.MsgNeedClarify:
		return "need_clarify"
	default:
		return "unknown"
	}
}

func formatPayload(p any) any {
	switch v := p.(type) {
	case string:
		return v
	case bool:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var result any
		json.Unmarshal(data, &result)
		return result
	}
}
