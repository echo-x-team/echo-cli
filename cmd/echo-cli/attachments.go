package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/prompts"
)

func loadAttachments(paths []string, workdir string) []agent.Message {
	var msgs []agent.Message
	for _, p := range paths {
		path := p
		if !filepath.IsAbs(path) && workdir != "" {
			path = filepath.Join(workdir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Warnf("attachment read failed (%s): %v", p, err)
			continue
		}
		msgs = append(msgs, agent.Message{
			Role:    agent.RoleUser,
			Content: "Attachment " + p + ":\n" + string(data),
		})
	}
	return msgs
}

// Images are threaded as references so we avoid emitting binary blobs into the transcript.
func loadImageAttachments(paths []string, workdir string) []agent.Message {
	var msgs []agent.Message
	for _, p := range paths {
		resolved := p
		if !filepath.IsAbs(resolved) && workdir != "" {
			resolved = filepath.Join(workdir, resolved)
		}
		msgs = append(msgs, agent.Message{
			Role:    agent.RoleUser,
			Content: fmt.Sprintf("Image attachment: %s (resolved: %s)", p, resolved),
		})
	}
	return msgs
}

// attachmentMessages 加载附件并返回 InputMessage 格式
func attachmentMessages(paths []string, workdir string) []events.InputMessage {
	var msgs []events.InputMessage
	for _, p := range paths {
		path := p
		if !filepath.IsAbs(path) && workdir != "" {
			path = filepath.Join(workdir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			log.Warnf("attachment read failed (%s): %v", p, err)
			continue
		}
		msgs = append(msgs, events.InputMessage{
			Role:    "user",
			Content: "Attachment " + p + ":\n" + string(data),
		})
	}
	return msgs
}

// imageAttachmentMessages 加载图片附件引用并返回 InputMessage 格式
func imageAttachmentMessages(paths []string, workdir string) []events.InputMessage {
	var msgs []events.InputMessage
	for _, p := range paths {
		resolved := p
		if !filepath.IsAbs(resolved) && workdir != "" {
			resolved = filepath.Join(workdir, resolved)
		}
		msgs = append(msgs, events.InputMessage{
			Role:    "user",
			Content: fmt.Sprintf("Image attachment: %s (resolved: %s)", p, resolved),
		})
	}
	return msgs
}

// extractConversationHistory 从历史消息中提取纯对话内容，过滤掉系统注入的内容
func extractConversationHistory(messages []agent.Message) []agent.Message {
	var filtered []agent.Message
	for _, msg := range messages {
		// 跳过系统注入的内容
		if msg.Role == agent.RoleSystem {
			// 跳过输出格式定义
			if strings.HasPrefix(msg.Content, prompts.OutputSchemaPrefix) {
				continue
			}
			// 跳过审查模式提示词
			if msg.Content == prompts.ReviewModeSystemPrompt {
				continue
			}
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// hasOutputSchema 检查历史中是否已包含输出格式定义（保留原函数以兼容性）
func hasOutputSchema(history []agent.Message) bool {
	for _, msg := range history {
		if msg.Role == agent.RoleSystem && strings.HasPrefix(msg.Content, prompts.OutputSchemaPrefix) {
			return true
		}
	}
	return false
}
