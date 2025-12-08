package events

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrSubmissionQueueClosed 表示队列已关闭，无法再提交或接收。
	ErrSubmissionQueueClosed = errors.New("submission queue closed")
)

// SubmissionQueue 是一个有界的提交队列（SQ）。
type SubmissionQueue struct {
	ch        chan Submission
	closeOnce sync.Once
}

// NewSubmissionQueue 创建一个新的 SubmissionQueue。
func NewSubmissionQueue(capacity int) *SubmissionQueue {
	if capacity <= 0 {
		capacity = 64
	}
	return &SubmissionQueue{ch: make(chan Submission, capacity)}
}

// Submit 将提交放入队列；支持 ctx 取消。
func (q *SubmissionQueue) Submit(ctx context.Context, submission Submission) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.ch <- submission:
		return nil
	default:
		// 等待一个可写位或取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		case q.ch <- submission:
			return nil
		}
	}
}

// Receive 读取一条提交；若队列已关闭则返回 ErrSubmissionQueueClosed。
func (q *SubmissionQueue) Receive(ctx context.Context) (Submission, error) {
	select {
	case <-ctx.Done():
		return Submission{}, ctx.Err()
	case sub, ok := <-q.ch:
		if !ok {
			return Submission{}, ErrSubmissionQueueClosed
		}
		return sub, nil
	}
}

// Len 返回当前队列长度。
func (q *SubmissionQueue) Len() int {
	return len(q.ch)
}

// Close 关闭队列，停止进一步提交。
func (q *SubmissionQueue) Close() {
	q.closeOnce.Do(func() {
		close(q.ch)
	})
}
