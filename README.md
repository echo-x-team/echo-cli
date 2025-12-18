# echo-cli (Go + Bubble Tea)

[English](README.md) | [日本語](readme-jp.md) | [中文](readme-cn.md)

Echo Team command-line + TUI client built in Go and Bubble Tea, designed to deliver a capable local agent experience.

## Overview

echo-cli is a Go-based CLI/TUI client for Echo Team. It uses Bubble Tea for the terminal UI and provides chat with tool execution.

## Current Status

- Stage: **M2** (tool execution enabled).
- Bubble Tea TUI and exec mode share the same session pipeline; tool execution is fully automatic (no sandbox/approvals).

## Quickstart

### Launch TUI

```bash
cd echo-cli
go run ./cmd/echo-cli --prompt "你好"
```

### Exec mode

```bash
go run ./cmd/echo-cli exec --prompt "任务"
```

## Configuration & Environment

- Env (highest priority):
  - `ANTHROPIC_BASE_URL` (e.g. `https://open.bigmodel.cn/api/anthropic`)
  - `ANTHROPIC_AUTH_TOKEN` (provider auth token)
- Config file: `~/.echo/config.toml` (or override via `--config <path>`):
  - `url = "..."` and `token = "..."`
- Other runtime settings (language/timeouts) are controlled via CLI flags or `-c key=value` overrides and are not persisted in the config file.

## CLI (M1+)

- `--config <path>`: override config file (default `~/.echo/config.toml`).
- `--model <name>`: override model.
- `--cd <dir>`: set working directory shown in the status bar.
- `--prompt "<text>"`: initial user message (also positional).
- `ping`: ping configured Anthropic-compatible endpoint and print the returned text.
- `exec <prompt>`: non-interactive JSONL run with session persistence; supports `--session <id>` / `--resume-last`.
- Tool execution is fully automatic (no sandbox/approvals).

## AGENTS.md bootstrap

- Run `/init` in the TUI to ask the agent to scan the repo and draft `AGENTS.md` following the agents.md convention.
- If `AGENTS.md` already exists in the working directory, the command skips without touching the file and posts an info message instead.

## Code layout

- `cmd/echo-cli`: CLI entry.
- `internal/config`: endpoint config loading (url/token).
- `internal/agent`: agent loop + model abstraction (Anthropic-compatible client + streaming).
- `internal/tui`: Bubble Tea UI (transcript + composer + status bar + @ search + slash commands + session picker).
- `internal/tools`: shell + patch helpers (direct execution).
- `internal/search`: file search helper for `@` picker.
- `internal/instructions`: AGENTS.md discovery for system prompts.
- `internal/session`: session storage/resume for exec/TUI.

## Roadmap

- M2: command tool, apply_patch, file search picker, slash commands overlays (basic versions implemented).
- M3: richer exec mode (JSONL parity), session picker, AGENTS.md discovery (basic support in place).
- M4: MCP client/server, notifications, ZDR.
