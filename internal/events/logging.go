package events

import (
	"io"

	"echo-cli/internal/logger"
)

// 默认的 SQ/EQ 日志文件路径。
const (
	DefaultSQLogPath = "logs/sq.log"
	DefaultEQLogPath = "logs/eq.log"
)

// log 复用全局 logger，标记事件组件。
var log = logger.Named("events")

func newQueueLogger(component, path string) (*logger.LogEntry, io.Closer) {
	if path == "" {
		return logger.Named(component), nil
	}
	entry, closer, _, err := logger.SetupComponentFile(component, path)
	if err != nil {
		log.Warnf("failed to set up %s log file (%s): %v", component, path, err)
		return logger.Named(component), nil
	}
	return entry, closer
}
