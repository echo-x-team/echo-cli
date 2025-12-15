package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/prompts"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
)

type Options struct {
	APIKey  string
	BaseURL string
	Model   string
	WireAPI string
}

type Client struct {
	api   *openai.Client
	model string
	wire  string
}

// 确保Client实现了agent.ModelClient接口
var _ agent.ModelClient = (*Client)(nil)

func New(opts Options) (*Client, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	cfg := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
	}
	if base := strings.TrimSpace(opts.BaseURL); base != "" {
		cfg = append(cfg, option.WithBaseURL(strings.TrimRight(normalizeBaseURL(base), "/")))
	}
	client := openai.NewClient(cfg...)

	return &Client{
		api:   &client,
		model: opts.Model,
		wire:  strings.ToLower(strings.TrimSpace(opts.WireAPI)),
	}, nil
}

func (c *Client) resolveModel(model string) string {
	if strings.TrimSpace(model) != "" {
		return model
	}
	return c.model
}

func (c *Client) Complete(ctx context.Context, prompt agent.Prompt) (string, error) {
	if c.wire == "responses" {
		return c.completeResponses(ctx, prompt)
	}
	return c.completeChat(ctx, prompt)
}

func (c *Client) Stream(ctx context.Context, prompt agent.Prompt, onEvent func(agent.StreamEvent)) error {
	if c.wire == "responses" {
		return c.streamResponses(ctx, prompt, onEvent)
	}
	return c.streamChat(ctx, prompt, onEvent)
}

func (c *Client) completeChat(ctx context.Context, prompt agent.Prompt) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(c.resolveModel(prompt.Model)),
		Messages: toChatMessages(prompt.Messages),
	}
	if len(prompt.Tools) > 0 {
		params.Tools = toChatTools(prompt.Tools)
		params.ParallelToolCalls = openai.Bool(prompt.ParallelToolCalls)
	}

	resp, err := c.api.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", wrapHTTPError(err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no completion choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) streamChat(ctx context.Context, prompt agent.Prompt, onEvent func(agent.StreamEvent)) error {
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(c.resolveModel(prompt.Model)),
		Messages: toChatMessages(prompt.Messages),
	}
	if len(prompt.Tools) > 0 {
		params.Tools = toChatTools(prompt.Tools)
		params.ParallelToolCalls = openai.Bool(prompt.ParallelToolCalls)
	}

	stream := c.api.Chat.Completions.NewStreaming(ctx, params)
	collector := newToolCallCollector()

	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: choice.Delta.Content})
			}
			for _, call := range choice.Delta.ToolCalls {
				collector.Add(call.ID, call.Function.Name, call.Function.Arguments)
			}
			if call := choice.Delta.FunctionCall; call.Name != "" || call.Arguments != "" {
				collector.Add("", call.Name, call.Arguments)
			}
			switch choice.FinishReason {
			case "tool_calls", "function_call":
				for _, raw := range collector.Flush() {
					onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: raw})
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return wrapHTTPError(err)
	}
	for _, raw := range collector.Flush() {
		onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: raw})
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

func (c *Client) completeResponses(ctx context.Context, prompt agent.Prompt) (string, error) {
	params := buildResponseParams(prompt, c.resolveModel(prompt.Model))
	resp, err := c.api.Responses.New(ctx, params)
	if err != nil {
		return "", wrapHTTPError(err)
	}
	if resp.Error.Message != "" && resp.Error.JSON.Message.Valid() {
		return "", errors.New(resp.Error.Message)
	}
	if text := extractResponseText(resp); text != "" {
		return text, nil
	}
	return "", errors.New("responses api returned no text")
}

func (c *Client) streamResponses(ctx context.Context, prompt agent.Prompt, onEvent func(agent.StreamEvent)) error {
	params := buildResponseParams(prompt, c.resolveModel(prompt.Model))
	stream := c.api.Responses.NewStreaming(ctx, params)

	var sawText bool
	for stream.Next() {
		event := stream.Current()
		switch v := event.AsAny().(type) {
		case responses.ResponseTextDeltaEvent:
			if v.Delta != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: v.Delta})
				sawText = true
			}
		case responses.ResponseReasoningTextDeltaEvent:
			if v.Delta != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: v.Delta})
				sawText = true
			}
		case responses.ResponseReasoningSummaryTextDeltaEvent:
			if v.Delta != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: v.Delta})
				sawText = true
			}
		case responses.ResponseTextDoneEvent:
			if !sawText && v.Text != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: v.Text})
				sawText = true
			}
		case responses.ResponseOutputItemDoneEvent:
			if raw := strings.TrimSpace(v.Item.RawJSON()); raw != "" {
				onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: json.RawMessage(raw)})
			}
		case responses.ResponseErrorEvent:
			return errors.New(v.Message)
		case responses.ResponseFailedEvent:
			if msg := v.Response.Error.Message; msg != "" {
				return errors.New(msg)
			}
			return errors.New("response failed")
		case responses.ResponseCompletedEvent:
			onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
			return nil
		}
	}
	if err := stream.Err(); err != nil {
		return wrapHTTPError(err)
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

func toChatMessages(msgs []agent.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case agent.RoleSystem:
			out = append(out, openai.SystemMessage(msg.Content))
		case agent.RoleAssistant:
			out = append(out, openai.AssistantMessage(msg.Content))
		default:
			out = append(out, openai.UserMessage(msg.Content))
		}
	}
	return out
}

func toChatTools(specs []agent.ToolSpec) []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		fn := shared.FunctionDefinitionParam{
			Name:       name,
			Parameters: spec.Parameters,
			Strict:     openai.Bool(true),
		}
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			fn.Description = openai.String(desc)
		}
		tools = append(tools, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: fn,
			},
		})
	}
	return tools
}

func toResponseTools(specs []agent.ToolSpec) []responses.ToolUnionParam {
	tools := make([]responses.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		tool := responses.FunctionToolParam{
			Name:       name,
			Parameters: spec.Parameters,
			Strict:     openai.Bool(true),
			Type:       constant.Function("").Default(),
		}
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			tool.Description = openai.String(desc)
		}
		tools = append(tools, responses.ToolUnionParam{OfFunction: &tool})
	}
	return tools
}

func buildResponseParams(prompt agent.Prompt, model string) responses.ResponseNewParams {
	instructions, convo := splitInstructions(prompt.Messages)
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if len(convo) > 0 {
		params.Input.OfInputItemList = responses.ResponseInputParam(toResponseInput(convo))
	}
	if len(prompt.Tools) > 0 {
		params.Tools = toResponseTools(prompt.Tools)
		params.ParallelToolCalls = openai.Bool(prompt.ParallelToolCalls)
	}
	if effort := prompts.ExtractReasoningEffort(instructions); effort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(effort)}
	}
	if schema, ok := parseOutputSchema(prompt.OutputSchema); ok {
		var format responses.ResponseFormatTextJSONSchemaConfigParam
		format.Name = "echo_output_schema"
		format.Schema = schema
		format.Strict = openai.Bool(true)
		format.Type = constant.JSONSchema("").Default()

		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &format},
		}
	}
	return params
}

func parseOutputSchema(schema string) (map[string]any, bool) {
	raw := strings.TrimSpace(schema)
	if raw == "" {
		return nil, false
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func wrapHTTPError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr != nil {
		respDump := strings.TrimSpace(string(apiErr.DumpResponse(true)))
		if respDump != "" {
			return fmt.Errorf("http_%d: %s", apiErr.StatusCode, respDump)
		}
		raw := strings.TrimSpace(apiErr.RawJSON())
		if raw != "" {
			return fmt.Errorf("http_%d: %s", apiErr.StatusCode, raw)
		}
		return fmt.Errorf("http_%d: %v", apiErr.StatusCode, err)
	}
	return err
}

func toResponseInput(msgs []agent.Message) []responses.ResponseInputItemUnionParam {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(msgs))
	for _, msg := range msgs {
		items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, toResponseRole(msg.Role)))
	}
	return items
}

func toResponseRole(role agent.Role) responses.EasyInputMessageRole {
	switch role {
	case agent.RoleAssistant:
		return responses.EasyInputMessageRoleAssistant
	case agent.RoleSystem:
		return responses.EasyInputMessageRoleSystem
	default:
		return responses.EasyInputMessageRoleUser
	}
}

func splitInstructions(messages []agent.Message) (string, []agent.Message) {
	var instructions []string
	convo := make([]agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == agent.RoleSystem {
			instructions = append(instructions, strings.TrimSpace(msg.Content))
			continue
		}
		convo = append(convo, msg)
	}
	return strings.Join(instructions, "\n\n"), convo
}

func extractResponseText(resp *responses.Response) string {
	if resp == nil {
		return ""
	}
	if text := strings.TrimSpace(resp.OutputText()); text != "" {
		return text
	}
	for _, item := range resp.Output {
		for _, content := range item.Content {
			if text := strings.TrimSpace(content.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

type toolCallCollector struct {
	calls map[string]*pendingToolCall
}

type pendingToolCall struct {
	ID   string
	Name string
	Args strings.Builder
}

func newToolCallCollector() *toolCallCollector {
	return &toolCallCollector{
		calls: make(map[string]*pendingToolCall),
	}
}

func (c *toolCallCollector) Add(id, name, args string) {
	if strings.TrimSpace(id) == "" && strings.TrimSpace(name) == "" {
		return
	}
	callID := id
	if callID == "" {
		callID = fmt.Sprintf("call-%d", len(c.calls)+1)
	}
	entry := c.calls[callID]
	if entry == nil {
		entry = &pendingToolCall{ID: callID}
		c.calls[callID] = entry
	}
	if name != "" {
		entry.Name = name
	}
	if args != "" {
		entry.Args.WriteString(args)
	}
}

func (c *toolCallCollector) Flush() []json.RawMessage {
	if len(c.calls) == 0 {
		return nil
	}
	out := make([]json.RawMessage, 0, len(c.calls))
	for _, call := range c.calls {
		if call == nil || strings.TrimSpace(call.Name) == "" {
			continue
		}
		raw := encodeFunctionCallItem(call.ID, call.Name, call.Args.String())
		if len(raw) > 0 {
			out = append(out, raw)
		}
	}
	c.calls = make(map[string]*pendingToolCall)
	return out
}

func encodeFunctionCallItem(id, name, args string) json.RawMessage {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	payload := map[string]any{
		"type":      "function_call",
		"id":        id,
		"call_id":   id,
		"name":      name,
		"arguments": strings.TrimSpace(args),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}
