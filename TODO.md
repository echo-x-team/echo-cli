# Pending Work

- [x] 对齐 `docs/codex_render_module.md` 重建 `internal/tui` 渲染模块（Renderable 抽象、布局容器、行工具、Bash 高亮）。
- [x] 在 `internal/repl` 监听 EQ 队列，调用新的 TUI 渲染模块做增量渲染；确保数据提交统一走 SQ，执行结果统一走 EQ。
- [x] 更新 `AGENTS.md` 强调 SQ/EQ 队列规范和 REPL 渲染路径。
