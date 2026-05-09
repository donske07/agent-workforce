package office

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/agent-workforce/agent-workforce/internal/assets"
	"github.com/agent-workforce/agent-workforce/internal/product"
)

type Event map[string]any

type Broker struct {
	mu      sync.Mutex
	clients map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: map[chan Event]struct{}{}}
}

func (b *Broker) Broadcast(event Event) {
	b.mu.Lock()
	clients := make([]chan Event, 0, len(b.clients))
	for ch := range b.clients {
		clients = append(clients, ch)
	}
	b.mu.Unlock()
	for _, ch := range clients {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *Broker) Add(ch chan Event) {
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
}

func (b *Broker) Remove(ch chan Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func NewHandler() http.Handler {
	broker := NewBroker()
	mux := http.NewServeMux()
	frontend, err := fs.Sub(assets.Files, "files/frontend")
	if err != nil {
		panic(err)
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/agents.js", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		data, err := json.Marshal(product.Agents)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = fmt.Fprintf(w, "window.agentWorkforceAgents = %s;\n", data)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		ch := make(chan Event, 16)
		broker.Add(ch)
		defer broker.Remove(ch)
		writeSSE(w, Event{"type": "connected"})
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-ch:
				writeSSE(w, event)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("/agent-state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
			return
		}
		var body struct {
			AgentID string `json:"agent_id"`
			State   string `json:"state"`
			Title   string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body: " + err.Error()})
			return
		}
		body.AgentID = strings.TrimSpace(body.AgentID)
		body.State = strings.TrimSpace(body.State)
		body.Title = strings.TrimSpace(body.Title)
		if body.AgentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "agent_id is required."})
			return
		}
		if body.State != product.OfficeStateThinking && body.State != product.OfficeStateIdle {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("state must be %q or %q.", product.OfficeStateThinking, product.OfficeStateIdle)})
			return
		}
		broker.Broadcast(Event{"type": "agent_state", "agent_id": body.AgentID, "state": body.State, "title": body.Title})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("/forge-event", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
			return
		}
		var body Event
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body: " + err.Error()})
			return
		}
		if err := validateForgeEvent(body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		broker.Broadcast(body)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		assetPath := strings.TrimPrefix(r.URL.Path, "/")
		if assetPath == "" {
			assetPath = "index.html"
		}
		for _, segment := range strings.Split(assetPath, "/") {
			if segment == ".." {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		assetPath = strings.TrimPrefix(path.Clean("/"+assetPath), "/")
		data, err := fs.ReadFile(frontend, assetPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if ctype := mime.TypeByExtension(path.Ext(assetPath)); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		_, _ = w.Write(data)
	})
	return mux
}

func Run(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Forge Agent Office running at http://%s\n", addr)
	server := &http.Server{Addr: addr, Handler: NewHandler()}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func validateForgeEvent(event Event) error {
	eventType := eventString(event, "type")
	if eventType != "forge_run_started" && eventType != "forge_output" && eventType != "forge_progress" && eventType != "forge_run_finished" {
		return fmt.Errorf("unsupported forge event type: %s", eventType)
	}
	if eventString(event, "run_id") == "" {
		return fmt.Errorf("run_id is required")
	}
	if eventString(event, "agent_id") == "" {
		return fmt.Errorf("agent_id is required")
	}
	if eventType == "forge_output" {
		stream := eventString(event, "stream")
		if stream != "stdout" && stream != "stderr" {
			return fmt.Errorf("stream must be stdout or stderr")
		}
		if _, ok := event["chunk"]; !ok {
			return fmt.Errorf("chunk is required")
		}
	}
	if eventType == "forge_progress" && eventString(event, "message") == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

func eventString(event Event, key string) string {
	value, ok := event[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func writeSSE(w http.ResponseWriter, event Event) {
	data, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
