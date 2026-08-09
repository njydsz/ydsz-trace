// Package source 定义日志采集源抽象层，屏蔽底层文件/Docker/K8s API 差异。
//
// 设计目标：
//   - 统一读取接口 Source.Read / Source.Tail，不同模式逻辑上完全一致
//   - 自动发现接口 Source.Discover，动态感知容器/Pod 上下线
//   - 进度接口 Source.DiscoveryEvent，为注册中心提供变更通知
//
// 调用方（logc）通过 Source 接口操作日志，不直接依赖具体实现。
package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Source 定义日志采集源的统一操作接口。
//
// 实现该接口的类型包括：
//   - FileSource：传统文件系统日志（mode = "file"）
//   - DockerSource：通过 Docker socket 采集（mode = "docker"）
//   - K8sSource：通过 K8s API 采集（mode = "k8s"）
type Source interface {
	// Read 从日志源读取匹配过滤规则的日志行，写入 output。
	//
	// path 在不同模式下含义不同：
	//   - FileSource：宿主机上的日志绝对路径
	//   - DockerSource：容器 ID 或 "name:containerName"
	//   - K8sSource：虚拟路径 "k8s://namespace/pod/container"
	//
	// 返回写入的字节数和可能发生的错误。
	Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error)

	// Tail 实时跟踪日志源，通过 callback 推送每个匹配行。
	//
	// 当 ctx 取消或回调返回错误时退出。
	// followDuration 为非正数时由 ctx 取消控制退出时机。
	Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error

	// Discover 返回当前目标列表的 channel，后续推送变更事件。
	// 首次消费 channel 时会先收到一次全量快照（type=snapshot）。
	// ctx 取消时关闭 channel。
	Discover(ctx context.Context) (<-chan DiscoveryEvent, error)

	// Info 返回此 Source 的摘要信息（用于日志和调试）
	Info() SourceInfo
}

// ScanConfig 日志扫描过滤配置。
// 与 logc/controllers/file.FileReq 字段对齐。
type ScanConfig struct {
	// Key 搜索关键字（空表示不过滤）
	Key string
	// Regex 是否使用正则匹配模式（默认 false = 字符串包含）
	Regex bool
	// Level 日志级别过滤（空=不过滤，可选: DEBUG/INFO/WARN/ERROR/FATAL）
	Level string
	// StartTime 时间范围起始（格式 HH:MM:SS 或 HH:MM，空=不限制）
	StartTime string
	// EndTime 时间范围结束（格式 HH:MM:SS 或 HH:MM，空=不限制）
	EndTime string
	// ContextLines 命中行后追加读取的上下文行数
	ContextLines int64
}

// TailConfig 实时跟踪配置。
type TailConfig struct {
	// Key 搜索关键字
	Key string
	// Regex 是否正则模式
	Regex bool
	// Level 日志级别过滤
	Level string
	// FollowDuration 持续跟踪秒数(非正数表示无限)
	FollowDuration int64
}

// DiscoveryEvent 目标变更事件。
type DiscoveryEvent struct {
	// Type 事件类型：snapshot / add / update / remove
	Type string
	// Targets 受影响的目标列表（snapshot 时为全量）
	Targets []DiscoveryTarget
	// EventTime 事件时间
	EventTime time.Time
}

// DiscoveryTarget 被发现的单个采集目标。
type DiscoveryTarget struct {
	// Identity 稳定唯一标识（如 K8s: pod-uid，Docker: container-id 前缀）
	Identity string
	// DisplayName 展示名称
	DisplayName string
	// SourceType 来源类型：file / docker / k8s
	SourceType string
	// LogPath 此目标的可读路径（根据 SourceType 格式不同）
	LogPath string
	// Labels 扩展标签（如 namespace、pod、container、app name 等）
	Labels map[string]string
}

// SourceInfo 源实例的信息摘要。
type SourceInfo struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	StartedAt   time.Time `json:"startedAt"`
}

// ============ 通用过滤与扫描逻辑 ============

// lineFilter 实现关键字/级别/时间范围/上下文的过滤逻辑。
// 用于各 Source 在逐行读取时判断保留或丢弃该行。
type lineFilter struct {
	key          string
	regex        bool
	re           *regexp.Regexp
	level        string
	hasTimeRange bool
	startSeconds int
	endSeconds   int
	contextLines int64
	curLine      int64
	windowEnd    int64 // 当前上下文窗口结束行号
}

// newLineFilter 根据配置创建过滤器；regex 编译失败返回 error。
func newLineFilter(cfg ScanConfig) (*lineFilter, error) {
	f := &lineFilter{
		key:          cfg.Key,
		regex:        cfg.Regex,
		level:        normalizeLevel(cfg.Level),
		contextLines: cfg.ContextLines,
	}

	if cfg.Regex && cfg.Key != "" {
		var err error
		f.re, err = regexp.Compile(cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("正则编译失败: %w", err)
		}
	}

	if cfg.StartTime != "" || cfg.EndTime != "" {
		f.hasTimeRange = true
		f.startSeconds = parseTimeToSeconds(cfg.StartTime)
		f.endSeconds = parseTimeToSeconds(cfg.EndTime)
	}

	return f, nil
}

// shouldKeep 判断给定日志行是否应写入输出（命中主要条件或在上下文窗口内）。
//
// 命中主要条件的行及其后 contextLines 行均视为应当保留。
func (f *lineFilter) shouldKeep(line string) bool {
	f.curLine++

	// 检查在上下文窗口内
	inWindow := f.windowEnd > 0 && f.curLine <= f.windowEnd
	if inWindow {
		return true
	}

	// 关键字匹配
	keyMatched := false
	if f.regex && f.re != nil {
		keyMatched = f.re.MatchString(line)
	} else if f.key != "" {
		keyMatched = strings.Contains(line, f.key)
	} else {
		keyMatched = true
	}
	if !keyMatched {
		return false
	}

	// 级别过滤
	if f.level != "" {
		levelFound := extractLogLevel(line)
		if !strings.EqualFold(levelFound, f.level) {
			return false
		}
	}

	// 时间范围过滤
	if f.hasTimeRange {
		lineSeconds := extractTimeFromLogLine(line)
		if lineSeconds >= 0 {
			if f.startSeconds > 0 && lineSeconds < f.startSeconds {
				return false
			}
			if f.endSeconds > 0 && lineSeconds > f.endSeconds {
				return false
			}
		}
	}

	// 命中，扩展上下文窗口
	f.windowEnd = f.curLine + f.contextLines
	return true
}

// normalizeLevel 统一日志级别大小写与别名。
func normalizeLevel(level string) string {
	switch strings.ToUpper(level) {
	case "WARN", "WARNING":
		return "WARN"
	case "DEBUG":
		return "DEBUG"
	case "INFO":
		return "INFO"
	case "ERROR":
		return "ERROR"
	case "FATAL":
		return "FATAL"
	default:
		return strings.ToUpper(level)
	}
}

// logLevelPattern 常见日志级别匹配模式。
var logLevelPattern = regexp.MustCompile(`(?i)\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL)\b`)

// extractLogLevel 从日志行中提取级别字符串；未命中返回 ""。
func extractLogLevel(line string) string {
	m := logLevelPattern.FindString(line)
	if m == "" {
		return ""
	}
	return normalizeLevel(m)
}

// parseTimeToSeconds 将 HH:MM:SS 或 HH:MM 格式转为当日秒数；解析失败或空返回 -1。
func parseTimeToSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return -1
	}
	h, err1 := parseIntField(parts[0])
	m, err2 := parseIntField(parts[1])
	if err1 != nil || err2 != nil || h > 23 || m > 59 {
		return -1
	}
	total := h*3600 + m*60
	if len(parts) == 3 {
		sec, err := parseIntField(parts[2])
		if err != nil || sec > 59 {
			return -1
		}
		total += sec
	}
	return total
}

func parseIntField(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// timeFormats 支持的行首时间戳格式。
var timeFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006/01/02 15:04:05",
	"2006/01/02 15:04",
	"15:04:05",
	"15:04",
}

// extractTimeFromLogLine 从行首尝试解析时间戳并返回当日秒数。
// 仅返回当日内的秒数（因为过滤逻辑是限制当天的 HH:MM:SS 范围）。
func extractTimeFromLogLine(line string) int {
	line = strings.TrimSpace(line)
	if len(line) < 8 {
		return -1
	}

	// 仅时间格式 "HH:MM:SS" 和 "HH:MM"
	shortFormats := []string{"15:04:05", "15:04"}
	for _, f := range shortFormats {
		if len(line) >= len(f) {
			if t, err := time.Parse(f, line[:len(f)]); err == nil {
				return t.Hour()*3600 + t.Minute()*60 + t.Second()
			}
		}
	}

	// 日期+时间格式
	longFormats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
	}
	for _, f := range longFormats {
		if len(line) >= len(f) {
			if t, err := time.Parse(f, line[:len(f)]); err == nil {
				return t.Hour()*3600 + t.Minute()*60 + t.Second()
			}
		}
	}
	return -1
}

// ScanAndFilter 从 input 逐行扫描，命中过滤条件的行写入 output。
// 返回实际写入的字节数。
func ScanAndFilter(input io.Reader, cfg ScanConfig, output io.Writer) (int64, error) {
	filter, err := newLineFilter(cfg)
	if err != nil {
		return 0, err
	}
	written, err := scanAndFilter(input, filter, output)
	return written, err
}

// scanAndFilter 使用已构建的 filter 执行扫描。
func scanAndFilter(input io.Reader, filter *lineFilter, output io.Writer) (written int64, err error) {
	reader := bufio.NewReader(input)
	for {
		line, rerr := reader.ReadString('\n')
		if len(line) > 0 {
			if filter.shouldKeep(line) {
				n, werr := output.Write([]byte(line))
				written += int64(n)
				if werr != nil {
					return written, werr
				}
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				return written, rerr
			}
			break
		}
	}
	return written, nil
}

// TailFilter 专为实时跟踪设计的轻量过滤器（不需要上下文窗口折叠）。
type TailFilter struct {
	regex bool
	re    *regexp.Regexp
	key   string
	level string
}

// NewTailFilter 构建 TailFilter；regex 编译失败返回 error。
func NewTailFilter(cfg TailConfig) (*TailFilter, error) {
	f := &TailFilter{
		regex: cfg.Regex,
		key:   cfg.Key,
		level: normalizeLevel(cfg.Level),
	}
	if cfg.Regex && cfg.Key != "" {
		var err error
		f.re, err = regexp.Compile(cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("正则编译失败: %w", err)
		}
	}
	return f, nil
}

// Match 判断日志行是否通过 Tail 过滤。
func (f *TailFilter) Match(line string) bool {
	if f.regex && f.re != nil {
		if !f.re.MatchString(line) {
			return false
		}
	} else if f.key != "" {
		if !strings.Contains(line, f.key) {
			return false
		}
	}
	if f.level != "" {
		if !strings.EqualFold(extractLogLevel(line), f.level) {
			return false
		}
	}
	return true
}
