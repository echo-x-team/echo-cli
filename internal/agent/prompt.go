package agent

// ToolSpec 描述可供模型调用的工具定义，遵循 OpenAI function 工具的 schema 约定。
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Prompt 代表一次模型调用的完整请求，包括模型、消息与工具配置。
type Prompt struct {
	Model             string
	Messages          []Message
	Tools             []ToolSpec
	ParallelToolCalls bool
	OutputSchema      string
}

// DefaultTools 返回 Echo CLI 内置的工具规范，供模型端暴露调用能力。
func DefaultTools() []ToolSpec {
	return []ToolSpec{
		{
			Name:        "command",
			Description: "在工作区执行 shell 命令，返回 stdout/stderr 与退出码。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "要执行的完整 shell 命令。",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "apply_patch",
			Description: "应用统一 diff 补丁。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patch": map[string]any{
						"type":        "string",
						"description": "统一 diff 补丁内容。",
					},
				},
				"required":             []string{"patch"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "file_read",
			Description: "读取给定文件内容。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "要读取的文件相对路径。",
					},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "file_search",
			Description: "枚举工作区文件列表，可选提供关键词过滤。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "可选：用于过滤结果的关键词或模式。",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			Name:        "update_plan",
			Description: "更新当前计划列表，支持 pending/in_progress/completed 三种状态。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"explanation": map[string]any{
						"type":        "string",
						"description": "可选：对计划变更的简要说明。",
					},
					"plan": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"step": map[string]any{
									"type":        "string",
									"description": "步骤描述。",
								},
								"status": map[string]any{
									"type": "string",
									"enum": []string{
										"pending",
										"in_progress",
										"completed",
									},
								},
							},
							"required":             []string{"step", "status"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"plan"},
				"additionalProperties": false,
			},
		},
	}
}
