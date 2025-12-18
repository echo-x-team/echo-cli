# echo-cli (Go + Bubble Tea)

[English](README.md) | [日本語](readme-jp.md) | [中文](readme-cn.md)

Echo Team 命令行与终端 UI 客户端，使用 Go 和 Bubble Tea 构建，旨在提供强大的本地智能体体验。

## 概览

echo-cli 是一款面向 Echo Team 的 Go 编写 CLI/TUI 客户端，终端界面使用 Bubble Tea，目前聚焦 M1 脚手架：CLI 入口、配置加载、基础 TUI，以及不含工具执行的聊天。

## 当前状态

- 阶段：**M1 脚手架**（CLI 入口、配置加载、基础 TUI、无工具的 Echo Team 聊天）。
- Bubble Tea TUI 与 exec 模式共享同一会话管线；工具执行仍是占位，等待沙箱集成完成。

## 快速开始

### 启动 TUI

```bash
cd echo-cli
go run ./cmd/echo-cli --prompt "你好"
```

### Exec 模式

```bash
go run ./cmd/echo-cli exec --prompt "任务"
```

## 配置与环境

- 环境变量（优先级最高）：
  - `ANTHROPIC_BASE_URL`（例如 `https://open.bigmodel.cn/api/anthropic`）
  - `ANTHROPIC_AUTH_TOKEN`（鉴权 token）
- 配置文件：`~/.echo/config.toml`（可通过 `--config <path>` 覆盖）包含：
  - `url = "..."`、`token = "..."`、`model = "glm4.6"`
- 其他运行参数（语言/超时等）通过 CLI flag 或 `-c key=value` 覆盖控制。

## CLI（M1+）

- `--config <path>`：覆盖配置文件（默认 `~/.echo/config.toml`）。
- `--model <name>`：覆盖模型名称。
- `--cd <dir>`：设置状态栏显示的工作目录。
- `--prompt "<text>"`：初始用户消息（亦可作为位置参数）。
- `ping`：对配置的 Anthropic 兼容端点做连通性测试并打印返回文本。
- `exec <prompt>`：非交互 JSONL 运行，支持 `--session <id>` / `--resume-last` 持久化会话。
- 工具执行默认全自动；危险命令会先由大模型做安全审查并要求前端人工审批。

## AGENTS.md 引导

- 在 TUI 中运行 `/init`，请求智能体扫描仓库并按 agents.md 规范生成 `AGENTS.md`。
- 若工作目录已存在 `AGENTS.md`，该命令会跳过并仅给出提示信息，不会修改文件。

## 代码结构

- `cmd/echo-cli`：CLI 入口。
- `internal/config`：端点配置加载（url/token/model）。
- `internal/agent`：agent 循环与模型抽象（Anthropic 兼容客户端与流式支持）。
- `internal/tui`：Bubble Tea UI（对话流、输入栏、状态栏、@ 搜索、斜杠命令、会话选择器）。
- `internal/tools`：shell 与补丁工具（直接执行）。
- `internal/search`：供 `@` 选择器使用的文件搜索。
- `internal/instructions`：为系统提示发现 `AGENTS.md`。
- `internal/session`：exec/TUI 的会话存储与恢复。

## 路线图

- M2：命令工具、apply_patch、文件搜索选择器、斜杠命令浮层（基础版本已实现）。
- M3：更完善的 exec 模式（JSONL 对齐）、会话选择器、AGENTS.md 发现（已有基础支持）。
- M4：MCP 客户端/服务端、通知、ZDR。
