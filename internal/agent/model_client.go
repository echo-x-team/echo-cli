package agent

import (
	"context"
	"errors"
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
