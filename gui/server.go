package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-get-request/events"
	"go-get-request/mock"
	"go-get-request/store"
)

//go:embed web/index.html
var indexHTML []byte

type Server struct {
	port  int
	store *store.Store
	bus   *events.Bus
	mock  *mock.Server
}

func New(port int, s *store.Store, b *events.Bus, m *mock.Server) *Server {
	return &Server{port: port, store: s, bus: b, mock: m}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/routes", s.handleRoutes)
	mux.HandleFunc("/api/routes/", s.handleRoute)
	mux.HandleFunc("/api/mock/start", s.handleMockStart)
	mux.HandleFunc("/api/mock/stop", s.handleMockStop)
	mux.HandleFunc("/api/mock/status", s.handleMockStatus)
	mux.HandleFunc("/api/auth", s.handleAuth)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/logs/clear", s.handleLogsClear)
	mux.HandleFunc("/api/events", s.handleSSE)

	fmt.Printf("GUI running → http://localhost:%d\n", s.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), mux)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ── static ────────────────────────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

// ── routes API ────────────────────────────────────────────────────────────────

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListRoutes())
	case http.MethodPost:
		var route store.Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, s.store.CreateRoute(&route))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/routes/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var route store.Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		route.ID = id
		if !s.store.UpdateRoute(&route) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, route)
	case http.MethodDelete:
		if !s.store.DeleteRoute(id) {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── mock server API ───────────────────────────────────────────────────────────

func (s *Server) handleMockStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Port int `json:"port"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Port == 0 {
		req.Port = 3000
	}
	if err := s.mock.Start(req.Port); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	running, port := s.mock.Status()
	s.bus.Publish(statusEvent(running, port))
	writeJSON(w, http.StatusOK, map[string]any{"running": running, "port": port})
}

func (s *Server) handleMockStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.mock.Stop(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.bus.Publish(statusEvent(false, 0))
	writeJSON(w, http.StatusOK, map[string]any{"running": false})
}

func (s *Server) handleMockStatus(w http.ResponseWriter, r *http.Request) {
	running, port := s.mock.Status()
	writeJSON(w, http.StatusOK, map[string]any{"running": running, "port": port})
}

// ── auth API ──────────────────────────────────────────────────────────────────

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.GetAuth())
	case http.MethodPut:
		var cfg store.AuthConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if cfg.LoginPath == "" {
			cfg.LoginPath = "/auth/login"
		}
		if cfg.TTLSeconds <= 0 {
			cfg.TTLSeconds = 3600
		}
		if cfg.Secret == "" {
			cfg.Secret = s.store.GetAuth().Secret
		}
		s.store.SetAuth(cfg)
		writeJSON(w, http.StatusOK, cfg)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── logs API ──────────────────────────────────────────────────────────────────

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.store.ListLogs())
}

func (s *Server) handleLogsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.store.ClearLogs()
	s.bus.Publish([]byte(`{"type":"clear"}`))
	w.WriteHeader(http.StatusNoContent)
}

// ── SSE ───────────────────────────────────────────────────────────────────────

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	running, port := s.mock.Status()
	fmt.Fprintf(w, "data: %s\n\n", statusEvent(running, port))
	flusher.Flush()

	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func statusEvent(running bool, port int) []byte {
	data, _ := json.Marshal(map[string]any{"type": "status", "running": running, "port": port})
	return data
}
