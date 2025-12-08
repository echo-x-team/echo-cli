package events

import "sync"

// Simple pub-sub for tool events.
type Bus struct {
	mu     sync.Mutex
	subs   []chan any
	closed bool
}

func NewBus() *Bus {
	return &Bus{}
}

func (b *Bus) Subscribe() <-chan any {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		ch := make(chan any)
		close(ch)
		return ch
	}
	ch := make(chan any, 32)
	b.subs = append(b.subs, ch)
	return ch
}

func (b *Bus) Publish(evt any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		close(ch)
	}
	b.closed = true
}
