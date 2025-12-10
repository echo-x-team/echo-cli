# echo-cli (Go + Bubble Tea) — WIP

This is a Go implementation of the echo-cli tool modeled after @echo-rs. It uses Bubble Tea for the TUI and mirrors the MRD features. Current status: **M1 scaffold** (CLI entry, config loader, basic TUI, Echo Team chat without tools).

## Quickstart

```bash
cd echo-cli
go run ./cmd/echo-cli --prompt "hello"
```

Environment:

- `OPENAI_API_KEY` — optional; if unset, the app echoes responses for local testing.
- Config file (optional): `~/.echo/config.toml` or `--config path`. See `internal/config/config.go` for fields.
- `default_language` in config chooses the preferred response language (defaults to Chinese when omitted).

## CLI (M1+)

- `--config <path>`: override config file (default `~/.echo/config.toml`).
- `--model <name>`: override model.
- `--cd <dir>`: set working directory in status bar.
- `--prompt "<text>"`: initial user message (also positional).
- `exec <prompt>`: non-interactive JSONL run with session persistence; supports `--session <id>` / `--resume-last`.
- Approval/sandbox: honors `sandbox_mode` and `approval_policy` (read-only blocks writes/commands; on-request/untrusted prompts).

## AGENTS.md bootstrap

- Run `/init` in the TUI to ask the agent to scan the repo and draft `AGENTS.md` following the agents.md convention.
- If `AGENTS.md` already exists in the working directory, the command skips without touching the file and posts an info message instead.

## Code layout

-- `cmd/echo-cli`: CLI entry.
-- `internal/config`: config loading/CLI overrides.
-- `internal/agent`: agent loop + model abstraction (including Echo Team client with streaming support).
-- `internal/tui`: Bubble Tea UI (transcript + composer + status bar + @ search + slash commands + approvals + session picker).
-- `internal/policy`: sandbox/approval gating.
-- `internal/tools`: shell + patch helpers (sandbox plumbing stubbed).
-- `internal/search`: file search helper for `@` picker.
-- `internal/instructions`: AGENTS.md discovery for system prompts.
-- `internal/session`: session storage/resume for exec/TUI.

## Roadmap

- M2: sandboxed command tool, apply_patch, file search picker, slash commands overlays (basic versions implemented).
- M3: richer exec mode (JSONL parity), session picker, AGENTS.md discovery (basic support in place).
- M4: MCP client/server, notifications, ZDR, execpolicy.
