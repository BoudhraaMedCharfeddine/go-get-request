package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Route struct {
	ID              string            `json:"id"`
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	StatusCode      int               `json:"statusCode"`
	ResponseHeaders map[string]string `json:"responseHeaders"`
	ResponseBody    string            `json:"responseBody"`
	DelayMs         int               `json:"delayMs"`
	Protected       bool              `json:"protected"`
	CreatedAt       time.Time         `json:"createdAt"`
}

type Log struct {
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	StatusCode int               `json:"statusCode"`
	Matched    bool              `json:"matched"`
	RouteID    string            `json:"routeId,omitempty"`
	AuthFailed bool              `json:"authFailed,omitempty"`
}

type AuthConfig struct {
	Enabled    bool   `json:"enabled"`
	LoginPath  string `json:"loginPath"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Secret     string `json:"secret"`
	TTLSeconds int    `json:"ttlSeconds"`
}

type Store struct {
	mu      sync.RWMutex
	routes  map[string]*Route
	logs    []*Log
	auth    AuthConfig
	counter atomic.Int64
}

func New() *Store {
	return &Store{
		routes: make(map[string]*Route),
		auth:   defaultAuth(),
	}
}

func defaultAuth() AuthConfig {
	return AuthConfig{
		Enabled:    false,
		LoginPath:  "/auth/login",
		Username:   "admin",
		Password:   "secret",
		Secret:     generateSecret(),
		TTLSeconds: 3600,
	}
}

func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Store) nextID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), s.counter.Add(1))
}

// ── Routes ────────────────────────────────────────────────────────────────────

func (s *Store) CreateRoute(r *Route) *Route {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = s.nextID()
	r.CreatedAt = time.Now()
	if r.StatusCode == 0 {
		r.StatusCode = 200
	}
	if r.ResponseHeaders == nil {
		r.ResponseHeaders = make(map[string]string)
	}
	s.routes[r.ID] = r
	return r
}

func (s *Store) UpdateRoute(r *Route) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.routes[r.ID]
	if !ok {
		return false
	}
	r.CreatedAt = existing.CreatedAt
	if r.ResponseHeaders == nil {
		r.ResponseHeaders = make(map[string]string)
	}
	s.routes[r.ID] = r
	return true
}

func (s *Store) DeleteRoute(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.routes[id]; !ok {
		return false
	}
	delete(s.routes, id)
	return true
}

func (s *Store) ListRoutes() []*Route {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Route, 0, len(s.routes))
	for _, r := range s.routes {
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func (s *Store) AddLog(l *Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.ID = s.nextID()
	s.logs = append([]*Log{l}, s.logs...)
	if len(s.logs) > 500 {
		s.logs = s.logs[:500]
	}
}

func (s *Store) ListLogs() []*Log {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Log, len(s.logs))
	copy(result, s.logs)
	return result
}

func (s *Store) ClearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = s.logs[:0]
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func (s *Store) GetAuth() AuthConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.auth
}

func (s *Store) SetAuth(a AuthConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth = a
}
