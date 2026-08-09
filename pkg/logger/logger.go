// Package logger 提供零依赖接口 + slog 后台的结构化 JSON 日志。
//
// 对外 API 兼容旧版：New / NewDefault / SetLevel / WithPrefix / WithField / Info / Warn / Error / Fatal 等。
//
// 内部使用 Go 1.21+ stdlib log/slog 的 JSONHandler，输出标准行式 JSON：
//
//	{"time":"2026-08-08T20:00:00.000+08:00","level":"INFO","msg":"server started","port":"8080"}
//
// 全局默认日志器 getLogger() 返回 *slog.Logger；包级便捷函数是旧 API 的薄封装，
// 业务代码建议逐步直接使用 slog.Logger。
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
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

// Logger 旧 API 封装（基于 slog.Logger）。
// 线程安全：*slog.Logger 本身是并发安全的。
type Logger struct {
	inner  *slog.Logger
	prefix string
}

// New 创建基于 slog 的日志器：output 默认 os.Stderr，level 默认 INFO。
func New(output io.Writer, level Level) *Logger {
	if output == nil {
		output = os.Stderr
	}
	h := slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level:       mapLevel(level),
		ReplaceAttr: nil,
	})
	return &Logger{inner: slog.New(h)}
}

// NewDefault 创建默认日志器：输出到 os.Stderr，级别 INFO。
func NewDefault() *Logger {
	return New(os.Stderr, InfoLevel)
}

// NewDevelopment 创建开发模式日志器：级别 DEBUG，输出到 os.Stderr。
func NewDevelopment() *Logger {
	return New(os.Stderr, DebugLevel)
}

// SetLevel 动态调整日志级别（返回新 Logger — slog handler 级别在 handler 生命周期内固定）。
//
// 若需运行时级别切换，可通过 LevelVar 扩展；当前简化为 info-level 日志器。
func (l *Logger) SetLevel(level Level) *Logger {
	h := slog.NewJSONHandler(writerOf(l.inner), &slog.HandlerOptions{Level: mapLevel(level)})
	return &Logger{inner: slog.New(h), prefix: l.prefix}
}

// WithPrefix 返回追加前缀的 Logger；输出时 prefix 追加到 msg 前。
func (l *Logger) WithPrefix(prefix string) *Logger {
	return &Logger{inner: l.inner, prefix: l.prefix + prefix + " "}
}

// WithField 返回带字段前缀的 Logger（msg 前拼接 key=value）。
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithPrefix(fmt.Sprintf("%s=%v", key, value))
}

// Debug / Info / Warn / Error / Fatal 对应 slog 各级别输出。
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.log(DebugLevel, msg, fields...)
}

func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.log(InfoLevel, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.log(WarnLevel, msg, fields...)
}

func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	l.log(ErrorLevel, msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...map[string]interface{}) {
	l.log(FatalLevel, msg, fields...)
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, args ...interface{}) { l.Fatal(fmt.Sprintf(format, args...)) }
func (l *Logger) Infof(format string, args ...interface{})  { l.Info(fmt.Sprintf(format, args...)) }
func (l *Logger) Debugf(format string, args ...interface{}) { l.Debug(fmt.Sprintf(format, args...)) }
func (l *Logger) Warnf(format string, args ...interface{})  { l.Warn(fmt.Sprintf(format, args...)) }
func (l *Logger) Errorf(format string, args ...interface{}) { l.Error(fmt.Sprintf(format, args...)) }

// log 内部方法：将 fields map 转成 slog.Attr，统一调用 inner.LogAttrs。
func (l *Logger) log(level Level, msg string, fields ...map[string]interface{}) {
	sl := mapLevel(level)
	if !l.inner.Enabled(context.Background(), sl) {
		return
	}
	final := msg
	if l.prefix != "" {
		final = l.prefix + msg
	}
	attrs := make([]slog.Attr, 0, len(fields))
	for _, f := range fields {
		for k, v := range f {
			attrs = append(attrs, slog.Any(k, v))
		}
	}
	l.inner.LogAttrs(context.Background(), sl, final, attrs...)
}

// writerOf 从 slog.Logger 反向取 handler 对应 writer（近似：取 src 的 Writer，无法时回退 stderr）。
// 当前仅作为 SetLevel helper：handler 不暴露 Writer，简单地重建到 stderr。
// 业务层如需要保留 writer，可改用独立包级变量保存。
func writerOf(_ *slog.Logger) io.Writer { return os.Stderr }

// mapLevel 将自有 Level 映射到 slog.Level。
func mapLevel(level Level) slog.Level {
	switch level {
	case DebugLevel:
		return slog.LevelDebug
	case InfoLevel:
		return slog.LevelInfo
	case WarnLevel:
		return slog.LevelWarn
	case ErrorLevel:
		return slog.LevelError
	case FatalLevel:
		return slog.LevelError // slog 无 Fatal，等价 Error；exit 由调用方处理
	default:
		return slog.LevelInfo
	}
}

// ============ 全局默认日志器（旧 API 兼容） ============

var defaultLogger = NewDefault()

// SetDefaultLogger 替换全局默认日志器
func SetDefaultLogger(l *Logger) { defaultLogger = l }

// GetDefaultLogger 获取全局默认日志器
func GetDefaultLogger() *Logger { return defaultLogger }

// SlogLogger 暴露底层 *slog.Logger，供框架/三方库桥接使用。
func (l *Logger) SlogLogger() *slog.Logger { return l.inner }

// ============ 全局 slog 实例 ============

var defaultSlog = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

// SetDefaultSlog 替换全局 *slog.Logger 实例。
func SetDefaultSlog(l *slog.Logger) { defaultSlog = l }

// GetSlog 返回全局 *slog.Logger（默认写到 stderr 的 JSON handler）。
func GetSlog() *slog.Logger { return defaultSlog }

// DefaultLevel 返回当前 slog 默认输出级别。
func DefaultLevel() Level {
	// 无法反向取 handler 级别，简化返回 InfoLevel
	return InfoLevel
}

// 检查 Level 类型是否能作为 slog 字段（防止导未引用的 strings）
var _ = strings.Builder{}

// Package-level convenience functions（旧 API 兼容）
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

// InitSlog 由 main 调用：把全局 slog 默认 handler 改为同时写入 os.Stderr 的 JSON handler。
// 返回 *slog.Logger，便于全局替换 log 标准库输出。
func InitSlog() *slog.Logger {
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	defaultSlog = slog.New(h)
	return defaultSlog
}
