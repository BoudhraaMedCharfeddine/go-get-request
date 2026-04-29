package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go-get-request/events"
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

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
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

	s.store.AddLog(entry)
	if data, err := json.Marshal(map[string]any{"type": "log", "data": entry}); err == nil {
		s.bus.Publish(data)
	}
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
