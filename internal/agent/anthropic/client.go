package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/logger"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
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
var streamLog = logger.Named("llm")

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
	state := newToolUseStreamState()
	model := string(c.resolveModel(prompt.Model))
	summary := streamSummary{}
	logSummary := newStreamSummaryLogger(model, &summary)

	for stream.Next() {
		event := stream.Current()
		variant := event.AsAny()
		switch v := variant.(type) {
		case anthropic.MessageDeltaEvent:
			if v.Delta.StopReason != "" {
				summary.stopReason = string(v.Delta.StopReason)
			}
			if v.Delta.StopSequence != "" {
				summary.stopSequence = v.Delta.StopSequence
			}
		case anthropic.ContentBlockDeltaEvent:
			switch d := v.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if d.Text != "" {
					summary.rawDeltaHasContent = true
				}
			}
		case anthropic.MessageStopEvent:
			if summary.finishReason == "" {
				summary.finishReason = "message_stop"
			}
		}
		if state.Handle(variant, onEvent) {
			logSummary(nil)
			return nil
		}
	}
	if err := stream.Err(); err != nil {
		logSummary(err)
		return err
	}
	state.Flush(onEvent)
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	logSummary(nil)
	return nil
}

func buildMessageParams(prompt agent.Prompt, model anthropic.Model) anthropic.MessageNewParams {
	var system []anthropic.TextBlockParam
	var messages []anthropic.MessageParam

	for _, msg := range prompt.Messages {
		switch msg.Role {
		case agent.RoleSystem:
			text := strings.TrimSpace(msg.Content)
			if text == "" {
				continue
			}
			system = append(system, anthropic.TextBlockParam{Text: text})
		case agent.RoleAssistant:
			blocks := messageBlocks(msg)
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, anthropic.NewAssistantMessage(blocks...))
		default:
			blocks := messageBlocks(msg)
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, anthropic.NewUserMessage(blocks...))
		}
	}

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 1024,
		Messages:  messages,
		Tools:     toolSpecsToParams(prompt.Tools),
	}
	if len(system) > 0 {
		params.System = system
	}
	return params
}

func messageBlocks(msg agent.Message) []anthropic.ContentBlockParamUnion {
	if msg.ToolResult != nil && msg.ToolResult.ToolUseID != "" {
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewToolResultBlock(
				msg.ToolResult.ToolUseID,
				msg.ToolResult.Content,
				msg.ToolResult.IsError,
			),
		}
	}
	if msg.ToolUse != nil && msg.ToolUse.ID != "" && msg.ToolUse.Name != "" {
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewToolUseBlock(msg.ToolUse.ID, msg.ToolUse.Input, msg.ToolUse.Name),
		}
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		return nil
	}
	return []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(text)}
}

func toolSpecsToParams(specs []agent.ToolSpec) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		param, err := toolSpecToToolParam(spec)
		if err != nil {
			continue
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &param})
	}
	return out
}

func toolSpecToToolParam(spec agent.ToolSpec) (anthropic.ToolParam, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return anthropic.ToolParam{}, errors.New("missing tool name")
	}
	props := spec.Parameters["properties"]
	rawRequired := spec.Parameters["required"]
	required, err := schemaStrings(rawRequired)
	if err != nil {
		return anthropic.ToolParam{}, fmt.Errorf("tool %q schema.required: %w", name, err)
	}

	schema := anthropic.ToolInputSchemaParam{
		Type:       constant.Object("object"),
		Properties: props,
		Required:   required,
	}
	if additional, ok := spec.Parameters["additionalProperties"]; ok {
		if schema.ExtraFields == nil {
			schema.ExtraFields = map[string]any{}
		}
		schema.ExtraFields["additionalProperties"] = additional
	}

	tool := anthropic.ToolParam{
		Name:        name,
		InputSchema: schema,
		Type:        anthropic.ToolTypeCustom,
	}
	if desc := strings.TrimSpace(spec.Description); desc != "" {
		tool.Description = anthropic.String(desc)
	}
	return tool, nil
}

func schemaStrings(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item[%d] type %T", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", value)
	}
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

type pendingToolUse struct {
	id        string
	name      string
	startArgs json.RawMessage
	partial   strings.Builder
}

type toolUseStreamState struct {
	pending map[int64]*pendingToolUse
}

type streamSummary struct {
	stopReason         string
	stopSequence       string
	finishReason       string
	rawDeltaHasContent bool
}

func newStreamSummaryLogger(model string, summary *streamSummary) func(error) {
	logged := false
	return func(err error) {
		if logged {
			return
		}
		logged = true
		finishReason := summary.finishReason
		if finishReason == "" && summary.stopReason != "" {
			finishReason = summary.stopReason
		}
		fields := logger.Fields{
			"model":                 model,
			"stop_reason":           summary.stopReason,
			"stop_sequence":         summary.stopSequence,
			"finish_reason":         finishReason,
			"raw_delta_has_content": summary.rawDeltaHasContent,
		}
		if err != nil {
			fields["stream_error"] = err.Error()
		}
		streamLog.WithFields(fields).Info("llm stream summary")
	}
}

func newToolUseStreamState() *toolUseStreamState {
	return &toolUseStreamState{pending: make(map[int64]*pendingToolUse)}
}

func (s *toolUseStreamState) Handle(event any, onEvent func(agent.StreamEvent)) (completed bool) {
	switch v := event.(type) {
	case anthropic.MessageDeltaEvent:
		onEvent(agent.StreamEvent{
			Type: agent.StreamEventUsage,
			Usage: &agent.TokenUsage{
				InputTokens:              v.Usage.InputTokens,
				OutputTokens:             v.Usage.OutputTokens,
				CacheCreationInputTokens: v.Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     v.Usage.CacheReadInputTokens,
			},
		})
	case anthropic.ContentBlockStartEvent:
		switch b := v.ContentBlock.AsAny().(type) {
		case anthropic.ToolUseBlock:
			s.pending[v.Index] = &pendingToolUse{
				id:        b.ID,
				name:      b.Name,
				startArgs: b.Input,
			}
		}
	case anthropic.ContentBlockDeltaEvent:
		switch d := v.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			if d.Text != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: d.Text})
			}
		case anthropic.InputJSONDelta:
			if pending := s.pending[v.Index]; pending != nil {
				pending.partial.WriteString(d.PartialJSON)
			}
		}
	case anthropic.ContentBlockStopEvent:
		s.flushIndex(v.Index, onEvent)
	case anthropic.MessageStopEvent:
		s.Flush(onEvent)
		onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
		return true
	}
	return false
}

func (s *toolUseStreamState) Flush(onEvent func(agent.StreamEvent)) {
	if len(s.pending) == 0 {
		return
	}
	indexes := make([]int64, 0, len(s.pending))
	for idx := range s.pending {
		indexes = append(indexes, idx)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })
	for _, idx := range indexes {
		s.flushIndex(idx, onEvent)
	}
}

func (s *toolUseStreamState) flushIndex(idx int64, onEvent func(agent.StreamEvent)) {
	pending := s.pending[idx]
	if pending == nil {
		return
	}
	delete(s.pending, idx)

	args := strings.TrimSpace(pending.partial.String())
	if args == "" {
		args = strings.TrimSpace(string(pending.startArgs))
	}

	raw := functionCallItem(pending.name, pending.id, args)
	if len(raw) == 0 {
		return
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: raw})
}

func functionCallItem(name, callID, args string) json.RawMessage {
	args = strings.TrimSpace(args)
	if args == "" {
		args = "{}"
	}
	payload := map[string]any{
		"type":      "function_call",
		"name":      name,
		"arguments": args,
		"call_id":   callID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}
