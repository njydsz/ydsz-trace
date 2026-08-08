// Package logger 提供轻量级结构化日志，基于标准库 log 实现，
// 不依赖第三方库，提供类似 zap 的风格接口，便于后续平滑升级到 zap。
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

var levelNames = map[Level]string{
	DebugLevel: "DEBUG",
	InfoLevel:  "INFO",
	WarnLevel:  "WARN",
	ErrorLevel: "ERROR",
	FatalLevel: "FATAL",
}

// Logger 结构化日志器
type Logger struct {
	mu     sync.Mutex
	output io.Writer
	level  Level
	prefix string
}

// New 创建新的日志器
func New(output io.Writer, level Level) *Logger {
	if output == nil {
		output = os.Stderr
	}
	return &Logger{
		output: output,
		level:  level,
	}
}

// NewDefault 创建默认日志器（输出到 stderr，级别 INFO）
func NewDefault() *Logger {
	return New(os.Stderr, InfoLevel)
}

// NewDevelopment 创建开发模式日志器（DEBUG 级别）
func NewDevelopment() *Logger {
	return New(os.Stderr, DebugLevel)
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// WithPrefix 添加日志前缀
func (l *Logger) WithPrefix(prefix string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{
		output: l.output,
		level:  l.level,
		prefix: l.prefix + prefix + " ",
	}
}

// WithField 添加字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithPrefix(fmt.Sprintf("%s=%v", key, value))
}

// outputEntry 输出日志条目
func (l *Logger) outputEntry(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := map[string]interface{}{
		"time":  time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		"level": levelNames[level],
		"msg":   l.prefix + msg,
	}

	for k, v := range fields {
		entry[k] = v
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.output, `{"time":"%s","level":"ERROR","msg":"日志序列化失败: %v"}`+"\n", time.Now().Format("2006-01-02T15:04:05"), err)
		return
	}

	l.output.Write(data)
	l.output.Write([]byte("\n"))
}

// Debug 调试日志
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.outputEntry(DebugLevel, msg, f)
}

// Info 信息日志
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.outputEntry(InfoLevel, msg, f)
}

// Warn 警告日志
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.outputEntry(WarnLevel, msg, f)
}

// Error 错误日志
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.outputEntry(ErrorLevel, msg, f)
}

// Fatal 致命日志（触发后退出）
func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.outputEntry(FatalLevel, msg, f)
	os.Exit(1)
}

// Fatalf 格式化致命日志
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(fmt.Sprintf(format, args...))
}

// Infof 格式化信息日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Debugf 格式化调试日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Warnf 格式化警告日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf 格式化错误日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// ============ 全局默认日志器 ============

var defaultLogger = NewDefault()

// SetDefaultLogger 设置全局默认日志器
func SetDefaultLogger(l *Logger) {
	defaultLogger = l
}

// GetDefaultLogger 获取全局默认日志器
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Package-level convenience functions
func Debug(msg string, fields ...map[string]interface{}) { defaultLogger.Debug(msg, fields...) }
func Info(msg string, fields ...map[string]interface{})  { defaultLogger.Info(msg, fields...) }
func Warn(msg string, fields ...map[string]interface{})  { defaultLogger.Warn(msg, fields...) }
func Error(msg string, fields ...map[string]interface{}) { defaultLogger.Error(msg, fields...) }
func Fatal(msg string, fields ...map[string]interface{}) { defaultLogger.Fatal(msg, fields...) }

func Infof(format string, args ...interface{})  { defaultLogger.Infof(format, args...) }
func Debugf(format string, args ...interface{}) { defaultLogger.Debugf(format, args...) }
func Warnf(format string, args ...interface{})  { defaultLogger.Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { defaultLogger.Errorf(format, args...) }
func Fatalf(format string, args ...interface{}) { defaultLogger.Fatalf(format, args...) }
