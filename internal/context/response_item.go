package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"echo-cli/internal/agent"
)

// ResponseItemType mirrors the tagged enum from codex-rs.
type ResponseItemType string

const (
	ResponseItemTypeMessage            ResponseItemType = "message"
	ResponseItemTypeReasoning          ResponseItemType = "reasoning"
	ResponseItemTypeLocalShellCall     ResponseItemType = "local_shell_call"
	ResponseItemTypeFunctionCall       ResponseItemType = "function_call"
	ResponseItemTypeFunctionCallOutput ResponseItemType = "function_call_output"
	ResponseItemTypeWebSearchCall      ResponseItemType = "web_search_call"
	ResponseItemTypeGhostSnapshot      ResponseItemType = "ghost_snapshot"
	ResponseItemTypeCompactionSummary  ResponseItemType = "compaction_summary"
	ResponseItemTypeOther              ResponseItemType = "other"
)

// ResponseInputItemType mirrors the inputs codex expects to feed back into the model.
type ResponseInputItemType string

const (
	ResponseInputTypeMessage            ResponseInputItemType = "message"
	ResponseInputTypeFunctionCallOutput ResponseInputItemType = "function_call_output"
)

// ContentItemType enumerates message content kinds.
type ContentItemType string

const (
	ContentItemInputText  ContentItemType = "input_text"
	ContentItemInputImage ContentItemType = "input_image"
	ContentItemOutputText ContentItemType = "output_text"
)

// ResponseItem is a tagged union mirroring codex-rs `ResponseItem`.
// JSON 形态遵循 `type` 字段做判别，负载字段与变体同级。
type ResponseItem struct {
	Type ResponseItemType `json:"type"`

	Message            *MessageResponseItem            `json:"-"`
	Reasoning          *ReasoningResponseItem          `json:"-"`
	LocalShellCall     *LocalShellCallResponseItem     `json:"-"`
	FunctionCall       *FunctionCallResponseItem       `json:"-"`
	FunctionCallOutput *FunctionCallOutputResponseItem `json:"-"`
	WebSearchCall      *WebSearchCallResponseItem      `json:"-"`
	GhostSnapshot      *GhostSnapshotResponseItem      `json:"-"`
	CompactionSummary  *CompactionSummaryResponseItem  `json:"-"`
}

// ResponseInputItem captures tool outputs or user messages sent back to the model.
type ResponseInputItem struct {
	Type               ResponseInputItemType
	Message            *MessageResponseItem
	FunctionCallOutput *FunctionCallOutputInput
}

// ContentItem mirrors the Rust variant with a tagged "type" field.
type ContentItem struct {
	Type     ContentItemType `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL string          `json:"image_url,omitempty"`
}

// MessageResponseItem represents a role/content message.
type MessageResponseItem struct {
	ID      string        `json:"id,omitempty"`
	Role    string        `json:"role"`
	Content []ContentItem `json:"content"`
}

// ReasoningResponseItem holds model reasoning blocks.
type ReasoningResponseItem struct {
	ID               string                          `json:"id,omitempty"`
	Summary          []ReasoningItemReasoningSummary `json:"summary"`
	Content          []ReasoningItemContent          `json:"content,omitempty"`
	EncryptedContent string                          `json:"encrypted_content,omitempty"`
}

// LocalShellCallResponseItem records a shell call request/result.
type LocalShellCallResponseItem struct {
	ID     string           `json:"id,omitempty"`
	CallID string           `json:"call_id,omitempty"`
	Status LocalShellStatus `json:"status"`
	Action LocalShellAction `json:"action"`
}

// FunctionCallResponseItem mirrors the tool/function call payload.
type FunctionCallResponseItem struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
}

// FunctionCallOutputResponseItem is the structured tool output.
type FunctionCallOutputResponseItem struct {
	CallID string                    `json:"call_id"`
	Output FunctionCallOutputPayload `json:"output"`
}

// WebSearchAction captures the search action payload.
type WebSearchAction struct {
	Type    string `json:"type"`
	Query   string `json:"query,omitempty"`
	URL     string `json:"url,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

// WebSearchCallResponseItem represents search triggers.
type WebSearchCallResponseItem struct {
	ID     string          `json:"id,omitempty"`
	Status string          `json:"status,omitempty"`
	Action WebSearchAction `json:"action"`
}

// GhostSnapshotResponseItem is a placeholder for ghost commits.
type GhostSnapshotResponseItem struct {
	GhostCommit GhostCommit `json:"ghost_commit"`
}

// CompactionSummaryResponseItem stores compacted content.
type CompactionSummaryResponseItem struct {
	EncryptedContent string `json:"encrypted_content"`
}

// ReasoningItemReasoningSummary mirrors summary text blocks.
type ReasoningItemReasoningSummary struct {
	Type ContentItemType `json:"type"`
	Text string          `json:"text"`
}

// ReasoningItemContent mirrors reasoning or plain text.
type ReasoningItemContent struct {
	Type ContentItemType `json:"type"`
	Text string          `json:"text"`
}

// LocalShellStatus describes execution state.
type LocalShellStatus string

const (
	LocalShellStatusCompleted  LocalShellStatus = "completed"
	LocalShellStatusInProgress LocalShellStatus = "in_progress"
	LocalShellStatusIncomplete LocalShellStatus = "incomplete"
)

// LocalShellAction currently only supports exec.
type LocalShellAction struct {
	Type             string            `json:"type"`
	Command          []string          `json:"command,omitempty"`
	TimeoutMs        int64             `json:"timeout_ms,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	User             string            `json:"user,omitempty"`
}

// FunctionCallOutputInput wraps the payload we feed back to the model.
type FunctionCallOutputInput struct {
	CallID string                    `json:"call_id"`
	Output FunctionCallOutputPayload `json:"output"`
}

// FunctionCallOutputContentItem mirrors the Responses API.
type FunctionCallOutputContentItem struct {
	Type     ContentItemType `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL string          `json:"image_url,omitempty"`
}

// FunctionCallOutputPayload carries both legacy and structured outputs.
type FunctionCallOutputPayload struct {
	Content      string                          `json:"content"`
	ContentItems []FunctionCallOutputContentItem `json:"content_items,omitempty"`
	Success      *bool                           `json:"success,omitempty"`
}

// GhostCommit is a lightweight placeholder for the upstream type.
type GhostCommit struct {
	ID string `json:"id,omitempty"`
}

// ToResponseItem mirrors the Rust From<ResponseInputItem> implementation.
func (r ResponseInputItem) ToResponseItem() ResponseItem {
	switch r.Type {
	case ResponseInputTypeMessage:
		if r.Message != nil {
			return newMessageItem(*r.Message)
		}
	case ResponseInputTypeFunctionCallOutput:
		if r.FunctionCallOutput != nil {
			return ResponseItem{
				Type: ResponseItemTypeFunctionCallOutput,
				FunctionCallOutput: &FunctionCallOutputResponseItem{
					CallID: r.FunctionCallOutput.CallID,
					Output: r.FunctionCallOutput.Output,
				},
			}
		}
	}
	return ResponseItem{Type: ResponseItemTypeOther}
}

func newMessageItem(msg MessageResponseItem) ResponseItem {
	return ResponseItem{Type: ResponseItemTypeMessage, Message: &msg}
}

// NewAssistantMessageItem builds a simple assistant message response item.
func NewAssistantMessageItem(text string) ResponseItem {
	return ResponseItem{
		Type: ResponseItemTypeMessage,
		Message: &MessageResponseItem{
			Role: "assistant",
			Content: []ContentItem{
				{Type: ContentItemOutputText, Text: text},
			},
		},
	}
}

// NewUserMessageItem builds a user message response item.
func NewUserMessageItem(text string) ResponseItem {
	return ResponseItem{
		Type: ResponseItemTypeMessage,
		Message: &MessageResponseItem{
			Role: "user",
			Content: []ContentItem{
				{Type: ContentItemInputText, Text: text},
			},
		},
	}
}

// MarshalJSON customizes tagged-union encoding.
func (r ResponseItem) MarshalJSON() ([]byte, error) {
	switch r.Type {
	case ResponseItemTypeMessage:
		if r.Message == nil {
			break
		}
		payload := struct {
			Type    ResponseItemType `json:"type"`
			ID      string           `json:"id,omitempty"`
			Role    string           `json:"role"`
			Content []ContentItem    `json:"content"`
		}{
			Type:    r.Type,
			ID:      r.Message.ID,
			Role:    r.Message.Role,
			Content: r.Message.Content,
		}
		return json.Marshal(payload)
	case ResponseItemTypeReasoning:
		if r.Reasoning == nil {
			break
		}
		payload := struct {
			Type             ResponseItemType                `json:"type"`
			ID               string                          `json:"id,omitempty"`
			Summary          []ReasoningItemReasoningSummary `json:"summary"`
			Content          []ReasoningItemContent          `json:"content,omitempty"`
			EncryptedContent string                          `json:"encrypted_content,omitempty"`
		}{
			Type:             r.Type,
			ID:               r.Reasoning.ID,
			Summary:          r.Reasoning.Summary,
			Content:          r.Reasoning.Content,
			EncryptedContent: r.Reasoning.EncryptedContent,
		}
		return json.Marshal(payload)
	case ResponseItemTypeLocalShellCall:
		if r.LocalShellCall == nil {
			break
		}
		payload := struct {
			Type   ResponseItemType `json:"type"`
			ID     string           `json:"id,omitempty"`
			CallID string           `json:"call_id,omitempty"`
			Status LocalShellStatus `json:"status"`
			Action LocalShellAction `json:"action"`
		}{
			Type:   r.Type,
			ID:     r.LocalShellCall.ID,
			CallID: r.LocalShellCall.CallID,
			Status: r.LocalShellCall.Status,
			Action: r.LocalShellCall.Action,
		}
		return json.Marshal(payload)
	case ResponseItemTypeFunctionCall:
		if r.FunctionCall == nil {
			break
		}
		payload := struct {
			Type      ResponseItemType `json:"type"`
			ID        string           `json:"id,omitempty"`
			Name      string           `json:"name"`
			Arguments string           `json:"arguments"`
			CallID    string           `json:"call_id"`
		}{
			Type:      r.Type,
			ID:        r.FunctionCall.ID,
			Name:      r.FunctionCall.Name,
			Arguments: r.FunctionCall.Arguments,
			CallID:    r.FunctionCall.CallID,
		}
		return json.Marshal(payload)
	case ResponseItemTypeFunctionCallOutput:
		if r.FunctionCallOutput == nil {
			break
		}
		payload := struct {
			Type   ResponseItemType          `json:"type"`
			CallID string                    `json:"call_id"`
			Output FunctionCallOutputPayload `json:"output"`
		}{
			Type:   r.Type,
			CallID: r.FunctionCallOutput.CallID,
			Output: r.FunctionCallOutput.Output,
		}
		return json.Marshal(payload)
	case ResponseItemTypeWebSearchCall:
		if r.WebSearchCall == nil {
			break
		}
		payload := struct {
			Type   ResponseItemType `json:"type"`
			ID     string           `json:"id,omitempty"`
			Status string           `json:"status,omitempty"`
			Action WebSearchAction  `json:"action"`
		}{
			Type:   r.Type,
			ID:     r.WebSearchCall.ID,
			Status: r.WebSearchCall.Status,
			Action: r.WebSearchCall.Action,
		}
		return json.Marshal(payload)
	case ResponseItemTypeGhostSnapshot:
		if r.GhostSnapshot == nil {
			break
		}
		payload := struct {
			Type        ResponseItemType `json:"type"`
			GhostCommit GhostCommit      `json:"ghost_commit"`
		}{
			Type:        r.Type,
			GhostCommit: r.GhostSnapshot.GhostCommit,
		}
		return json.Marshal(payload)
	case ResponseItemTypeCompactionSummary:
		if r.CompactionSummary == nil {
			break
		}
		payload := struct {
			Type             ResponseItemType `json:"type"`
			EncryptedContent string           `json:"encrypted_content"`
		}{
			Type:             r.Type,
			EncryptedContent: r.CompactionSummary.EncryptedContent,
		}
		return json.Marshal(payload)
	}

	// fallback
	return json.Marshal(struct {
		Type ResponseItemType `json:"type"`
	}{Type: ResponseItemTypeOther})
}

// UnmarshalJSON decodes tagged-union JSON into ResponseItem.
func (r *ResponseItem) UnmarshalJSON(data []byte) error {
	var header struct {
		Type ResponseItemType `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	r.Type = header.Type

	switch header.Type {
	case ResponseItemTypeMessage:
		var payload struct {
			ID      string        `json:"id,omitempty"`
			Role    string        `json:"role"`
			Content []ContentItem `json:"content"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.Message = &MessageResponseItem{ID: payload.ID, Role: payload.Role, Content: payload.Content}
	case ResponseItemTypeReasoning:
		var payload struct {
			ID               string                          `json:"id,omitempty"`
			Summary          []ReasoningItemReasoningSummary `json:"summary"`
			Content          []ReasoningItemContent          `json:"content,omitempty"`
			EncryptedContent string                          `json:"encrypted_content,omitempty"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.Reasoning = &ReasoningResponseItem{ID: payload.ID, Summary: payload.Summary, Content: payload.Content, EncryptedContent: payload.EncryptedContent}
	case ResponseItemTypeLocalShellCall:
		var payload struct {
			ID     string           `json:"id,omitempty"`
			CallID string           `json:"call_id,omitempty"`
			Status LocalShellStatus `json:"status"`
			Action LocalShellAction `json:"action"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.LocalShellCall = &LocalShellCallResponseItem{ID: payload.ID, CallID: payload.CallID, Status: payload.Status, Action: payload.Action}
	case ResponseItemTypeFunctionCall:
		var payload struct {
			ID        string `json:"id,omitempty"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			CallID    string `json:"call_id"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.FunctionCall = &FunctionCallResponseItem{ID: payload.ID, Name: payload.Name, Arguments: payload.Arguments, CallID: payload.CallID}
	case ResponseItemTypeFunctionCallOutput:
		var payload struct {
			CallID string                    `json:"call_id"`
			Output FunctionCallOutputPayload `json:"output"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.FunctionCallOutput = &FunctionCallOutputResponseItem{CallID: payload.CallID, Output: payload.Output}
	case ResponseItemTypeWebSearchCall:
		var payload struct {
			ID     string          `json:"id,omitempty"`
			Status string          `json:"status,omitempty"`
			Action WebSearchAction `json:"action"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.WebSearchCall = &WebSearchCallResponseItem{ID: payload.ID, Status: payload.Status, Action: payload.Action}
	case ResponseItemTypeGhostSnapshot:
		var payload struct {
			GhostCommit GhostCommit `json:"ghost_commit"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.GhostSnapshot = &GhostSnapshotResponseItem{GhostCommit: payload.GhostCommit}
	case ResponseItemTypeCompactionSummary:
		var payload struct {
			EncryptedContent string `json:"encrypted_content"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		r.CompactionSummary = &CompactionSummaryResponseItem{EncryptedContent: payload.EncryptedContent}
	default:
		// ignore unknown variants
	}
	return nil
}

// NormalizeRawJSON 将字符串规整为合法 JSON bytes（对齐 codex 行为：非 JSON 则按字符串编码）。
func NormalizeRawJSON(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return json.RawMessage("null")
	}
	data := []byte(trimmed)
	if json.Valid(data) {
		return json.RawMessage(data)
	}
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(encoded)
}

func ResponseItemsToAgentMessages(items []ResponseItem) []agent.Message {
	out := make([]agent.Message, 0, len(items))
	for _, item := range items {
		out = append(out, responseItemToAgentMessages(item)...)
	}
	return out
}

func responseItemToAgentMessages(item ResponseItem) []agent.Message {
	switch item.Type {
	case ResponseItemTypeMessage:
		if item.Message == nil {
			return nil
		}
		return []agent.Message{{Role: agent.Role(item.Message.Role), Content: FlattenContentItems(item.Message.Content)}}
	case ResponseItemTypeFunctionCall:
		if item.FunctionCall == nil {
			return nil
		}
		args := NormalizeRawJSON(item.FunctionCall.Arguments)
		content := fmt.Sprintf("[tool_use] %s (%s)", item.FunctionCall.Name, item.FunctionCall.CallID)
		return []agent.Message{{
			Role:    agent.RoleAssistant,
			Content: content,
			ToolUse: &agent.ToolUse{
				ID:    item.FunctionCall.CallID,
				Name:  item.FunctionCall.Name,
				Input: args,
			},
		}}
	case ResponseItemTypeFunctionCallOutput:
		if item.FunctionCallOutput == nil {
			return nil
		}
		content := strings.TrimSpace(item.FunctionCallOutput.Output.Content)
		isError := false
		if item.FunctionCallOutput.Output.Success != nil {
			isError = !*item.FunctionCallOutput.Output.Success
		}
		return []agent.Message{{
			Role:    agent.RoleUser,
			Content: content,
			ToolResult: &agent.ToolResult{
				ToolUseID: item.FunctionCallOutput.CallID,
				Content:   content,
				IsError:   isError,
			},
		}}
	case ResponseItemTypeReasoning:
		if item.Reasoning == nil {
			return nil
		}
		text := FlattenReasoning(*item.Reasoning)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []agent.Message{{Role: agent.RoleAssistant, Content: text}}
	default:
		return nil
	}
}

func FlattenContentItems(items []ContentItem) string {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case ContentItemInputText, ContentItemOutputText:
			parts = append(parts, item.Text)
		case ContentItemInputImage:
			if item.ImageURL != "" {
				parts = append(parts, fmt.Sprintf("[image: %s]", item.ImageURL))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func FlattenReasoning(item ReasoningResponseItem) string {
	var parts []string
	for _, summary := range item.Summary {
		parts = append(parts, summary.Text)
	}
	for _, content := range item.Content {
		parts = append(parts, content.Text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func LastAssistantMessage(items []ResponseItem) string {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type == ResponseItemTypeMessage && items[i].Message != nil && items[i].Message.Role == "assistant" {
			return FlattenContentItems(items[i].Message.Content)
		}
	}
	return ""
}
