package events

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]chan []byte
	counter     atomic.Int64
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[string]chan []byte)}
}

func (b *Bus) Subscribe() (string, <-chan []byte) {
	id := fmt.Sprintf("%d", b.counter.Add(1))
	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.subscribers[id] = ch
	b.mu.Unlock()
	return id, ch
}

func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}
}

func (b *Bus) Publish(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}
