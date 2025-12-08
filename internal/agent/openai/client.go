package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/prompts"

	"github.com/sashabaranov/go-openai"
)

type Options struct {
	APIKey  string
	BaseURL string
	Model   string
	WireAPI string
}

type Client struct {
	api        *openai.Client
	model      string
	wire       string
	baseURL    string
	apiKey     string
	httpClient openai.HTTPDoer
}

func New(opts Options) (*Client, error) {
	if opts.APIKey == "" {
		return nil, errors.New("missing OPENAI_API_KEY")
	}
	cfg := openai.DefaultConfig(opts.APIKey)
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	return &Client{
		api:        openai.NewClientWithConfig(cfg),
		model:      opts.Model,
		wire:       strings.ToLower(strings.TrimSpace(opts.WireAPI)),
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     opts.APIKey,
		httpClient: cfg.HTTPClient,
	}, nil
}

// 确保Client实现了agent.ModelClient接口
var _ agent.ModelClient = (*Client)(nil)

func (c *Client) Complete(ctx context.Context, messages []agent.Message, model string) (string, error) {
	if c.wire == "responses" {
		return c.completeResponses(ctx, messages, model)
	}
	useModel := c.model
	if model != "" {
		useModel = model
	}
	req := openai.ChatCompletionRequest{
		Model:    useModel,
		Messages: toWireMessages(messages),
		Stream:   false,
	}
	resp, err := c.api.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no completion choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) Stream(ctx context.Context, messages []agent.Message, model string, onChunk func(string)) error {
	if c.wire == "responses" {
		return c.streamResponses(ctx, messages, model, onChunk)
	}
	useModel := c.model
	if model != "" {
		useModel = model
	}
	req := openai.ChatCompletionRequest{
		Model:    useModel,
		Messages: toWireMessages(messages),
		Stream:   true,
	}
	stream, err := c.api.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return err
	}
	defer stream.Close()
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, choice := range response.Choices {
			onChunk(choice.Delta.Content)
		}
	}
}

func toWireMessages(msgs []agent.Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, openai.ChatCompletionMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}
	return out
}

func (c *Client) httpDo(req *http.Request) (*http.Response, error) {
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (c *Client) completeResponses(ctx context.Context, messages []agent.Message, model string) (string, error) {
	useModel := c.model
	if model != "" {
		useModel = model
	}
	reqPayload := buildResponsesRequest(messages, useModel, false)
	endpoint := strings.TrimRight(c.baseURL, "/") + "/responses"
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpDo(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("responses api error: status=%d body=%s", resp.StatusCode, string(data))
	}
	var decoded responsesResponsePayload
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", errors.New(decoded.Error.Message)
	}
	if text := decoded.OutputText; text != "" {
		return text, nil
	}
	for _, out := range decoded.Output {
		for _, content := range out.Content {
			if content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("responses api returned no text")
}

func (c *Client) streamResponses(ctx context.Context, messages []agent.Message, model string, onChunk func(string)) error {
	useModel := c.model
	if model != "" {
		useModel = model
	}
	reqPayload := buildResponsesRequest(messages, useModel, true)
	endpoint := strings.TrimRight(c.baseURL, "/") + "/responses"
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("responses api error: status=%d body=%s", resp.StatusCode, string(data))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var sawText bool
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if strings.TrimSpace(line) != "" {
			continue
		}
		if len(dataLines) == 0 {
			continue
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		done, err := c.handleResponsesEvent(data, onChunk, &sawText)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if len(dataLines) > 0 {
		data := strings.Join(dataLines, "\n")
		done, err := c.handleResponsesEvent(data, onChunk, &sawText)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (c *Client) handleResponsesEvent(data string, onChunk func(string), sawText *bool) (bool, error) {
	payload := strings.TrimSpace(data)
	if payload == "" {
		return false, nil
	}
	if payload == "[DONE]" {
		return true, nil
	}
	var event responsesSSE
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return false, err
	}
	if event.Error != nil && event.Error.Message != "" {
		return false, errors.New(event.Error.Message)
	}
	if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
		return false, errors.New(event.Response.Error.Message)
	}
	text := extractResponsesText(event)
	if text != "" && !(event.Type == "response.completed" && *sawText) {
		onChunk(text)
		*sawText = true
	}
	switch event.Type {
	case "response.output_text.delta":
	case "response.completed":
		return true, nil
	}
	return false, nil
}

type responsesRequest struct {
	Model        string                 `json:"model"`
	Instructions string                 `json:"instructions,omitempty"`
	Input        []responsesMessage     `json:"input"`
	Stream       bool                   `json:"stream"`
	Reasoning    map[string]string      `json:"reasoning,omitempty"`
	Extra        map[string]interface{} `json:"-"`
}

type responsesMessage struct {
	Role    string              `json:"role"`
	Content []responsesFragment `json:"content"`
}

type responsesFragment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesSSE struct {
	Type       string                 `json:"type"`
	Delta      string                 `json:"delta"`
	Output     []responsesOutputBlock `json:"output"`
	OutputText string                 `json:"output_text"`
	Response   *responsesResponseBody `json:"response"`
	Item       *responsesOutputBlock  `json:"item"`
	Error      *responsesError        `json:"error"`
}

type responsesResponseBody struct {
	OutputText string                 `json:"output_text"`
	Output     []responsesOutputBlock `json:"output"`
	Error      *responsesError        `json:"error"`
}

type responsesOutputBlock struct {
	Content []responsesFragment `json:"content"`
}

type responsesError struct {
	Message string `json:"message"`
}

type responsesResponsePayload struct {
	OutputText string                 `json:"output_text"`
	Output     []responsesOutputBlock `json:"output"`
	Error      *responsesError        `json:"error"`
}

func extractResponsesText(event responsesSSE) string {
	if event.Delta != "" {
		return event.Delta
	}
	if event.OutputText != "" {
		return event.OutputText
	}
	if event.Response != nil {
		if event.Response.OutputText != "" {
			return event.Response.OutputText
		}
		for _, out := range event.Response.Output {
			for _, frag := range out.Content {
				if frag.Text != "" {
					return frag.Text
				}
			}
		}
	}
	if event.Item != nil {
		for _, frag := range event.Item.Content {
			if frag.Text != "" {
				return frag.Text
			}
		}
	}
	for _, out := range event.Output {
		for _, frag := range out.Content {
			if frag.Text != "" {
				return frag.Text
			}
		}
	}
	return ""
}

func buildResponsesRequest(messages []agent.Message, model string, stream bool) responsesRequest {
	instructions, convo := splitInstructions(messages)
	items := make([]responsesMessage, 0, len(convo))
	for _, msg := range convo {
		items = append(items, responsesMessage{
			Role:    string(msg.Role),
			Content: []responsesFragment{{Type: fragmentTypeForRole(msg.Role), Text: msg.Content}},
		})
	}
	req := responsesRequest{
		Model:        model,
		Instructions: instructions,
		Input:        items,
		Stream:       stream,
	}
	if effort := prompts.ExtractReasoningEffort(instructions); effort != "" {
		req.Reasoning = map[string]string{"effort": effort}
	}
	return req
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

func fragmentTypeForRole(role agent.Role) string {
	switch role {
	case agent.RoleAssistant:
		return "output_text"
	default:
		return "input_text"
	}
}
