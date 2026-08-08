// Package logger 提供零依赖的结构化日志，输出 JSON 格式便于日志采集。
//
// 设计定位：
//   - 在标准 log 基础上封装，零第三方依赖
//   - 输出结构化 JSON，便于 ELK / ClickHouse 采集解析
//   - 接口风格贴近 zap（WithField / WithPrefix），便于后续平滑升级
//
// 输出示例：
//
//	{"time":"2026-08-08T20:00:00.000+08:00","level":"INFO","msg":"server started","port":"8080"}
//
// 全局默认日志器通过 SetDefaultLogger 替换；业务代码建议使用包级便捷函数。
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level 日志级别，数值越大越严重。
type Level int

const (
	// DebugLevel 调试信息，生产环境通常关闭
	DebugLevel Level = iota
	// InfoLevel 常规信息（启动、配置、关键事件）
	InfoLevel
	// WarnLevel 警告（可恢复异常、降级）
	WarnLevel
	// ErrorLevel 错误（业务失败、外部依赖异常）
	ErrorLevel
	// FatalLevel 致命错误（输出后调用 os.Exit(1)）
	FatalLevel
)

// levelNames 日志级别到字符串的映射。
var levelNames = map[Level]string{
	DebugLevel: "DEBUG",
	InfoLevel:  "INFO",
	WarnLevel:  "WARN",
	ErrorLevel: "ERROR",
	FatalLevel: "FATAL",
}

// Logger 结构化日志器实例。
//
// 并发安全：通过互斥锁保护 output;level;prefix 写操作。
type Logger struct {
	mu     sync.Mutex
	output io.Writer
	level  Level
	prefix string
}

// New 创建新的日志器。
//
// 参数：
//   - output: 输出目标，nil 默认 os.Stderr
//   - level: 最低输出级别
func New(output io.Writer, level Level) *Logger {
	if output == nil {
		output = os.Stderr
	}
	return &Logger{
		output: output,
		level:  level,
	}
}

// NewDefault 创建默认日志器：输出到 os.Stderr，级别 INFO。
func NewDefault() *Logger {
	return New(os.Stderr, InfoLevel)
}

// NewDevelopment 创建开发模式日志器：级别 DEBUG，输出到 os.Stderr。
func NewDevelopment() *Logger {
	return New(os.Stderr, DebugLevel)
}

// SetLevel 动态调整日志级别（并发安全）。
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// WithPrefix 返回带指定前缀的新 Logger（不影响原实例）。
//
// 适用场景：为某一类日志增加模块标识，例如 WithPrefix("[Auth]") 。
func (l *Logger) WithPrefix(prefix string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{
		output: l.output,
		level:  l.level,
		prefix: l.prefix + prefix + " ",
	}
}

// WithField 返回带字段前缀的新 Logger，输出时 key=value 作为 msg 前缀。
//
// 如需输出为 JSON 字段而非前缀，可改造 outputEntry 支持 fields 写入 JSON。
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithPrefix(fmt.Sprintf("%s=%v", key, value))
}

// outputEntry 序列化并写入一条日志（并发安全，级别过滤）。
//
// 序列化失败时输出一条专用的错误日志，避免静默丢失。
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
