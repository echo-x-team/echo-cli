package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger/LogEntry/Fields 暴露底层类型，避免调用方直接依赖 logrus 包。
type Logger = logrus.Logger
type LogEntry = logrus.Entry
type Fields = logrus.Fields

// DefaultLogPath 默认日志文件路径。
const DefaultLogPath = "logs/echo-cli.log"

var rootLogger = logrus.StandardLogger()

// Configure 设置全局日志格式与 caller 输出。
func Configure() {
	root().SetReportCaller(true)
	root().SetFormatter(PlainFormatter{})
}

// SetupFile 将全局日志输出重定向到指定路径（默认 logs/echo-cli.log）。
// 返回底层文件的 closer 以便调用方清理。
func SetupFile(logPath string) (io.Closer, string, error) {
	if logPath == "" {
		logPath = DefaultLogPath
	}
	f, resolved, err := openLogFile(logPath)
	if err != nil {
		return nil, "", err
	}
	root().SetOutput(f)
	return f, resolved, nil
}

// SetupComponentFile 创建独立的 logger，输出到指定文件并附加 component 字段。
// 返回 entry、文件 closer 及实际路径。
func SetupComponentFile(component, logPath string) (*LogEntry, io.Closer, string, error) {
	f, resolved, err := openLogFile(logPath)
	if err != nil {
		return nil, nil, "", err
	}
	l := logrus.New()
	l.SetReportCaller(true)
	l.SetFormatter(PlainFormatter{})
	l.SetOutput(f)

	entry := logrus.NewEntry(l)
	if component != "" {
		entry = entry.WithField("component", component)
	}
	return entry, f, resolved, nil
}

// Root 返回全局共享的 logger。
func Root() *Logger {
	return root()
}

// SetRoot 覆盖全局 logger，传入 nil 时重置为标准 logger。
func SetRoot(l *Logger) {
	if l == nil {
		l = logrus.StandardLogger()
	}
	rootLogger = l
}

// Entry 返回未附加字段的全局入口。
func Entry() *LogEntry {
	return logrus.NewEntry(root())
}

// Named 为指定组件创建入口，统一 component 字段。
func Named(component string) *LogEntry {
	entry := Entry()
	if component != "" {
		entry = entry.WithField("component", component)
	}
	return entry
}

// Info 输出 Info 日志。
func Info(args ...any) {
	root().Info(args...)
}

// Infof 输出格式化 Info 日志。
func Infof(format string, args ...any) {
	root().Infof(format, args...)
}

// Warnf 输出格式化 Warn 日志。
func Warnf(format string, args ...any) {
	root().Warnf(format, args...)
}

// Fatalf 输出格式化 Fatal 日志并退出。
func Fatalf(format string, args ...any) {
	root().Fatalf(format, args...)
}

func root() *logrus.Logger {
	if rootLogger == nil {
		rootLogger = logrus.StandardLogger()
	}
	return rootLogger
}

// PlainFormatter 统一输出格式：[timestamp] [LEVEL] [component] caller message fields。
type PlainFormatter struct{}

// Format 实现 logrus Formatter。
func (PlainFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry == nil {
		return []byte{}, nil
	}
	timestamp := entry.Time.UTC().Format(time.RFC3339Nano)
	level := strings.ToUpper(entry.Level.String())
	component := ""
	if val, ok := entry.Data["component"].(string); ok && val != "" {
		component = val
	}
	caller := formatCaller(entry)
	fields := formatFields(entry.Data)

	parts := make([]string, 0, 6)
	if caller != "" {
		parts = append(parts, caller)
	}
	parts = append(parts, fmt.Sprintf("[%s]", timestamp))
	parts = append(parts, fmt.Sprintf("[%s]", level))
	if component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", component))
	}
	parts = append(parts, entry.Message)
	if fields != "" {
		parts = append(parts, fields)
	}
	return []byte(strings.Join(parts, " ") + "\n"), nil
}

func formatCaller(entry *logrus.Entry) string {
	if entry == nil {
		return ""
	}
	if entry.HasCaller() && entry.Caller != nil {
		return fmt.Sprintf("%s:%d", shortenFilePath(entry.Caller.File), entry.Caller.Line)
	}
	if caller, ok := entry.Data["caller"].(string); ok && caller != "" {
		return caller
	}
	return ""
}

func formatFields(fields logrus.Fields) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k == "component" || k == "caller" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, fields[k]))
	}
	return strings.Join(parts, " ")
}

func shortenFilePath(file string) string {
	file = filepath.ToSlash(file)
	if idx := strings.Index(file, "/internal/"); idx != -1 {
		return file[idx+1:]
	}
	if idx := strings.Index(file, "/cmd/"); idx != -1 {
		return file[idx+1:]
	}
	if idx := strings.Index(file, "/echo-cli/"); idx != -1 {
		return file[idx+len("/echo-cli/"):]
	}
	return filepath.Base(file)
}

func openLogFile(logPath string) (*os.File, string, error) {
	if logPath == "" {
		logPath = DefaultLogPath
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, "", err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", err
	}
	return f, logPath, nil
}
