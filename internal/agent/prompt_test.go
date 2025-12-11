package agent

import "testing"

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
