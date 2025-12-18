package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"echo-cli/internal/agent"
)

type CommandReview struct {
	Description string `json:"description"`
	RiskLevel   string `json:"risk_level"`
}

type CommandReviewer interface {
	Review(ctx context.Context, workdir string, command string) (CommandReview, error)
}

type LLMCommandReviewer struct {
	client agent.ModelClient
	model  string
}

func NewLLMCommandReviewer(client agent.ModelClient, model string) *LLMCommandReviewer {
	return &LLMCommandReviewer{client: client, model: model}
}

func (r *LLMCommandReviewer) Review(ctx context.Context, workdir string, command string) (CommandReview, error) {
	if r == nil || r.client == nil {
		return CommandReview{}, fmt.Errorf("reviewer not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	sys := strings.TrimSpace(`
你是一名安全分析员，正在评估被沙箱拦截的 shell 命令。请根据给定的元数据，概括命令的可能意图并给出风险判断，帮助用户决定是否批准执行。仅返回合法的 JSON，键为：

description（用现在时的一句话概述命令意图及潜在影响，保持简洁）
risk_level（"low"、"medium" 或 "high"） 风险等级示例：
low：只读检查、列目录、打印配置、从可信来源获取制品
medium：修改项目文件、安装依赖
high：删除或覆盖数据、外泄机密、提权、关闭安全控制 若信息不足，请选择证据支持的最谨慎等级。 只输出 JSON，不要使用 Markdown 代码块或额外说明。
`)

	in := map[string]any{
		"workdir":         workdir,
		"command":         command,
		"execution_model": "unified_exec",
	}
	raw, _ := json.Marshal(in)
	prompt := agent.Prompt{
		Model: r.model,
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: sys},
			{Role: agent.RoleUser, Content: string(raw)},
		},
	}
	text, err := r.client.Complete(ctx, prompt)
	if err != nil {
		return CommandReview{}, err
	}
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var out CommandReview
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return CommandReview{}, fmt.Errorf("invalid reviewer output: %w (raw=%q)", err, preview(text, 300))
	}
	out.Description = strings.TrimSpace(out.Description)
	out.RiskLevel = strings.ToLower(strings.TrimSpace(out.RiskLevel))
	if out.Description == "" {
		out.Description = "no description provided"
	}
	switch out.RiskLevel {
	case "low", "medium", "high":
	default:
		return CommandReview{}, fmt.Errorf("invalid risk_level %q", out.RiskLevel)
	}
	return out, nil
}

func preview(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
