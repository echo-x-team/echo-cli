package tools

import (
	"io"
	"sync"

	"echo-cli/internal/logger"
)

// DefaultToolsLogPath 工具调用日志的默认路径。
const DefaultToolsLogPath = "logs/tools.log"

var (
	toolsLog           = logger.Named("tools")
	toolsLogConfigured bool
	toolsLogMu         sync.Mutex
	toolsLogCloser     io.Closer
	toolsLogPath       string
)

// SetupToolsLog 配置工具调用专用日志，返回文件 closer 及实际路径。
// 若 logPath 为空，则使用 DefaultToolsLogPath。
// 多次调用只会在首次生效。
func SetupToolsLog(logPath string) (io.Closer, string, error) {
	toolsLogMu.Lock()
	defer toolsLogMu.Unlock()

	if toolsLogConfigured {
		return toolsLogCloser, toolsLogPath, nil
	}
	if logPath == "" {
		logPath = DefaultToolsLogPath
	}

	entry, closer, resolved, err := logger.SetupComponentFile("tools", logPath)
	toolsLogConfigured = true
	toolsLogPath = resolved
	if err != nil {
		return nil, resolved, err
	}
	if entry != nil {
		toolsLog = entry
	}
	toolsLogCloser = closer
	return closer, resolved, nil
}

func ensureToolsLogger() {
	toolsLogMu.Lock()
	configured := toolsLogConfigured
	toolsLogMu.Unlock()
	if configured {
		return
	}
	if _, _, err := SetupToolsLog(DefaultToolsLogPath); err != nil {
		logger.Named("tools").Warnf("failed to initialize tools log (%s): %v", DefaultToolsLogPath, err)
	}
}

// CloseToolsLog 关闭工具日志文件句柄（如已初始化）。
func CloseToolsLog() {
	toolsLogMu.Lock()
	defer toolsLogMu.Unlock()
	if toolsLogCloser != nil {
		_ = toolsLogCloser.Close()
		toolsLogCloser = nil
	}
}
