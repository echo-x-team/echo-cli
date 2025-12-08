# Pending Work

- [x] 对齐 `docs/codex_render_module.md` 重建 `internal/tui` 渲染模块（Renderable 抽象、布局容器、行工具、Bash 高亮）。
- [x] 在 `internal/repl` 监听 EQ 队列，调用新的 TUI 渲染模块做增量渲染；确保数据提交统一走 SQ，执行结果统一走 EQ。
- [x] 更新 `AGENTS.md` 强调 SQ/EQ 队列规范和 REPL 渲染路径。
- [x] 参照 `docs/codex_render_no_flicker.md` 审视现有渲染链路，梳理当前闪烁成因与缺口。
- [x] 为渲染层引入高性能视口组件，减少重绘范围并整合到现有布局/渲染流程。
- [x] 为视口组件补充表驱动测试，覆盖滚动、差分与多宽字符场景。
- [x] 运行 `go test ./...` 确认变更无回归。
