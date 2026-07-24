package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/KyberixCo/Pharus/pkg/logger"
)

type EventType string

const (
	EventTaskUpdated  EventType = "task_updated"
	EventToolsChanged EventType = "tools_changed"
)

type EventMessage struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

type SubscriptionManager struct {
	mu          sync.RWMutex
	subscribers map[string]chan EventMessage
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		subscribers: make(map[string]chan EventMessage),
	}
}

func (sm *SubscriptionManager) Subscribe(subID string) chan EventMessage {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ch := make(chan EventMessage, 50)
	sm.subscribers[subID] = ch
	return ch
}

func (sm *SubscriptionManager) Unsubscribe(subID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ch, ok := sm.subscribers[subID]; ok {
		close(ch)
		delete(sm.subscribers, subID)
	}
}

func (sm *SubscriptionManager) Broadcast(evtType EventType, payload any) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	msg := EventMessage{
		ID:        fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:      evtType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	for _, ch := range sm.subscribers {
		select {
		case ch <- msg:
		default:
			// Buffer lleno, descarta mensaje antiguo para no bloquear
		}
	}
}

func (sm *SubscriptionManager) HandleListen(w http.ResponseWriter, r *http.Request) {
	tokenInfo, ok := TokenInfoFromContext(r.Context())
	if !ok || tokenInfo == nil || tokenInfo.UserID == "" {
		http.Error(w, `{"error":"unauthorized: missing or invalid TokenInfo.UserID"}`, http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming no soportado", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	msgChan := sm.Subscribe(subID)
	defer sm.Unsubscribe(subID)

	log := logger.Get()
	log.Info("Cliente MCP conectado a canal de suscripción SSE", "subID", subID)

	// Enviar heartbeat inicial
	fmt.Fprintf(w, "event: connected\ndata: {\"subscription_id\":\"%s\"}\n\n", subID)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Type, string(data))
			flusher.Flush()
		}
	}
}
