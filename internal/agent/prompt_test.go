package agent

import (
	"strings"
	"testing"
)

func TestApplyPatchToolSchema(t *testing.T) {
	var spec *ToolSpec
	for _, tool := range DefaultTools() {
		if tool.Name == "apply_patch" {
			spec = &tool
			break
		}
	}
	if spec == nil {
		t.Fatalf("apply_patch tool not found")
	}
	if !strings.Contains(spec.Description, "*** Add File:") {
		t.Fatalf("apply_patch.description should mention Echo Patch directives, got: %q", spec.Description)
	}

	params := spec.Parameters
	if got := params["type"]; got != "object" {
		t.Fatalf("apply_patch.type = %v, want object", got)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", params["properties"])
	}
	if _, ok := props["patch"]; !ok {
		t.Fatalf("patch property missing")
	}
	if patchProp, ok := props["patch"].(map[string]any); ok {
		if desc, _ := patchProp["description"].(string); !strings.Contains(desc, "*** Add File") {
			t.Fatalf("apply_patch.patch.description should mention directive names, got: %q", desc)
		}
	}
	if _, ok := props["path"]; ok {
		t.Fatalf("path property should be omitted for strict schema")
	}
	rawRequired, ok := params["required"]
	if !ok {
		t.Fatalf("required missing")
	}
	required, ok := toStrings(rawRequired)
	if !ok || len(required) != 1 || required[0] != "patch" {
		t.Fatalf("required = %v, want [patch]", rawRequired)
	}
	rawAdditional, ok := params["additionalProperties"]
	if !ok {
		t.Fatalf("additionalProperties missing")
	}
	additional, ok := rawAdditional.(bool)
	if !ok || additional {
		t.Fatalf("additionalProperties = %v, want false", rawAdditional)
	}
}

func TestFileSearchToolSchema(t *testing.T) {
	var spec *ToolSpec
	for _, tool := range DefaultTools() {
		if tool.Name == "file_search" {
			spec = &tool
			break
		}
	}
	if spec == nil {
		t.Fatalf("file_search tool not found")
	}

	params := spec.Parameters
	if got := params["type"]; got != "object" {
		t.Fatalf("file_search.type = %v, want object", got)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", params["properties"])
	}
	if _, ok := props["query"]; !ok {
		t.Fatalf("query property missing")
	}
	rawRequired, ok := params["required"]
	if !ok {
		t.Fatalf("required missing")
	}
	required, ok := toStrings(rawRequired)
	if !ok || len(required) != 1 || required[0] != "query" {
		t.Fatalf("required = %v, want [query]", rawRequired)
	}
	rawAdditional, ok := params["additionalProperties"]
	if !ok {
		t.Fatalf("additionalProperties missing")
	}
	additional, ok := rawAdditional.(bool)
	if !ok || additional {
		t.Fatalf("additionalProperties = %v, want false", rawAdditional)
	}
}

func TestUpdatePlanToolSchema(t *testing.T) {
	var spec *ToolSpec
	for _, tool := range DefaultTools() {
		if tool.Name == "update_plan" {
			spec = &tool
			break
		}
	}
	if spec == nil {
		t.Fatalf("update_plan tool not found")
	}

	params := spec.Parameters
	if got := params["type"]; got != "object" {
		t.Fatalf("update_plan.type = %v, want object", got)
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", params["properties"])
	}
	for _, key := range []string{"explanation", "plan"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("%s property missing", key)
		}
	}
	rawRequired, ok := params["required"]
	if !ok {
		t.Fatalf("required missing")
	}
	required, ok := toStrings(rawRequired)
	if !ok {
		t.Fatalf("required type = %T, want []string/[]any", rawRequired)
	}
	if len(required) != len(props) {
		t.Fatalf("required length = %d, want %d (%v)", len(required), len(props), props)
	}
	for _, key := range []string{"explanation", "plan"} {
		if !contains(required, key) {
			t.Fatalf("required missing %q: %v", key, required)
		}
	}
	rawAdditional, ok := params["additionalProperties"]
	if !ok {
		t.Fatalf("additionalProperties missing")
	}
	additional, ok := rawAdditional.(bool)
	if !ok || additional {
		t.Fatalf("additionalProperties = %v, want false", rawAdditional)
	}

	planProps, ok := props["plan"].(map[string]any)
	if !ok {
		t.Fatalf("plan properties type = %T, want map[string]any", props["plan"])
	}
	if got := planProps["type"]; got != "array" {
		t.Fatalf("plan.type = %v, want array", planProps["type"])
	}
	items, ok := planProps["items"].(map[string]any)
	if !ok {
		t.Fatalf("plan.items type = %T, want map[string]any", planProps["items"])
	}
	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("plan.items.properties type = %T, want map[string]any", items["properties"])
	}
	for _, key := range []string{"step", "status"} {
		if _, ok := itemProps[key]; !ok {
			t.Fatalf("plan.items.%s property missing", key)
		}
	}
	itemRequired, ok := toStrings(items["required"])
	if !ok {
		t.Fatalf("plan.items.required type = %T, want []string/[]any", items["required"])
	}
	if len(itemRequired) != len(itemProps) {
		t.Fatalf("plan.items.required length = %d, want %d (%v)", len(itemRequired), len(itemProps), itemProps)
	}
	for _, key := range []string{"step", "status"} {
		if !contains(itemRequired, key) {
			t.Fatalf("plan.items.required missing %q: %v", key, itemRequired)
		}
	}
	rawItemAdditional, ok := items["additionalProperties"]
	if !ok {
		t.Fatalf("plan.items.additionalProperties missing")
	}
	itemAdditional, ok := rawItemAdditional.(bool)
	if !ok || itemAdditional {
		t.Fatalf("plan.items.additionalProperties = %v, want false", rawItemAdditional)
	}
}

func toStrings(value any) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	default:
		return nil, false
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
