package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"echo-cli/internal/logger"
	"github.com/google/uuid"
)

// Handler 处理 Submission 并通过 EventPublisher 发出事件。
type Handler interface {
	Handle(ctx context.Context, submission Submission, emit EventPublisher) error
}

// HandlerFunc 让函数实现 Handler。
type HandlerFunc func(ctx context.Context, submission Submission, emit EventPublisher) error

func (f HandlerFunc) Handle(ctx context.Context, submission Submission, emit EventPublisher) error {
	return f(ctx, submission, emit)
}

// EventPublisher 抽象 EQ，便于解耦。
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

// ManagerConfig 定义事件管理器参数。
type ManagerConfig struct {
	SubmissionBuffer int
	EventBuffer      int
	Workers          int
	SQLogPath        string
	EQLogPath        string
}

func (cfg ManagerConfig) withDefaults() ManagerConfig {
	if cfg.SubmissionBuffer == 0 {
		cfg.SubmissionBuffer = 64
	}
	if cfg.EventBuffer == 0 {
		cfg.EventBuffer = 128
	}
	if cfg.Workers == 0 {
		cfg.Workers = 1
	}
	if cfg.SQLogPath == "" {
		cfg.SQLogPath = DefaultSQLogPath
	}
	if cfg.EQLogPath == "" {
		cfg.EQLogPath = DefaultEQLogPath
	}
	return cfg
}

// Manager 协调 SQ/EQ，管理用户输入与智能体输出。
type Manager struct {
	queue    *SubmissionQueue
	events   *EventQueue
	handlers map[OperationKind]Handler
	hmu      sync.RWMutex
	workers  int

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	sqLog       *logger.LogEntry
	eqLog       *logger.LogEntry
	sqLogCloser io.Closer
	eqLogCloser io.Closer
}

// NewManager 创建新的事件管理器。
func NewManager(cfg ManagerConfig) *Manager {
	cfg = cfg.withDefaults()

	sqLog, sqCloser := newQueueLogger("sq", cfg.SQLogPath)
	eqLog, eqCloser := newQueueLogger("eq", cfg.EQLogPath)

	queue := NewSubmissionQueue(cfg.SubmissionBuffer)
	queue.SetLogger(sqLog)
	events := NewEventQueue(cfg.EventBuffer)
	events.SetLogger(eqLog)

	return &Manager{
		queue:       queue,
		events:      events,
		handlers:    map[OperationKind]Handler{},
		workers:     cfg.Workers,
		sqLog:       sqLog,
		eqLog:       eqLog,
		sqLogCloser: sqCloser,
		eqLogCloser: eqCloser,
	}
}

// RegisterHandler 为指定 OperationKind 注册处理器。
func (m *Manager) RegisterHandler(kind OperationKind, handler Handler) {
	if handler == nil {
		return
	}
	m.hmu.Lock()
	m.handlers[kind] = handler
	m.hmu.Unlock()
}

// Start 启动后台 worker。
func (m *Manager) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		m.cancel = cancel
		for i := 0; i < m.workers; i++ {
			m.wg.Add(1)
			go m.worker(runCtx)
		}
	})
}

// Close 停止队列和 worker，并关闭 EQ。
func (m *Manager) Close() {
	m.stopOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		m.queue.Close()
		m.wg.Wait()
		m.events.Close()
		if m.sqLogCloser != nil {
			_ = m.sqLogCloser.Close()
		}
		if m.eqLogCloser != nil {
			_ = m.eqLogCloser.Close()
		}
	})
}

// Subscribe 订阅事件。
func (m *Manager) Subscribe() <-chan Event {
	return m.events.Subscribe()
}

// SubmitUserInput 将用户输入放入 SQ。
func (m *Manager) SubmitUserInput(ctx context.Context, items []InputMessage, inputCtx InputContext) (string, error) {
	if len(items) == 0 {
		return "", errors.New("empty user input items")
	}
	meta := cloneMetadata(inputCtx.Metadata)
	sub := Submission{
		ID:        uuid.NewString(),
		Operation: Operation{Kind: OperationUserInput, UserInput: &UserInputOperation{Items: items, Context: inputCtx}},
		Timestamp: time.Now(),
		Priority:  PriorityNormal,
		SessionID: inputCtx.SessionID,
		Metadata:  meta,
	}
	return m.Submit(ctx, sub)
}

// Submit 将 Submission 放入 SQ。
func (m *Manager) Submit(ctx context.Context, submission Submission) (string, error) {
	if submission.ID == "" {
		submission.ID = uuid.NewString()
	}
	if submission.Timestamp.IsZero() {
		submission.Timestamp = time.Now()
	}
	if submission.Priority == 0 {
		submission.Priority = PriorityNormal
	}
	if submission.Operation.Kind == "" {
		return "", errors.New("submission operation kind required")
	}
	if err := m.queue.Submit(ctx, submission); err != nil {
		return "", err
	}
	_ = m.events.Publish(ctx, Event{
		Type:         EventSubmissionAccepted,
		SubmissionID: submission.ID,
		SessionID:    submission.SessionID,
		Timestamp:    time.Now(),
		Payload:      submission.Operation,
		Metadata:     submission.Metadata,
	})
	return submission.ID, nil
}

// PublishEvent 允许外部模块向 EQ 直接发布事件。
func (m *Manager) PublishEvent(ctx context.Context, event Event) error {
	return m.events.Publish(ctx, event)
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	for {
		sub, err := m.queue.Receive(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, ErrSubmissionQueueClosed) {
				return
			}
			continue
		}
		_ = m.events.Publish(ctx, Event{
			Type:         EventTaskStarted,
			SubmissionID: sub.ID,
			SessionID:    sub.SessionID,
			Timestamp:    time.Now(),
			Payload:      sub.Operation.Kind,
			Metadata:     sub.Metadata,
		})
		m.hmu.RLock()
		handler := m.handlers[sub.Operation.Kind]
		m.hmu.RUnlock()
		if handler == nil {
			msg := fmt.Sprintf("no handler registered for %s", sub.Operation.Kind)
			_ = m.events.Publish(ctx, Event{
				Type:         EventError,
				SubmissionID: sub.ID,
				SessionID:    sub.SessionID,
				Timestamp:    time.Now(),
				Payload:      msg,
				Metadata:     sub.Metadata,
			})
			_ = m.events.Publish(ctx, Event{
				Type:         EventTaskCompleted,
				SubmissionID: sub.ID,
				SessionID:    sub.SessionID,
				Timestamp:    time.Now(),
				Payload:      TaskResult{Status: "failed", Error: msg},
				Metadata:     sub.Metadata,
			})
			continue
		}
		if err := handler.Handle(ctx, sub, m.events); err != nil {
			_ = m.events.Publish(ctx, Event{
				Type:         EventError,
				SubmissionID: sub.ID,
				SessionID:    sub.SessionID,
				Timestamp:    time.Now(),
				Payload:      err.Error(),
				Metadata:     sub.Metadata,
			})
			_ = m.events.Publish(ctx, Event{
				Type:         EventTaskCompleted,
				SubmissionID: sub.ID,
				SessionID:    sub.SessionID,
				Timestamp:    time.Now(),
				Payload:      TaskResult{Status: "failed", Error: err.Error()},
				Metadata:     sub.Metadata,
			})
			continue
		}
		_ = m.events.Publish(ctx, Event{
			Type:         EventTaskCompleted,
			SubmissionID: sub.ID,
			SessionID:    sub.SessionID,
			Timestamp:    time.Now(),
			Payload:      TaskResult{Status: "completed"},
			Metadata:     sub.Metadata,
		})
	}
}

func cloneMetadata(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}
