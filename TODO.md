# Pending Work

- [x] 对齐 `docs/codex_render_module.md` 重建 `internal/tui` 渲染模块（Renderable 抽象、布局容器、行工具、Bash 高亮）。
- [x] 在 `internal/repl` 监听 EQ 队列，调用新的 TUI 渲染模块做增量渲染；确保数据提交统一走 SQ，执行结果统一走 EQ。
- [x] 更新 `AGENTS.md` 强调 SQ/EQ 队列规范和 REPL 渲染路径。
- [x] 参照 `docs/codex_render_no_flicker.md` 审视现有渲染链路，梳理当前闪烁成因与缺口。
- [x] 为渲染层引入高性能视口组件，减少重绘范围并整合到现有布局/渲染流程。
- [x] 为视口组件补充表驱动测试，覆盖滚动、差分与多宽字符场景。
- [x] 运行 `go test ./...` 确认变更无回归。
- [x] 升级 Bubble Tea 至 v1.3.10（同步依赖版本、`go mod tidy`）。
- [x] 按 v1.3.10 最新写法重构 `internal/tui`（Program 初始化、视口/输入适配）。
- [x] 运行 `gofmt -w ./...` 与 `go test ./...` 验证。
- [x] 停用高性能渲染路径，改用 Bubble Tea 默认渲染器并整理视口封装。
- [x] 重命名 `internal/tui/render/high_performance_viewport.go` 为 `viewport.go` 并同步相关引用/测试文件。
- [x] 运行 `gofmt -w ./...` 与 `go test ./...` 验证。
