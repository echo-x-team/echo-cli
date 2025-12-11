package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// PlanItem 描述 update_plan 的单个步骤。
type PlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// UpdatePlanArgs 是 update_plan 的入参。
type UpdatePlanArgs struct {
	Explanation string     `json:"explanation,omitempty"`
	Plan        []PlanItem `json:"plan"`
}

// DecodePlanArgs parses update_plan arguments with strict schema checks.
func DecodePlanArgs(raw []byte) (UpdatePlanArgs, error) {
	var args UpdatePlanArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return args, fmt.Errorf("failed to parse plan arguments: %w", err)
	}
	for i, item := range args.Plan {
		if strings.TrimSpace(item.Step) == "" {
			return args, fmt.Errorf("plan[%d]: step is required", i)
		}
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return args, fmt.Errorf("plan[%d]: invalid status %q", i, item.Status)
		}
	}
	return args, nil
}
