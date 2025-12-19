package context

import (
	"os"
	"strconv"
	"strings"

	"echo-cli/internal/logger"
)

const (
	defaultToolOutputTokenLimit = 2500
)

func toolOutputTruncationPolicy() TruncationPolicy {
	if raw := strings.TrimSpace(os.Getenv("ECHO_TOOL_OUTPUT_TOKEN_LIMIT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return TokensPolicy(n)
		}
	}
	return TokensPolicy(defaultToolOutputTokenLimit)
}

func isAPIMessage(item *ResponseItem) bool {
	switch item.Type {
	case ResponseItemTypeMessage:
		if item.Message == nil {
			return false
		}
		return strings.TrimSpace(item.Message.Role) != "system"
	case ResponseItemTypeFunctionCall,
		ResponseItemTypeFunctionCallOutput,
		ResponseItemTypeLocalShellCall,
		ResponseItemTypeReasoning,
		ResponseItemTypeWebSearchCall,
		ResponseItemTypeCompactionSummary:
		return true
	case ResponseItemTypeGhostSnapshot:
		// GhostSnapshot 不应发送给模型，但需要写入历史以支持本地能力（如 undo）。
		return true
	default:
		return false
	}
}

// processResponseItemsForHistory 对齐 codex 的 record_items/process_item：
// - 过滤非 API items（但保留 GhostSnapshot：仅用于本地能力，不会发给模型）
// - 对工具输出做截断（额外乘以 1.2 预留序列化开销）
func processResponseItemsForHistory(items []ResponseItem) []ResponseItem {
	if len(items) == 0 {
		return nil
	}

	policyWithSerializationBudget := toolOutputTruncationPolicy().Mul(1.2)
	out := make([]ResponseItem, 0, len(items))

	for _, item := range items {
		if !isAPIMessage(&item) {
			continue
		}

		switch item.Type {
		case ResponseItemTypeFunctionCallOutput:
			if item.FunctionCallOutput == nil {
				continue
			}
			payload := item.FunctionCallOutput.Output
			payload.Content = FormattedTruncateText(payload.Content, policyWithSerializationBudget)
			if len(payload.ContentItems) > 0 {
				payload.ContentItems = truncateFunctionOutputContentItems(payload.ContentItems, policyWithSerializationBudget)
			}
			out = append(out, ResponseItem{
				Type: item.Type,
				FunctionCallOutput: &FunctionCallOutputResponseItem{
					CallID: item.FunctionCallOutput.CallID,
					Output: payload,
				},
			})
		default:
			out = append(out, item)
		}
	}

	normalizeHistory(&out)
	return out
}

func truncateFunctionOutputContentItems(items []FunctionCallOutputContentItem, policy TruncationPolicy) []FunctionCallOutputContentItem {
	out := make([]FunctionCallOutputContentItem, 0, len(items))
	remainingBytes := policy.byteBudget()

	omittedTextItems := 0
	for _, it := range items {
		switch it.Type {
		case ContentItemInputText, ContentItemOutputText:
			if remainingBytes == 0 {
				omittedTextItems++
				continue
			}
			costBytes := len(it.Text)
			if costBytes <= remainingBytes {
				out = append(out, it)
				remainingBytes -= costBytes
				continue
			}

			snippet := truncateWithByteBudget(it.Text, remainingBytes, policy.Kind)
			if strings.TrimSpace(snippet) == "" {
				omittedTextItems++
			} else {
				out = append(out, FunctionCallOutputContentItem{Type: it.Type, Text: snippet})
			}
			remainingBytes = 0
		case ContentItemInputImage:
			out = append(out, it)
		default:
			out = append(out, it)
		}
	}
	if omittedTextItems > 0 {
		out = append(out, FunctionCallOutputContentItem{
			Type: ContentItemInputText,
			Text: "[omitted " + itoa(omittedTextItems) + " text items ...]",
		})
	}
	return out
}

func normalizeHistory(items *[]ResponseItem) {
	if items == nil || len(*items) == 0 {
		return
	}
	ensureCallOutputsPresent(items)
	removeOrphanOutputs(items)
}

func ensureCallOutputsPresent(items *[]ResponseItem) {
	in := *items
	type insert struct {
		idx  int
		item ResponseItem
	}
	var missing []insert

	for idx, item := range in {
		switch item.Type {
		case ResponseItemTypeFunctionCall:
			if item.FunctionCall == nil {
				continue
			}
			callID := strings.TrimSpace(item.FunctionCall.CallID)
			if callID == "" {
				continue
			}
			if hasOutputForCall(in, callID) {
				continue
			}
			logger.Warnf("function call output missing; inserting aborted placeholder call_id=%s", callID)
			missing = append(missing, insert{
				idx: idx,
				item: ResponseItem{
					Type: ResponseItemTypeFunctionCallOutput,
					FunctionCallOutput: &FunctionCallOutputResponseItem{
						CallID: callID,
						Output: FunctionCallOutputPayload{Content: "aborted"},
					},
				},
			})
		case ResponseItemTypeLocalShellCall:
			if item.LocalShellCall == nil {
				continue
			}
			callID := strings.TrimSpace(item.LocalShellCall.CallID)
			if callID == "" {
				continue
			}
			if hasOutputForCall(in, callID) {
				continue
			}
			logger.Warnf("local shell output missing; inserting aborted placeholder call_id=%s", callID)
			missing = append(missing, insert{
				idx: idx,
				item: ResponseItem{
					Type: ResponseItemTypeFunctionCallOutput,
					FunctionCallOutput: &FunctionCallOutputResponseItem{
						CallID: callID,
						Output: FunctionCallOutputPayload{Content: "aborted"},
					},
				},
			})
		}
	}

	if len(missing) == 0 {
		return
	}
	// reverse insert to avoid index shifting
	for i := len(missing) - 1; i >= 0; i-- {
		ins := missing[i]
		in = append(in[:ins.idx+1], append([]ResponseItem{ins.item}, in[ins.idx+1:]...)...)
	}
	*items = in
}

func hasOutputForCall(items []ResponseItem, callID string) bool {
	for _, it := range items {
		if it.Type != ResponseItemTypeFunctionCallOutput || it.FunctionCallOutput == nil {
			continue
		}
		if it.FunctionCallOutput.CallID == callID {
			return true
		}
	}
	return false
}

func removeOrphanOutputs(items *[]ResponseItem) {
	in := *items
	callIDs := map[string]struct{}{}
	localShellCallIDs := map[string]struct{}{}

	for _, it := range in {
		switch it.Type {
		case ResponseItemTypeFunctionCall:
			if it.FunctionCall != nil && strings.TrimSpace(it.FunctionCall.CallID) != "" {
				callIDs[it.FunctionCall.CallID] = struct{}{}
			}
		case ResponseItemTypeLocalShellCall:
			if it.LocalShellCall != nil && strings.TrimSpace(it.LocalShellCall.CallID) != "" {
				localShellCallIDs[it.LocalShellCall.CallID] = struct{}{}
			}
		}
	}

	out := in[:0]
	for _, it := range in {
		if it.Type == ResponseItemTypeFunctionCallOutput && it.FunctionCallOutput != nil {
			id := strings.TrimSpace(it.FunctionCallOutput.CallID)
			if id == "" {
				continue
			}
			if _, ok := callIDs[id]; ok {
				out = append(out, it)
				continue
			}
			if _, ok := localShellCallIDs[id]; ok {
				out = append(out, it)
				continue
			}
			logger.Warnf("orphan function call output dropped call_id=%s", id)
			continue
		}
		out = append(out, it)
	}
	*items = out
}

// RemoveFirstItem removes the oldest item while keeping call/output pairing intact (codex normalize invariant).
func RemoveFirstItem(items []ResponseItem) []ResponseItem {
	if len(items) == 0 {
		return items
	}
	removed := items[0]
	items = items[1:]
	return removeCorrespondingFor(items, removed)
}

func removeCorrespondingFor(items []ResponseItem, removed ResponseItem) []ResponseItem {
	switch removed.Type {
	case ResponseItemTypeFunctionCall:
		if removed.FunctionCall == nil {
			return items
		}
		return removeFirstMatching(items, func(it *ResponseItem) bool {
			return it.Type == ResponseItemTypeFunctionCallOutput && it.FunctionCallOutput != nil && it.FunctionCallOutput.CallID == removed.FunctionCall.CallID
		})
	case ResponseItemTypeFunctionCallOutput:
		if removed.FunctionCallOutput == nil {
			return items
		}
		callID := removed.FunctionCallOutput.CallID
		out := removeFirstMatching(items, func(it *ResponseItem) bool {
			return it.Type == ResponseItemTypeFunctionCall && it.FunctionCall != nil && it.FunctionCall.CallID == callID
		})
		if len(out) != len(items) {
			return out
		}
		return removeFirstMatching(items, func(it *ResponseItem) bool {
			return it.Type == ResponseItemTypeLocalShellCall && it.LocalShellCall != nil && it.LocalShellCall.CallID == callID
		})
	case ResponseItemTypeLocalShellCall:
		if removed.LocalShellCall == nil {
			return items
		}
		callID := removed.LocalShellCall.CallID
		return removeFirstMatching(items, func(it *ResponseItem) bool {
			return it.Type == ResponseItemTypeFunctionCallOutput && it.FunctionCallOutput != nil && it.FunctionCallOutput.CallID == callID
		})
	default:
		return items
	}
}

func removeFirstMatching(items []ResponseItem, match func(it *ResponseItem) bool) []ResponseItem {
	for i := range items {
		if match(&items[i]) {
			return append(items[:i], items[i+1:]...)
		}
	}
	return items
}
