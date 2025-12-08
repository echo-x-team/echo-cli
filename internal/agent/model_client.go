package agent

import (
	"context"
	"errors"

	"echo-cli/internal/logger"
)

// ModelClient 定义模型客户端接口
type ModelClient interface {
	Complete(ctx context.Context, messages []Message, model string) (string, error)
	Stream(ctx context.Context, messages []Message, model string, onChunk func(string)) error
}

// EchoClient is a fallback when no API key is available.
type EchoClient struct {
	Prefix string
}

func (c EchoClient) Complete(_ context.Context, messages []Message, _ string) (string, error) {
	if len(messages) == 0 {
		return "", errors.New("no messages to echo")
	}
	last := messages[len(messages)-1]
	return c.Prefix + last.Content, nil
}

func (c EchoClient) Stream(ctx context.Context, messages []Message, model string, onChunk func(string)) error {
	text, err := c.Complete(ctx, messages, model)
	if err != nil {
		return err
	}
	onChunk(text)
	return nil
}

// ToLLMMessages 将内部消息转换为日志友好的结构。
func ToLLMMessages(msgs []Message) []logger.LLMMessage {
	out := make([]logger.LLMMessage, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, logger.LLMMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}
	return out
}
