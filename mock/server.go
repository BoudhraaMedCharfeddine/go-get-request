package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-get-request/events"
	"go-get-request/jwt"
	"go-get-request/store"
)

type Server struct {
	store   *store.Store
	bus     *events.Bus
	srv     *http.Server
	mu      sync.Mutex
	running bool
	port    int
}

func New(s *store.Store, b *events.Bus) *Server {
	return &Server{store: s, bus: b}
}

func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("mock server already running on port %d", s.port)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	s.srv = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}

	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on :%d: %w", port, err)
	}
	s.port = port
	s.running = true
	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("mock server: %v", err)
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return fmt.Errorf("mock server is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

func (s *Server) Status() (running bool, port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.port
}

// ── Request handler ───────────────────────────────────────────────────────────

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	authCfg := s.store.GetAuth()

	// Intercept login endpoint when auth simulation is enabled.
	if authCfg.Enabled && r.Method == http.MethodPost && r.URL.Path == authCfg.LoginPath {
		s.handleLogin(w, r, authCfg)
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	routes := s.store.ListRoutes()
	var matched *store.Route
	for _, route := range routes {
		if pathMatches(route.Path, r.URL.Path) && methodMatches(route.Method, r.Method) {
			matched = route
			break
		}
	}

	reqHeaders := make(map[string]string, len(r.Header))
	for k := range r.Header {
		reqHeaders[k] = r.Header.Get(k)
	}

	entry := &store.Log{
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		Headers:   reqHeaders,
		Body:      string(body),
	}

	if matched != nil {
		// Enforce JWT on protected routes when auth simulation is active.
		if matched.Protected && authCfg.Enabled {
			if err := validateBearer(r, authCfg.Secret); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"unauthorized: %s"}`, err.Error())
				entry.Matched = true
				entry.AuthFailed = true
				entry.StatusCode = http.StatusUnauthorized
				s.logAndPublish(entry)
				return
			}
		}

		if matched.DelayMs > 0 {
			time.Sleep(time.Duration(matched.DelayMs) * time.Millisecond)
		}
		for k, v := range matched.ResponseHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(matched.StatusCode)
		if matched.ResponseBody != "" {
			fmt.Fprint(w, matched.ResponseBody)
		}
		entry.Matched = true
		entry.RouteID = matched.ID
		entry.StatusCode = matched.StatusCode
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":"no route matched %s %s"}`, r.Method, r.URL.Path)
		entry.StatusCode = http.StatusNotFound
	}

	s.logAndPublish(entry)
}

// ── Auth login ────────────────────────────────────────────────────────────────

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, cfg store.AuthConfig) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.Unmarshal(body, &creds)

	reqHeaders := make(map[string]string, len(r.Header))
	for k := range r.Header {
		reqHeaders[k] = r.Header.Get(k)
	}

	entry := &store.Log{
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		Headers:   reqHeaders,
		Body:      string(body),
		Matched:   true,
	}
	defer s.logAndPublish(entry)

	w.Header().Set("Content-Type", "application/json")

	if creds.Username != cfg.Username || creds.Password != cfg.Password {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid credentials"}`)
		entry.StatusCode = http.StatusUnauthorized
		return
	}

	token, err := jwt.Issue(creds.Username, cfg.Secret, time.Duration(cfg.TTLSeconds)*time.Second)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"could not sign token"}`)
		entry.StatusCode = http.StatusInternalServerError
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"token":     token,
		"type":      "Bearer",
		"expiresIn": cfg.TTLSeconds,
	})
	entry.StatusCode = http.StatusOK
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *Server) logAndPublish(entry *store.Log) {
	s.store.AddLog(entry)
	if data, err := json.Marshal(map[string]any{"type": "log", "data": entry}); err == nil {
		s.bus.Publish(data)
	}
}

func validateBearer(r *http.Request, secret string) error {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return errors.New("missing Authorization: Bearer header")
	}
	_, err := jwt.Validate(strings.TrimPrefix(auth, "Bearer "), secret)
	return err
}

func pathMatches(pattern, path string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == path
}

func methodMatches(routeMethod, reqMethod string) bool {
	return routeMethod == "*" || strings.EqualFold(routeMethod, reqMethod)
}
