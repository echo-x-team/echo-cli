package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"echo-cli/internal/agent"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Options struct {
	Token   string
	BaseURL string
	Model   string
}

type Client struct {
	api   *anthropic.Client
	model string
}

var _ agent.ModelClient = (*Client)(nil)

func New(opts Options) (*Client, error) {
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return nil, errors.New("missing token")
	}
	reqOpts := []option.RequestOption{
		option.WithAPIKey(token),
	}
	if base := normalizeBaseURL(opts.BaseURL); base != "" {
		reqOpts = append(reqOpts, option.WithBaseURL(base))
	}
	client := anthropic.NewClient(reqOpts...)
	return &Client{
		api:   &client,
		model: strings.TrimSpace(opts.Model),
	}, nil
}

func normalizeBaseURL(raw string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		return ""
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
		base = strings.TrimRight(base, "/")
	}
	return base
}

func (c *Client) resolveModel(m string) anthropic.Model {
	if strings.TrimSpace(m) != "" {
		return anthropic.Model(strings.TrimSpace(m))
	}
	return anthropic.Model(c.model)
}

func (c *Client) Complete(ctx context.Context, prompt agent.Prompt) (string, error) {
	params := buildMessageParams(prompt, c.resolveModel(prompt.Model))
	msg, err := c.api.Messages.New(ctx, params)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(extractText(msg.Content)), nil
}

func (c *Client) Stream(ctx context.Context, prompt agent.Prompt, onEvent func(agent.StreamEvent)) error {
	params := buildMessageParams(prompt, c.resolveModel(prompt.Model))
	stream := c.api.Messages.NewStreaming(ctx, params)

	for stream.Next() {
		event := stream.Current()
		switch v := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch d := v.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if d.Text != "" {
					onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: d.Text})
				}
			}
		case anthropic.ContentBlockStartEvent:
			switch b := v.ContentBlock.AsAny().(type) {
			case anthropic.ToolUseBlock:
				raw := toolUseToFunctionCallItem(b)
				if len(raw) > 0 {
					onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: raw})
				}
			}
		case anthropic.MessageStopEvent:
			onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
			return nil
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

func buildMessageParams(prompt agent.Prompt, model anthropic.Model) anthropic.MessageNewParams {
	var system []anthropic.TextBlockParam
	var messages []anthropic.MessageParam

	for _, msg := range prompt.Messages {
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		switch msg.Role {
		case agent.RoleSystem:
			system = append(system, anthropic.TextBlockParam{Text: text})
		case agent.RoleAssistant:
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(text)))
		default:
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
		}
	}

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 1024,
		Messages:  messages,
	}
	if len(system) > 0 {
		params.System = system
	}
	return params
}

func extractText(blocks []anthropic.ContentBlockUnion) string {
	var sb strings.Builder
	for _, block := range blocks {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			sb.WriteString(v.Text)
		}
	}
	return sb.String()
}

func toolUseToFunctionCallItem(block anthropic.ToolUseBlock) json.RawMessage {
	args := strings.TrimSpace(string(block.Input))
	if args == "" {
		args = "{}"
	}
	payload := map[string]any{
		"type":      "function_call",
		"name":      block.Name,
		"arguments": args,
		"call_id":   block.ID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}
