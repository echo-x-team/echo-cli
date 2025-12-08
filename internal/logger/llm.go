package logger

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// LLMMessage 表示一次请求中的对话消息。
type LLMMessage struct {
	Role    string
	Content string
}

// LLMLogger 负责输出与 LLM 交互的请求、响应与错误信息。
type LLMLogger interface {
	Request(model string, messages []LLMMessage, attempt int)
	Response(model string, content string, attempt int)
	StreamChunk(model string, chunk string, index int)
	StreamComplete(model string, attempt int)
	Error(model string, err error, attempt int)
}

// LLMLog 是全局唯一的 LLM 日志器实例。
var LLMLog LLMLogger = NewLLMLogger(nil)

// GlobalLLMLogger 返回全局唯一的 LLM 日志实例。
func GlobalLLMLogger() LLMLogger {
	return LLMLog
}

// SetGlobalLLMLogger 覆盖全局 LLM 日志实例，传入 nil 将重置为默认实现。
func SetGlobalLLMLogger(logger LLMLogger) {
	if logger == nil {
		logger = NewLLMLogger(nil)
	}
	LLMLog = logger
}

// StdLLMLogger 使用 logrus 输出日志。
type StdLLMLogger struct {
	logger *logrus.Entry
}

// NewLLMLogger 构造默认的 LLM 日志记录器。
func NewLLMLogger(l *Logger) *StdLLMLogger {
	if l == nil {
		l = root()
	}
	l.SetFormatter(PlainFormatter{})
	l.SetReportCaller(true)
	return &StdLLMLogger{logger: logrus.NewEntry(l).WithField("component", "llm")}
}

// Request 记录一次请求的上下文。
func (l *StdLLMLogger) Request(model string, messages []LLMMessage, attempt int) {
	l.printf(logrus.InfoLevel, "-> request attempt=%d model=%s messages=%d", attempt, model, len(messages))
	for i, msg := range messages {
		l.printf(logrus.InfoLevel, "-> message[%d] role=%s content=%s", i, msg.Role, sanitize(msg.Content))
	}
}

// Response 记录一次非流式响应。
func (l *StdLLMLogger) Response(model string, content string, attempt int) {
	l.printf(logrus.InfoLevel, "<- response attempt=%d model=%s text=%s", attempt, model, sanitize(content))
}

// StreamChunk 记录流式响应的单个分片。
func (l *StdLLMLogger) StreamChunk(model string, chunk string, index int) {
	l.printf(logrus.InfoLevel, "<- chunk model=%s seq=%d text=%s", model, index, sanitize(chunk))
}

// StreamComplete 记录流式响应完成。
func (l *StdLLMLogger) StreamComplete(model string, attempt int) {
	l.printf(logrus.InfoLevel, "<- stream completed attempt=%d model=%s", attempt, model)
}

// Error 记录请求错误。
func (l *StdLLMLogger) Error(model string, err error, attempt int) {
	l.printf(logrus.ErrorLevel, "!! error attempt=%d model=%s err=%v", attempt, model, err)
}

// NoopLLMLogger 忽略所有日志输出。
type NoopLLMLogger struct{}

// NewNoopLLMLogger 创建一个不输出的记录器。
func NewNoopLLMLogger() NoopLLMLogger {
	return NoopLLMLogger{}
}

func (NoopLLMLogger) Request(model string, messages []LLMMessage, attempt int) {}
func (NoopLLMLogger) Response(model string, content string, attempt int)       {}
func (NoopLLMLogger) StreamChunk(model string, chunk string, index int)        {}
func (NoopLLMLogger) StreamComplete(model string, attempt int)                 {}
func (NoopLLMLogger) Error(model string, err error, attempt int)               {}

// Request 记录一次 LLM 请求。
func Request(model string, messages []LLMMessage, attempt int) {
	if LLMLog != nil {
		LLMLog.Request(model, messages, attempt)
	}
}

// StreamChunk 记录流式响应的分片。
func StreamChunk(model string, chunk string, index int) {
	if LLMLog != nil {
		LLMLog.StreamChunk(model, chunk, index)
	}
}

// StreamComplete 记录流式响应完成。
func StreamComplete(model string, attempt int) {
	if LLMLog != nil {
		LLMLog.StreamComplete(model, attempt)
	}
}

// Error 记录请求错误。
func Error(model string, err error, attempt int) {
	if LLMLog != nil {
		LLMLog.Error(model, err, attempt)
	}
}

func (l *StdLLMLogger) printf(level logrus.Level, format string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	if !l.logger.Logger.IsLevelEnabled(level) {
		return
	}

	msg := fmt.Sprintf(format, args...)
	caller := findCaller()
	entry := l.logger
	if caller != "" {
		entry = entry.WithField("caller", caller)
	}
	entry.Log(level, msg)
}

func sanitize(text string) string {
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
}

func findCaller() string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if frame.File != "" && !strings.Contains(frame.File, "llm.go") {
			return fmt.Sprintf("%s:%d", shortenFilePath(frame.File), frame.Line)
		}
		if !more {
			break
		}
	}
	return ""
}
