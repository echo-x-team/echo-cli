package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// ModelClient 定义模型客户端接口
type ModelClient interface {
	Complete(ctx context.Context, prompt Prompt) (string, error)
	Stream(ctx context.Context, prompt Prompt, onEvent func(StreamEvent)) error
}

// EchoClient is a fallback when no API key is available.
type EchoClient struct {
	Prefix string
}

func (c EchoClient) Complete(_ context.Context, prompt Prompt) (string, error) {
	if len(prompt.Messages) == 0 {
		return "", errors.New("no messages to echo")
	}
	last := prompt.Messages[len(prompt.Messages)-1]
	return c.Prefix + last.Content, nil
}

func (c EchoClient) Stream(ctx context.Context, prompt Prompt, onEvent func(StreamEvent)) error {
	text, err := c.Complete(ctx, prompt)
	if err != nil {
		return err
	}
	onEvent(StreamEvent{Type: StreamEventTextDelta, Text: text})
	onEvent(StreamEvent{Type: StreamEventCompleted})
	return nil
}

// StreamEventType 表示流式事件类型，对齐 Responses API 的语义。
type StreamEventType string

const (
	StreamEventTextDelta StreamEventType = "text_delta"
	StreamEventItem      StreamEventType = "item_done"
	StreamEventCompleted StreamEventType = "completed"
	StreamEventUsage     StreamEventType = "usage"
)

// StreamEvent 统一描述模型流式返回的结构化事件或文本增量。
// Item 使用 RawMessage 以避免 agent 包与 execution 包循环依赖。
type StreamEvent struct {
	Type StreamEventType
	Text string
	Item json.RawMessage
	// Usage 携带模型返回的 token 统计（如有）。
	Usage *TokenUsage
	// StopReason/StopSequence/FinishReason 记录流式完成原因（如有）。
	StopReason   string
	StopSequence string
	FinishReason string
}

// TokenUsage 描述一次请求的 token 使用情况。
type TokenUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}
