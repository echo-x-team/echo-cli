# Repository Guidelines（请用中文回复）

## 项目结构
- `cmd/echo-cli`：CLI 入口（TUI、exec）。
- `internal/agent`：模型客户端封装；统一负责工具注册（tools）与 Anthropic `tool_use/tool_result` 交互。
- `internal/tools`：工具抽象与执行（无审批/无沙箱）；`dispatcher/` 负责接收执行请求并产出工具事件。
- `internal/tui`：Bubble Tea TUI（输入、对话、事件 pane、slash 命令）。
- 其他：`instructions`（AGENTS 发现）、`config`、`events`、`search`、`session`、`model/openai`。

## 构建与运行
- `go test ./...`：运行全部测试。
- `gofmt -w ./...`：格式化 Go 代码（必须）。
- `go run ./cmd/echo-cli --prompt "你好"`：启动 TUI。
- `go run ./cmd/echo-cli exec --prompt "任务"`：非交互 exec。

## 代码风格
- 使用 `gofmt`；Go 命名惯例（类型/函数 CamelCase，局部 mixedCaps）。
- 事件命名对齐 exec JSONL：`item.*`，工具类型 `command_execution`、`file_change`、`file_read`、`file_search`。
- 日志统一使用 `logger` 模块，禁止引入其他日志实现，保持系统风格一致。

## 测试规范
- 优先表驱动测试，标准库 `testing`。
- 测试命名 `TestXxx`，辅助方法放 `_test.go`。
- 变更前运行 `go test ./...`；改动工具解析/dispatcher/沙箱路径校验时补充针对性测试。

## 提交与 PR
- 提交信息用简洁祈使句（例：“Refactor tool call pipeline”）。
- PR 写明范围、测试、审批/沙箱影响，提供复现命令（尤其 TUI 改动）。
- 关联 issue；TUI 改动附截图/录屏。

## Agent 提示
- 本项目不启用沙箱与审批：工具调用一律全自动直接执行。
- 生成 `command_execution` 的命令必须无提示/无交互：优先使用 `--yes/-y/--non-interactive/--force`、显式参数或 `CI=1`；npm 脚手架优先 `npx --yes create-vue@latest <dir> -- <flags>`，或 `npm_config_yes=true CI=1 npm create ...`；仅在确无开关时才用 `printf 'y\\n' | ...`/`yes | ...` 兜底。
- 新增工具务必发出 `item.*` 事件，并贯通 TUI/exec。
- 不考虑前向兼容性：不要为了“未来可能的协议/接口”保留旧分支、旧结构或兼容层（例如 marker/文本解析、别名映射、双协议并行）。
- 全局只保留一套实现方案：同一能力（尤其工具调用链路）必须收敛到单一路径；升级时直接替换并同步删除旧代码/旧测试/旧文档入口，保持架构精简。
- 本文件若流程变更需同步更新。
- 事件队列约定：数据提交一律进入 SQ，执行引擎生成结果一律通过 EQ 发布；REPL 监听 EQ 变化并调用 `internal/tui` 渲染模块做增量渲染。
- 接到任务后先在 `TODO.md` 拆解步骤并按顺序执行；完成项先勾选，确认已执行后删除该条目，再开展/总结后续工作。
