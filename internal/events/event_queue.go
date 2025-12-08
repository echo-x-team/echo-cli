package events

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrEventQueueClosed 表示事件队列已关闭。
	ErrEventQueueClosed = errors.New("event queue closed")
	// ErrEventDropped 表示事件被慢消费者丢弃。
	ErrEventDropped = errors.New("event dropped by slow subscriber")
)

// EventQueue 是 EQ，负责事件广播。
type EventQueue struct {
	mu     sync.Mutex
	subs   []chan Event
	buffer int
	closed bool
}

// NewEventQueue 创建事件队列，buffer 是每个订阅者的缓存大小。
func NewEventQueue(buffer int) *EventQueue {
	if buffer <= 0 {
		buffer = 64
	}
	return &EventQueue{buffer: buffer}
}

// Subscribe 订阅事件流。通道会在 Close 时关闭。
func (q *EventQueue) Subscribe() <-chan Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		ch := make(chan Event)
		close(ch)
		return ch
	}
	ch := make(chan Event, q.buffer)
	q.subs = append(q.subs, ch)
	return ch
}

// Publish 发布事件到所有订阅者。若存在丢弃，则返回 ErrEventDropped。
func (q *EventQueue) Publish(ctx context.Context, event Event) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrEventQueueClosed
	}
	subs := append([]chan Event{}, q.subs...)
	q.mu.Unlock()

	dropped := false
	for _, ch := range subs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- event:
		default:
			dropped = true
		}
	}
	if dropped {
		return ErrEventDropped
	}
	return nil
}

// Close 关闭事件队列和所有订阅通道。
func (q *EventQueue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	subs := q.subs
	q.subs = nil
	q.mu.Unlock()

	for _, ch := range subs {
		close(ch)
	}
}

// SubscriberCount 返回当前订阅者数量。
func (q *EventQueue) SubscriberCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.subs)
}
