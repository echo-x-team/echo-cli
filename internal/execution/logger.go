package execution

import (
	"io"
	"sync"

	"echo-cli/internal/logger"
)

// DefaultErrorLogPath runTask 错误日志默认路径。
const DefaultErrorLogPath = "logs/error.log"

var (
	// log 复用全局 logger。
	log = logger.Named("engine")

	// llmLog 标记 LLM 相关日志。
	llmLog = logger.Named("llm")

	// errorLog 专用错误日志。
	errorLog = logger.Named("error")

	errorLogConfigured bool
	errorLogMu         sync.Mutex
	errorLogCloser     io.Closer
	errorLogPath       string
)

// SetupErrorLog 配置 runTask 错误日志文件，返回 closer 及实际路径。
// 若 logPath 为空，则使用 DefaultErrorLogPath。
func SetupErrorLog(logPath string) (io.Closer, string, error) {
	errorLogMu.Lock()
	defer errorLogMu.Unlock()

	if errorLogConfigured {
		return errorLogCloser, errorLogPath, nil
	}
	if logPath == "" {
		logPath = DefaultErrorLogPath
	}
	entry, closer, resolved, err := logger.SetupComponentFile("error", logPath)
	errorLogConfigured = true
	errorLogPath = resolved
	if err != nil {
		return nil, resolved, err
	}
	if entry != nil {
		errorLog = entry
	}
	errorLogCloser = closer
	return closer, resolved, nil
}

func ensureErrorLogger(logPath string) {
	errorLogMu.Lock()
	configured := errorLogConfigured
	errorLogMu.Unlock()
	if configured {
		return
	}
	resolved := logPath
	if resolved == "" {
		resolved = DefaultErrorLogPath
	}
	if _, _, err := SetupErrorLog(logPath); err != nil {
		logger.Named("error").Warnf("failed to initialize error log (%s): %v", resolved, err)
	}
}

// CloseErrorLog 关闭错误日志文件句柄（如已初始化）。
func CloseErrorLog() {
	errorLogMu.Lock()
	defer errorLogMu.Unlock()
	if errorLogCloser != nil {
		_ = errorLogCloser.Close()
		errorLogCloser = nil
	}
}
