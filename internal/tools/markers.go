package tools

import (
	"encoding/json"
	"strings"
)

// ToolCallMarker is the structured payload we expect from the model.
// Example: {"tool":"command","id":"call-1","args":{"command":"ls"}}
type ToolCallMarker struct {
	Tool string          `json:"tool"`
	ID   string          `json:"id"`
	Args json.RawMessage `json:"args"`
}

// ParseMarkers extracts tool calls from model text. Markers are JSON objects
// delimited by ```tool ... ``` blocks or inline JSON objects containing a "tool" field.
func ParseMarkers(text string) ([]ToolCallMarker, error) {
	var markers []ToolCallMarker
	chunks := extractJSONChunks(text)
	for _, chunk := range chunks {
		var marker ToolCallMarker
		if err := json.Unmarshal([]byte(chunk), &marker); err != nil {
			continue
		}
		if marker.Tool == "" {
			continue
		}
		markers = append(markers, marker)
	}
	return markers, nil
}

func extractJSONChunks(text string) []string {
	var chunks []string
	lines := strings.Split(text, "\n")
	var buf []string
	inBlock := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```tool") {
			inBlock = true
			buf = nil
			continue
		}
		if inBlock {
			if strings.HasPrefix(trim, "```") {
				inBlock = false
				if len(buf) > 0 {
					chunks = append(chunks, strings.Join(buf, "\n"))
				}
				buf = nil
				continue
			}
			buf = append(buf, line)
		} else if strings.HasPrefix(trim, "{") && strings.Contains(trim, "\"tool\"") && strings.HasSuffix(trim, "}") {
			chunks = append(chunks, trim)
		}
	}
	return chunks
}
