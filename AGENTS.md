# Repository Guidelines（请用中文回复）

## 项目结构
- `cmd/echo-cli`：CLI 入口（TUI、exec）。
- `internal/agent`：模型客户端封装，发布工具调用标记。
- `internal/tools`：工具抽象与执行；`engine/` 负责审批与沙箱；`dispatcher/` 负责模型标记转工具请求；`markers` 解析模型输出。
- `internal/tui`：Bubble Tea TUI（输入、对话、事件 pane、slash 命令）。
- `internal/sandbox`：沙箱运行器（seatbelt/landlock 包装、路径校验）。
- 其他：`instructions`（AGENTS 发现）、`config`、`policy`、`events`、`search`、`session`、`model/openai`。

## 构建与运行
- `go test ./...`：运行全部测试。
- `gofmt -w ./...`：格式化 Go 代码（必须）。
- `go run ./cmd/echo-cli --prompt "你好"`：启动 TUI。
- `go run ./cmd/echo-cli exec --prompt "任务"`：非交互 exec。

## 代码风格
- 使用 `gofmt`；Go 命名惯例（类型/函数 CamelCase，局部 mixedCaps）。
- 事件命名对齐 exec JSONL：`item.*`、`approval.*`，工具类型 `command_execution`、`file_change`、`file_read`、`file_search`。
- 沙箱运行器必须传入 workspace roots；不确定时宁可拒绝。
- 日志统一使用 `logger` 模块，禁止引入其他日志实现，保持系统风格一致。

## 测试规范
- 优先表驱动测试，标准库 `testing`。
- 测试命名 `TestXxx`，辅助方法放 `_test.go`。
- 变更前运行 `go test ./...`；改动工具解析/dispatcher/沙箱路径校验时补充针对性测试。

## 提交与 PR
- 提交信息用简洁祈使句（例：“Add dispatcher for tool markers”）。
- PR 写明范围、测试、审批/沙箱影响，提供复现命令（尤其 TUI 改动）。
- 关联 issue；TUI 改动附截图/录屏。

## Agent 提示
- 默认尊重沙箱（read-only/workspace-write），勿随意放宽。
- 新增工具务必发出 `approval.requested/approval.completed/item.*` 事件，并贯通 TUI/exec。
- 本文件若流程变更需同步更新。
- 事件队列约定：数据提交一律进入 SQ，执行引擎生成结果一律通过 EQ 发布；REPL 监听 EQ 变化并调用 `internal/tui` 渲染模块做增量渲染。
- 接到任务后先在 `TODO.md` 拆解步骤并按顺序执行，逐项勾选直至清单完成，再开展/总结后续工作。
