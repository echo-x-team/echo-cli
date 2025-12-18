package agent

// ToolSpec 描述可供模型调用的工具定义，遵循 function 工具的通用 schema 约定。
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
			Name:        "exec_command",
			Description: "在 PTY 会话中启动命令（Unified Exec），适用于交互式脚手架；返回 output，若进程仍在运行则返回 session_id。若出现交互提示/选择器，用 write_stdin 写入回车/选项驱动完成。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "要执行的完整 shell 命令。",
					},
					"workdir": map[string]any{
						"type":        "string",
						"description": "可选：覆盖当前工作目录（默认使用会话工作目录）。",
					},
					"yield_time_ms": map[string]any{
						"type":        "integer",
						"description": "可选：最多等待输出的时间（毫秒），超时后返回当前输出与 session 状态。",
					},
					"max_output_bytes": map[string]any{
						"type":        "integer",
						"description": "可选：本次调用最多返回的输出字节数（用于避免超大输出）。",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "write_stdin",
			Description: "向 Unified Exec 会话写入 stdin（可用于选择/回车/Ctrl+C 等），并返回最近 output；进程退出后会返回 exit_code 且不再返回 session_id。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_id": map[string]any{
						"type":        "string",
						"description": "exec_command 返回的会话 id。",
					},
					"chars": map[string]any{
						"type":        "string",
						"description": "要写入 stdin 的字符（例如 \"\\n\" 回车，\"\\u0003\" 为 Ctrl+C）；可传空字符串用于轮询输出。",
					},
					"yield_time_ms": map[string]any{
						"type":        "integer",
						"description": "可选：最多等待输出的时间（毫秒）。",
					},
					"max_output_bytes": map[string]any{
						"type":        "integer",
						"description": "可选：本次调用最多返回的输出字节数。",
					},
				},
				"required":             []string{"session_id", "chars"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "apply_patch",
			Description: "应用补丁（支持 unified diff 或 Echo Patch 格式）。Echo Patch 需要以 \"*** Begin Patch\" 开头、以 \"*** End Patch\" 结束，并且仅允许使用 \"*** Add File:\" / \"*** Update File:\" / \"*** Delete File:\"（可选 \"*** Move to:\" 重命名；\"*** End of File\" 可用于标注文件结束）。注意：\"*** Update File\" 的 hunk 需要使用 \"@@\" 分隔，每一行必须以前缀开头：空格=上下文，\"-\"=删除，\"+\"=新增；不要直接粘贴无前缀的文件内容。若要整文件替换，优先用 \"*** Delete File\" + \"*** Add File\"。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patch": map[string]any{
						"type":        "string",
						"description": "补丁内容（unified diff 或 Echo Patch）。如果使用 Echo Patch，必须使用 *** Add File / *** Update File / *** Delete File 指令，不要使用 \"*** Create File\" 之类的变体；\"*** Update File\" 的每一行必须以 \" \"/\"+\"/\"-\" 开头（配合 @@ hunk）。",
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
						"description": "用于过滤结果的关键词或模式（可传空字符串获取全部）。",
					},
				},
				"required":             []string{"query"},
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
						"description": "对计划变更的简要说明，可为空字符串。",
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
				"required":             []string{"explanation", "plan"},
				"additionalProperties": false,
			},
		},
	}
}
