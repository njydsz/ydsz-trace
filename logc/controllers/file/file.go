// Package file 提供日志文件搜索与下载能力。
//
// 重构后：所有底层读取委托给 source.Source 抽象层。
// 不再直接调用 os.Open() 或文件扫描。
// 安全措施由 Source 实现自身保证（如 rootDir 限制）。
// 本层仅做参数校验、Source 获取、压缩打包。
package file

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/source"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// FileReq 文件查询请求体（与 logs 后端 LogsReq 对齐）。
type FileReq struct {
	Path      string `json:"path"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	// MaxLines 单节点返回行上限（0 表示不限制，用于日志检索分页场景）。
	MaxLines int `json:"maxLines"`
	// Query 可选布尔查询表达式（field:value AND/OR/NOT，隐式 AND）。
	// 与 Key 并用时取交集。空表示不使用布尔查询。
	Query string `json:"query"`
}

// TailReq 实时日志跟踪请求体。
type TailReq struct {
	Path           string `json:"path"`
	Key            string `json:"key"`
	Regex          bool   `json:"regex"`
	Level          string `json:"level"`
	FollowDuration int64  `json:"followDuration"`
}

// safeKeyPattern 合法 key 白名单：字母、数字、连字符、下划线，长度 1-128。
var safeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,128}$`)

// safeRegexPattern 正则模式下允许的字符：可打印 ASCII，长度 1-256。
var safeRegexPattern = regexp.MustCompile(`^[[:print:]]{1,256}$`)

// sanitizeKey 校验 key 参数；regex 模式下放宽校验。
func sanitizeKey(key string, regex bool) (string, bool) {
	if len(key) == 0 {
		return "", false
	}
	if regex {
		if len(key) > 256 {
			return "", false
		}
		if !safeRegexPattern.MatchString(key) {
			return "", false
		}
		return key, true
	}
	if len(key) > 128 {
		return "", false
	}
	if !safeKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

// resolveSource 从 Gin Context 获取激活的 Source；不存在返回 false。
func resolveSource(c *gin.Context) (source.Source, bool) {
	v, exists := c.Get("__source__")
	if !exists {
		return nil, false
	}
	s, ok := v.(source.Source)
	return s, ok
}

// Query 处理日志文件查询请求：委托 Source 读取 → zip 压缩 → 下载。
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	s, ok := resolveSource(c)
	if !ok || s == nil {
		c.JSON(503, gin.H{"error": "source not initialized"})
		return
	}

	var fileReq FileReq
	data, err := c.GetRawData()
	if err != nil {
		log.Printf("读取请求体失败: %v", err)
		c.Status(400)
		return
	}
	if err := json.Unmarshal(data, &fileReq); err != nil {
		log.Printf("JSON解析失败: %v", err)
		c.Status(400)
		return
	}

	// 安全校验
	safeKey, ok := sanitizeKey(fileReq.Key, fileReq.Regex)
	if !ok {
		log.Printf("非法key参数: %s (regex=%v)", fileReq.Key, fileReq.Regex)
		c.Status(400)
		return
	}

	// 委托 Source 读取
	var buf bytes.Buffer
	scanCfg := source.ScanConfig{
		Key:          safeKey,
		Regex:        fileReq.Regex,
		Level:        fileReq.Level,
		StartTime:    fileReq.StartTime,
		EndTime:      fileReq.EndTime,
		ContextLines: fileReq.Line,
		Query:        fileReq.Query,
	}
	written, err := s.Read(c.Request.Context(), fileReq.Path, scanCfg, &buf)
	if err != nil {
		log.Printf("Source.Read 失败: %v", err)
		c.Status(404)
		return
	}
	if written == 0 {
		c.Status(404)
		return
	}

	// 压缩为临时 zip 并返回
	temppath := cfg.StringOr("temppath", "./temp/logc/")
	if err := os.MkdirAll(temppath, 0755); err != nil {
		log.Printf("创建临时目录失败: %v", err)
		c.Status(500)
		return
	}

	// 安全使用 filepath.Join
	tmpName := filepath.Join(temppath, safeKey+".log")
	if err := os.WriteFile(tmpName, buf.Bytes(), 0644); err != nil {
		log.Printf("写入临时文件失败: %v", err)
		c.Status(500)
		return
	}
	defer func() { _ = os.Remove(tmpName) }()

	ip := getLocalIPv4()
	zipPath := filepath.Join(temppath, ip+".zip")
	if err := util.Zip(zipPath, tmpName); err != nil {
		log.Printf("压缩失败: %v", err)
		c.Status(500)
		return
	}
	defer func() { _ = os.Remove(zipPath) }()

	c.Header("Content-Disposition", `attachment; filename="`+filepath.Base(zipPath)+`"`)
	c.File(zipPath)
}

// Tail 实时日志跟踪端点：委托 Source.Tail 通过 SSE 推送新增匹配行。
func Tail(c *gin.Context) {
	var tailReq TailReq
	data, err := c.GetRawData()
	if err != nil {
		log.Printf("读取请求体失败: %v", err)
		c.Status(400)
		return
	}
	if err := json.Unmarshal(data, &tailReq); err != nil {
		log.Printf("JSON解析失败: %v", err)
		c.Status(400)
		return
	}

	s, ok := resolveSource(c)
	if !ok || s == nil {
		c.JSON(503, gin.H{"error": "source not initialized"})
		return
	}

	if strings.Contains(tailReq.Path, "..") && s.Info().Type == string(source.SourceTypeFile) {
		// file 模式额外兜底：禁止 ".." 路径遍历
		log.Printf("非法path参数(包含..): %s", tailReq.Path)
		c.Status(400)
		return
	}

	safeKey, ok := sanitizeKey(tailReq.Key, tailReq.Regex)
	if !ok {
		log.Printf("非法key参数: %s", tailReq.Key)
		c.Status(400)
		return
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	tailCfg := source.TailConfig{
		Key:            safeKey,
		Regex:          tailReq.Regex,
		Level:          tailReq.Level,
		FollowDuration: tailReq.FollowDuration,
	}

	c.SSEvent("connected", "tracking started")
	c.Writer.Flush()

	// 回调把匹配的行推送给客户端
	callback := func(line string) error {
		c.SSEvent("line", line)
		c.Writer.Flush()
		return nil
	}

	if err := s.Tail(c.Request.Context(), tailReq.Path, tailCfg, callback); err != nil {
		log.Printf("Source.Tail error: %v", err)
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}

	tailCleanupOutput() // 回调完成后清理临时输出资源
}

// searchResponse 在线分页搜索单节点响应。
//
// Lines：匹配到的文本行切片（含上下文行，不含空行）；Count：行数。
type searchResponse struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// Search 与 Query 共享 key/regex/level/time 校验与 Source 读取路径，
// 但以 JSON 行数组形式直接返回（不压缩为 zip），供 logs 服务端 /logs/search 在线分页流程使用。
//
// 安全措施：复用 sanitizeKey 校验 key，Path 仍由 Source 实现控制访问范围。
func Search(c *gin.Context) {
	s, ok := resolveSource(c)
	if !ok || s == nil {
		c.JSON(503, gin.H{"error": "source not initialized"})
		return
	}

	var fileReq FileReq
	data, err := c.GetRawData()
	if err != nil {
		log.Printf("读取请求体失败: %v", err)
		c.Status(400)
		return
	}
	if err := json.Unmarshal(data, &fileReq); err != nil {
		log.Printf("JSON解析失败: %v", err)
		c.Status(400)
		return
	}

	safeKey, ok := sanitizeKey(fileReq.Key, fileReq.Regex)
	if !ok {
		log.Printf("非法key参数: %s (regex=%v)", fileReq.Key, fileReq.Regex)
		c.Status(400)
		return
	}

	if len(fileReq.Query) > 1024 {
		log.Printf("Query 参数过长: %d 字符", len(fileReq.Query))
		c.Status(400)
		return
	}

	var buf bytes.Buffer
	scanCfg := source.ScanConfig{
		Key:          safeKey,
		Regex:        fileReq.Regex,
		Level:        fileReq.Level,
		StartTime:    fileReq.StartTime,
		EndTime:      fileReq.EndTime,
		ContextLines: fileReq.Line,
		Query:        fileReq.Query,
	}
	if _, err := s.Read(c.Request.Context(), fileReq.Path, scanCfg, &buf); err != nil {
		log.Printf("Source.Read 失败: %v", err)
		c.Status(404)
		return
	}

	var lines []string
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
		// 早停：达到 MaxLines 上限后不再继续扫描，降低内存与网络开销。
		if fileReq.MaxLines > 0 && len(lines) >= fileReq.MaxLines {
			break
		}
	}

	c.JSON(http.StatusOK, searchResponse{Lines: lines, Count: len(lines)})
}

// tailCleanupOutput Tail 任务结束后做的清理工作。
// Source 实现保证 Tail 退出后不再推送，无需关闭连接。
func tailCleanupOutput() {
	// 当前无独立资源需要清理（无额外文件句柄），保留接口以备扩展
}

// ReadString / ReadConfig 已迁移到通用 source.ScanSource，本包不再保留。
// 安全建议：如需内嵌文件操作逻辑，请调用 source.NewFileSource()。

// 以下保持向后兼容的导出包装：
// - 老单测可继续用 sanitizeKey / normalizeLevel

// normalizeLevel 统一日志级别大小写与别名。
func normalizeLevel(level string) string {
	switch strings.ToUpper(level) {
	case "WARN", "WARNING":
		return "WARN"
	case "DEBUG", "INFO", "ERROR", "FATAL":
		return strings.ToUpper(level)
	default:
		return strings.ToUpper(level)
	}
}

// parseTimeToSeconds 将 HH:MM:SS 或 HH:MM 格式转为当日秒数，解析失败返回 -1。
// 现由 source 包提供，这里保留以便外部调用方无缝迁移。
//
// Deprecated: 请使用 source 包中的过滤器。
func parseTimeToSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return -1
	}
	var h, m, sec int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil || h > 23 {
		return -1
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil || m > 59 {
		return -1
	}
	total := h*3600 + m*60
	if len(parts) == 3 {
		if _, err := fmt.Sscanf(parts[2], "%d", &sec); err != nil || sec > 59 {
			return -1
		}
		total += sec
	}
	return total
}

// extractTimeFromLogLine 从日志行首提取时间戳并返回当日秒数。
//
// Deprecated: 请使用 source 包中的过滤器。
func extractTimeFromLogLine(line string) int {
	line = strings.TrimSpace(line)
	if len(line) < 8 {
		return -1
	}
	loc := time.Local
	formats := []string{
		"2006-01-02 15:04:05", "2006-01-02 15:04",
		"2006/01/02 15:04:05", "2006/01/02 15:04",
		"15:04:05", "15:04",
	}
	for _, f := range formats {
		if len(line) >= len(f) {
			if t, err := time.ParseInLocation(f, line[:len(f)], loc); err == nil {
				return t.Hour()*3600 + t.Minute()*60 + t.Second()
			}
		}
	}
	return -1
}

// matchLogLine 判断日志行是否匹配过滤条件。由 source 包 TailFilter.Match 实现。
//
// Deprecated: 请使用 source 包。
func matchLogLine(line, key string, regex bool, re *regexp.Regexp, level string) bool {
	if regex && re != nil {
		if !re.MatchString(line) {
			return false
		}
	} else if key != "" {
		if !strings.Contains(line, key) {
			return false
		}
	}
	if level != "" {
		logLevelPattern := regexp.MustCompile(`(?i)\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL)\b`)
		levelFound := logLevelPattern.FindString(line)
		if !strings.EqualFold(levelFound, normalizeLevel(level)) {
			return false
		}
	}
	return true
}

// GetLocalIPv4 返回本机第一个非 loopback 的 IPv4 地址。
func getLocalIPv4() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		log.Println("net.Interfaces failed, err:", err.Error())
		return "unknown"
	}
	for i := 0; i < len(netInterfaces); i++ {
		if (netInterfaces[i].Flags & net.FlagUp) != 0 {
			addrs, _ := netInterfaces[i].Addrs()
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String()
					}
				}
			}
		}
	}
	return "unknown"
}

// bufferCompatOutput 保证 io.Writer 的最小缓冲需求。
// 对 source 的输出端，bytes.Buffer 已满足。
func bufferCompatOutput(w io.Writer) *bufio.Writer {
	if bw, ok := w.(*bufio.Writer); ok {
		return bw
	}
	return bufio.NewWriter(w)
}
