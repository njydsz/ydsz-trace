// Package file 提供日志文件搜索与下载能力。
//
// 关键安全措施：
//   - key 参数白名单校验（仅字母/数字/连字符/下划线，正则模式下放宽至可打印字符）
//   - path 参数禁止包含 ".."
//   - 所有文件拼接使用 filepath.Join
//   - 正则模式下限制正则长度 256，防止 ReDoS
package file

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// FileReq 文件查询请求体。
type FileReq struct {
	// Path 日志文件的绝对路径
	Path string `json:"path"`
	// Key 搜索关键字（同时用于生成临时文件名）
	Key string `json:"key"`
	// Line 命中行后追加读取的上下文行数
	Line int64 `json:"line"`
	// Regex 是否启用正则匹配模式（默认 false，使用简单字符串包含）
	Regex bool `json:"regex"`
	// Level 日志级别过滤（空=不过滤，可选: DEBUG/INFO/WARN/ERROR/FATAL）
	Level string `json:"level"`
	// StartTime 时间范围起始（格式 HH:MM:SS 或 HH:MM，空=不限制）
	StartTime string `json:"startTime"`
	// EndTime 时间范围结束（格式 HH:MM:SS 或 HH:MM，空=不限制）
	EndTime string `json:"endTime"`
}

// safeKeyPattern 合法 key 白名单：字母、数字、连字符、下划线，长度 1-128。
var safeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,128}$`)

// safeRegexPattern 正则模式下允许的字符：可打印 ASCII（不含控制字符），长度 1-256。
var safeRegexPattern = regexp.MustCompile(`^[[:print:]]{1,256}$`)

// logLevelPattern 常见日志级别匹配模式（行内搜索用）。
var logLevelPattern = regexp.MustCompile(`(?i)\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL)\b`)

// sanitizeKey 校验 key 参数，防止路径遍历与恶意输入。
//
// regex 模式下放宽校验（允许正则元字符），但限制长度防 ReDoS。
// 返回清洗后的 key 与是否合法。
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

// Query 处理日志文件查询请求：搜索 → 过滤上下文 → zip 压缩 → 下载。
//
// 响应：application/octet-stream（zip 文件）。
// 安全措施：调用 sanitizeKey 与 path ".." 校验，参数非法直接返回 400。
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	var fileReq FileReq
	data, err := c.GetRawData()
	if err != nil {
		log.Printf("读取请求体失败: %v", err)
		c.Status(400)
		return
	}
	err = json.Unmarshal(data, &fileReq)
	if err != nil {
		log.Printf("JSON解析失败: %v", err)
		c.Status(400)
		return
	}

	// 安全校验：清理 key 参数，防止路径遍历
	safeKey, ok := sanitizeKey(fileReq.Key, fileReq.Regex)
	if !ok {
		log.Printf("非法key参数: %s (regex=%v)", fileReq.Key, fileReq.Regex)
		c.Status(400)
		return
	}

	// 安全校验：path 不允许包含路径遍历字符
	if strings.Contains(fileReq.Path, "..") {
		log.Printf("非法path参数(包含..): %s", fileReq.Path)
		c.Status(400)
		return
	}

	result := ReadString(ReadConfig{
		Filename:  fileReq.Path,
		Key:       safeKey,
		Line:      fileReq.Line,
		TempPath:  cfg.StringOr("temppath", "./temp/logc/"),
		Regex:     fileReq.Regex,
		Level:     fileReq.Level,
		StartTime: fileReq.StartTime,
		EndTime:   fileReq.EndTime,
	})
	defer func() {
		if result != "" {
			os.Remove(result)
		}
	}()

	if result == "" {
		c.Status(404)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filepath.Base(result)+`"`)
	c.File(result)
}

// ReadConfig 日志搜索配置参数。
type ReadConfig struct {
	Filename  string
	Key       string
	Line      int64
	TempPath  string
	Regex     bool
	Level     string
	StartTime string
	EndTime   string
}

// ReadString 按关键字搜索日志文件并输出到临时文件，再压缩为 zip 返回路径。
//
// 行为：
//   - 每次命中关键字所在行及其后 line 行写入结果
//   - 若后续命中在当前上下文窗口内，则扩展窗口
//   - 支持 regex 模式：使用正则匹配替代简单字符串包含
//   - 支持 level 过滤：仅保留包含指定级别的行
//   - 支持时间范围：仅保留时间戳在范围内的行
//
// 返回 zip 文件路径；失败或无匹配返回空字符串。
func ReadString(cfg ReadConfig) (file string) {
	startTime := time.Now()
	defer func() {
		log.Printf("共耗时：%s\n", time.Since(startTime))
	}()

	f, err := os.Open(cfg.Filename)
	if err != nil {
		log.Printf("打开文件失败: %v", err)
		return ""
	}
	defer f.Close()

	// 预编译正则表达式（regex 模式下）
	var re *regexp.Regexp
	if cfg.Regex {
		var err error
		re, err = regexp.Compile(cfg.Key)
		if err != nil {
			log.Printf("正则编译失败: %v", err)
			return ""
		}
	}

	// 解析时间范围
	hasTimeRange := cfg.StartTime != "" || cfg.EndTime != ""
	var startSeconds, endSeconds int
	if hasTimeRange {
		startSeconds = parseTimeToSeconds(cfg.StartTime)
		endSeconds = parseTimeToSeconds(cfg.EndTime)
	}

	r := bufio.NewReader(f)
	var lineBegin int64
	var lineFirst int64
	var lineOver int64

	// 确保临时目录存在
	if err := os.MkdirAll(cfg.TempPath, 0755); err != nil {
		log.Printf("创建临时目录失败: %v", err)
		return ""
	}

	// 安全：使用 filepath.Join 拼接路径
	safeFilename := filepath.Join(cfg.TempPath, cfg.Key+".log")
	dstFile, err := os.OpenFile(safeFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("创建临时文件失败: %v", err)
		return ""
	}

	bufWriter := bufio.NewWriter(dstFile)

	for {
		str, err := r.ReadString('\n')
		lineBegin++
		if err != nil && str == "" {
			break
		}

		// 关键字匹配
		matched := false
		if cfg.Regex && re != nil {
			matched = re.MatchString(str)
		} else if cfg.Key != "" {
			matched = strings.Contains(str, cfg.Key)
		}

		if !matched {
			// 上下文窗口内仍写入
			if lineBegin < lineOver {
				bufWriter.WriteString(str)
			}
			if lineBegin == lineOver {
				bufWriter.WriteString(str)
				bufWriter.WriteString("\n")
			}
			if err != nil {
				break
			}
			continue
		}

		// 日志级别过滤
		if cfg.Level != "" {
			levelFound := logLevelPattern.FindString(str)
			if !strings.EqualFold(levelFound, normalizeLevel(cfg.Level)) {
				// 级别不匹配，但仍检查上下文窗口
				if lineBegin < lineOver {
					bufWriter.WriteString(str)
				}
				if lineBegin == lineOver {
					bufWriter.WriteString(str)
					bufWriter.WriteString("\n")
				}
				if err != nil {
					break
				}
				continue
			}
		}

		// 时间范围过滤
		if hasTimeRange {
			lineSeconds := extractTimeFromLogLine(str)
			if lineSeconds >= 0 {
				if cfg.StartTime != "" && lineSeconds < startSeconds {
					if lineBegin < lineOver {
						bufWriter.WriteString(str)
					}
					if lineBegin == lineOver {
						bufWriter.WriteString(str)
						bufWriter.WriteString("\n")
					}
					if err != nil {
						break
					}
					continue
				}
				if cfg.EndTime != "" && lineSeconds > endSeconds {
					if lineBegin < lineOver {
						bufWriter.WriteString(str)
					}
					if lineBegin == lineOver {
						bufWriter.WriteString(str)
						bufWriter.WriteString("\n")
					}
					if err != nil {
						break
					}
					continue
				}
			}
		}

		// 命中：更新上下文窗口
		if lineFirst == 0 && lineOver == 0 {
			lineFirst = lineBegin
			lineOver = lineFirst + cfg.Line
		} else if lineBegin > lineOver {
			// 扩展窗口
			lineFirst = lineBegin
			lineOver = lineFirst + cfg.Line
		}

		bufWriter.WriteString(str)
		if lineBegin == lineOver {
			bufWriter.WriteString("\n")
		}

		if err != nil {
			break
		}
	}

	bufWriter.Flush()
	dstFile.Close()

	log.Printf("查找耗时：%s\n", time.Since(startTime))

	// 压缩（复用 pkg/util.Zip 统一实现）
	ip := GetLocalIPv4()
	dst := filepath.Join(cfg.TempPath, ip+".zip")
	if err := util.Zip(dst, safeFilename); err != nil {
		log.Printf("压缩失败: %v", err)
		return ""
	}
	return dst
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

// parseTimeToSeconds 将 HH:MM:SS 或 HH:MM 格式转为当日秒数，解析失败返回 -1。
func parseTimeToSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return -1
	}
	h, err1 := parsePart(parts[0])
	m, err2 := parsePart(parts[1])
	if err1 != nil || err2 != nil || h > 23 || m > 59 {
		return -1
	}
	total := h*3600 + m*60
	if len(parts) == 3 {
		sec, err := parsePart(parts[2])
		if err != nil || sec > 59 {
			return -1
		}
		total += sec
	}
	return total
}

// parsePart 解析单个时间字段。
func parsePart(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// extractTimeFromLogLine 尝试从日志行首提取时间戳并返回当日秒数。
//
// 支持的格式：
//   - "2006-01-02 15:04:05" / "2006-01-02 15:04"
//   - "15:04:05" / "15:04"
//   - "2006/01/02 15:04:05"
// 无法提取时返回 -1。
func extractTimeFromLogLine(line string) int {
	line = strings.TrimSpace(line)
	if len(line) < 8 {
		return -1
	}

	// 尝试常见格式
	loc := time.Local
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
	}

	for _, f := range formats {
		if len(line) >= len(f) {
			t, err := time.ParseInLocation(f, line[:len(f)], loc)
			if err == nil {
				return t.Hour()*3600 + t.Minute()*60 + t.Second()
			}
		}
	}

	// 仅时间 "15:04:05" / "15:04"
	shortFormats := []string{
		"15:04:05",
		"15:04",
	}
	for _, f := range shortFormats {
		if len(line) >= len(f) {
			t, err := time.ParseInLocation(f, line[:len(f)], loc)
			if err == nil {
				return t.Hour()*3600 + t.Minute()*60 + t.Second()
			}
		}
	}

	return -1
}

// TailReq 实时日志跟踪请求体。
type TailReq struct {
	// Path 日志文件的绝对路径
	Path string `json:"path"`
	// Key 搜索关键字
	Key string `json:"key"`
	// Regex 是否使用正则匹配
	Regex bool `json:"regex"`
	// Level 日志级别过滤
	Level string `json:"level"`
	// FollowDuration 跟踪持续时间（秒），0 表示由客户端断开后结束
	FollowDuration int64 `json:"followDuration"`
}

// Tail 实时日志跟踪端点：读取文件末尾，并在文件增长时持续推送新匹配行。
//
// 行为：
//   - 跳转到文件末尾（仅跟踪新增内容）
//   - 通过轮询监听文件变化（每 500ms）
//   - 匹配 Key/Regex/Level 过滤后通过 SSE 推送给客户端
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

	// 安全校验
	if strings.Contains(tailReq.Path, "..") {
		log.Printf("非法path参数(包含..): %s", tailReq.Path)
		c.Status(400)
		return
	}

	// 验证 key
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
	c.Header("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	// 打开文件
	f, err := os.Open(tailReq.Path)
	if err != nil {
		log.Printf("打开文件失败: %v", err)
		c.SSEvent("error", fmt.Sprintf("打开文件失败: %v", err))
		c.Status(404)
		return
	}
	defer f.Close()

	// 预编译正则
	var re *regexp.Regexp
	if tailReq.Regex {
		re, err = regexp.Compile(safeKey)
		if err != nil {
			c.SSEvent("error", "正则编译失败")
			c.Status(400)
			return
		}
	}

	// 跳转到文件末尾（仅跟踪新增内容）
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		log.Printf("Seek文件末尾失败: %v", err)
	}

	// 通知客户端已连接
	c.SSEvent("connected", "tracking started")
	c.Writer.Flush()

	// 轮询跟踪
	bufReader := bufio.NewReader(f)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var deadline time.Time
	if tailReq.FollowDuration > 0 {
		deadline = time.Now().Add(time.Duration(tailReq.FollowDuration) * time.Second)
	}

	for {
		select {
		case <-c.Request.Context().Done():
			// 客户端断开
			return
		case <-ticker.C:
			// 检查超时
			if !deadline.IsZero() && time.Now().After(deadline) {
				c.SSEvent("done", "tracking timeout")
				c.Writer.Flush()
				return
			}

			// 读取新增内容
			for {
				line, err := bufReader.ReadString('\n')
				if err != nil {
					if err != io.EOF {
						log.Printf("读取日志行失败: %v", err)
					}
					break
				}

				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					continue
				}

				// 过滤匹配
				if !matchLogLine(line, safeKey, tailReq.Regex, re, tailReq.Level) {
					continue
				}

				// 推送匹配行
				c.SSEvent("line", line)
				c.Writer.Flush()
			}
		}
	}
}

// matchLogLine 判断日志行是否匹配过滤条件。
func matchLogLine(line, key string, regex bool, re *regexp.Regexp, level string) bool {
	// 关键字匹配
	if regex && re != nil {
		if !re.MatchString(line) {
			return false
		}
	} else if key != "" {
		if !strings.Contains(line, key) {
			return false
		}
	}

	// 级别过滤
	if level != "" {
		levelFound := logLevelPattern.FindString(line)
		if !strings.EqualFold(levelFound, normalizeLevel(level)) {
			return false
		}
	}

	return true
}

// GetLocalIPv4 返回本机第一个非 loopback 的 IPv4 地址。
//
// 未找到时返回 "unknown"。
func GetLocalIPv4() (ip string) {
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

// Zip / UnZip 已统一迁移至 pkg/util 包，本文件不再保留重复实现。
