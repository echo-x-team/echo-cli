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
	api       *anthropic.Client
	model     string
	newStream func(ctx context.Context, params anthropic.MessageNewParams) messageStream
}

var _ agent.ModelClient = (*Client)(nil)
var streamLog = logger.Named("llm")

type messageStream interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
}

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
		newStream: func(ctx context.Context, params anthropic.MessageNewParams) messageStream {
			return client.Messages.NewStreaming(ctx, params)
		},
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
	model := string(c.resolveModel(prompt.Model))
	return c.streamOnce(ctx, params, model, onEvent)
}

func (c *Client) streamOnce(ctx context.Context, params anthropic.MessageNewParams, model string, onEvent func(agent.StreamEvent)) error {
	var stream messageStream
	if c.newStream != nil {
		stream = c.newStream(ctx, params)
	} else {
		stream = c.api.Messages.NewStreaming(ctx, params)
	}
	state := newToolUseStreamState()
	summary := streamSummary{}
	logSummary := newStreamSummaryLogger(model, &summary)
	rawCapture := newRawStreamCapture()
	emittedText := false
	emittedItem := false
	streaming := false
	buffered := make([]agent.StreamEvent, 0, 8)
	wrappedOnEvent := func(evt agent.StreamEvent) {
		switch evt.Type {
		case agent.StreamEventTextDelta:
			if evt.Text != "" {
				emittedText = true
			}
		case agent.StreamEventItem:
			if len(evt.Item) > 0 {
				emittedItem = true
			}
		}
		if evt.Type == agent.StreamEventCompleted {
			evt.StopReason = summary.stopReason
			evt.StopSequence = summary.stopSequence
			evt.FinishReason = resolveFinishReason(&summary)
		}
		if streaming {
			onEvent(evt)
			return
		}
		buffered = append(buffered, evt)
		if (evt.Type == agent.StreamEventTextDelta && evt.Text != "") || (evt.Type == agent.StreamEventItem && len(evt.Item) > 0) {
			streaming = true
			for _, queued := range buffered {
				onEvent(queued)
			}
			buffered = nil
		}
	}

	for stream.Next() {
		event := stream.Current()
		variant := event.AsAny()
		raw := strings.TrimSpace(event.RawJSON())
		if raw == "" {
			if fallback, err := json.Marshal(variant); err == nil {
				raw = string(fallback)
			}
		}
		rawCapture.Add(raw)
		switch v := variant.(type) {
		case anthropic.MessageStartEvent:
			summary.bumpCount("message_start")
			if v.Message.ID != "" {
				summary.messageID = v.Message.ID
			}
			if v.Message.StopReason != "" {
				summary.stopReason = string(v.Message.StopReason)
			}
			if v.Message.StopSequence != "" {
				summary.stopSequence = v.Message.StopSequence
			}
			if len(v.Message.Content) > 0 {
				summary.rawDeltaHasContent = true
				for _, block := range v.Message.Content {
					summary.recordContentBlock(block.AsAny())
				}
			}
		case anthropic.MessageDeltaEvent:
			summary.bumpCount("message_delta")
			if v.Delta.StopReason != "" {
				summary.stopReason = string(v.Delta.StopReason)
			}
			if v.Delta.StopSequence != "" {
				summary.stopSequence = v.Delta.StopSequence
			}
		case anthropic.ContentBlockStartEvent:
			summary.bumpCount("content_block_start")
			summary.recordContentBlock(v.ContentBlock.AsAny())
		case anthropic.ContentBlockDeltaEvent:
			summary.bumpCount("content_block_delta")
			switch d := v.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				summary.bumpCount("text_delta")
				if d.Text != "" {
					summary.rawDeltaHasContent = true
				}
			case anthropic.InputJSONDelta:
				summary.bumpCount("input_json_delta")
				if d.PartialJSON != "" {
					summary.rawDeltaHasContent = true
				}
			case anthropic.CitationsDelta:
				summary.bumpCount("citations_delta")
				summary.rawDeltaHasContent = true
			case anthropic.ThinkingDelta:
				summary.bumpCount("thinking_delta")
				if d.Thinking != "" {
					summary.rawDeltaHasContent = true
				}
			case anthropic.SignatureDelta:
				summary.bumpCount("signature_delta")
				if d.Signature != "" {
					summary.rawDeltaHasContent = true
				}
			}
		case anthropic.ContentBlockStopEvent:
			summary.bumpCount("content_block_stop")
		case anthropic.MessageStopEvent:
			summary.bumpCount("message_stop")
			if summary.finishReason == "" {
				summary.finishReason = "message_stop"
			}
		default:
			summary.bumpCount("unknown_event")
		}
		if state.Handle(variant, wrappedOnEvent) {
			logSummary(nil)
			rawCapture.LogIfEmpty(model, emittedText, emittedItem)
			return nil
		}
	}
	if err := stream.Err(); err != nil {
		logSummary(err)
		return err
	}
	state.Flush(wrappedOnEvent)
	wrappedOnEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	logSummary(nil)
	rawCapture.LogIfEmpty(model, emittedText, emittedItem)
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
	messageID          string
	eventCounts        map[string]int
}

func (s *streamSummary) bumpCount(key string) {
	if s == nil || key == "" {
		return
	}
	if s.eventCounts == nil {
		s.eventCounts = make(map[string]int)
	}
	s.eventCounts[key]++
}

func (s *streamSummary) recordContentBlock(block any) {
	switch b := block.(type) {
	case anthropic.TextBlock:
		s.bumpCount("content_block_text")
		if b.Text != "" || len(b.Citations) > 0 {
			s.rawDeltaHasContent = true
		}
	case anthropic.ThinkingBlock:
		s.bumpCount("content_block_thinking")
		if b.Thinking != "" || b.Signature != "" {
			s.rawDeltaHasContent = true
		}
	case anthropic.RedactedThinkingBlock:
		s.bumpCount("content_block_redacted_thinking")
		if b.Data != "" {
			s.rawDeltaHasContent = true
		}
	case anthropic.ToolUseBlock:
		s.bumpCount("content_block_tool_use")
		s.rawDeltaHasContent = true
	case anthropic.ServerToolUseBlock:
		s.bumpCount("content_block_server_tool_use")
		s.rawDeltaHasContent = true
	case anthropic.WebSearchToolResultBlock:
		s.bumpCount("content_block_web_search_result")
		s.rawDeltaHasContent = true
	default:
		s.bumpCount("content_block_unknown")
	}
}

type rawStreamCapture struct {
	events    []string
	bytes     int
	truncated bool
}

const (
	rawStreamMaxEvents = 64
	rawStreamMaxBytes  = 16 * 1024
)

func newRawStreamCapture() *rawStreamCapture {
	return &rawStreamCapture{}
}

func (c *rawStreamCapture) Add(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || c.truncated {
		return
	}
	raw = sanitizeRawLogText(raw)
	if len(c.events) >= rawStreamMaxEvents || c.bytes+len(raw) > rawStreamMaxBytes {
		c.truncated = true
		return
	}
	c.events = append(c.events, raw)
	c.bytes += len(raw)
}

func (c *rawStreamCapture) LogIfEmpty(model string, emittedText bool, emittedItem bool) {
	if emittedText || emittedItem {
		return
	}
	fields := logger.Fields{
		"model":              model,
		"raw_event_count":    len(c.events),
		"raw_event_bytes":    c.bytes,
		"raw_events":         c.events,
		"raw_events_limited": c.truncated,
	}
	streamLog.WithFields(fields).Info("llm stream empty response raw events")
}

func sanitizeRawLogText(text string) string {
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
}

func newStreamSummaryLogger(model string, summary *streamSummary) func(error) {
	logged := false
	return func(err error) {
		if logged {
			return
		}
		logged = true
		finishReason := resolveFinishReason(summary)
		fields := logger.Fields{
			"model":                 model,
			"stop_reason":           summary.stopReason,
			"stop_sequence":         summary.stopSequence,
			"finish_reason":         finishReason,
			"raw_delta_has_content": summary.rawDeltaHasContent,
		}
		if summary.messageID != "" {
			fields["message_id"] = summary.messageID
		}
		if len(summary.eventCounts) > 0 {
			fields["event_counts"] = summary.eventCounts
		}
		if err != nil {
			fields["stream_error"] = err.Error()
		}
		streamLog.WithFields(fields).Info("llm stream summary")
	}
}

func resolveFinishReason(summary *streamSummary) string {
	if summary == nil {
		return ""
	}
	if summary.finishReason != "" {
		return summary.finishReason
	}
	if summary.stopReason != "" {
		return summary.stopReason
	}
	return ""
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
