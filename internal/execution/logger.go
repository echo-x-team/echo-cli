package execution

import "echo-cli/internal/logger"

// log 复用全局 logger。
var log = logger.Named("engine")

// llmLog 标记 LLM 相关日志。
var llmLog = logger.Named("llm")
