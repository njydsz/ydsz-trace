// Package logs 子文件：日志上下文回溯接口（FR-403）。
//
// 给定 refTime 或 refLine，在日志文件中定位到命中行，
// 向前/向后读取指定行数的上下文，便于人工排障时查看前后日志。
package logs

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/sanitize"

	"github.com/gin-gonic/gin"
)

const (
	// contextDefaultLines 默认前后行数
	contextDefaultLines = 10
	// contextMaxLines 单次请求最大行数限制
	contextMaxLines = 100
)

// Context 上下文回溯接口。
// GET /logs/context/:itemId?clientId=&before=&after=&refTime=&refLine=&date=
func Context(c *gin.Context) {
	itemID, err := strconv.ParseInt(c.Param("itemId"), 10, 64)
	if err != nil || itemID == 0 {
		api.Fail(c, api.CodeBadRequest, "itemId 非法")
		return
	}

	var req models.ContextReq
	if err := c.ShouldBindQuery(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数绑定失败")
		return
	}

	// 参数校验
	before := req.Before
	if before <= 0 {
		before = contextDefaultLines
	}
	if before > contextMaxLines {
		before = contextMaxLines
	}
	after := req.After
	if after <= 0 {
		after = contextDefaultLines
	}
	if after > contextMaxLines {
		after = contextMaxLines
	}

	if req.RefTime == "" && req.RefLine == "" {
		api.Fail(c, api.CodeBadRequest, "refTime 与 refLine 至少提供一个")
		return
	}

	// 查询日志项
	item := models.ReadItem(itemID)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}

	// 确定日期：优先使用显式 date 参数，否则尝试从 refTime 提取
	date := req.Date
	if date == "" && req.RefTime != "" {
		date = extractDateFromRefTime(req.RefTime)
	}
	if date == "" {
		date = time.Now().Format("20060102")
	}

	// 日期格式兼容：统一为 8 位 YYYYMMDD
	date = normalizeDate(date)
	if date == "" {
		api.Fail(c, api.CodeBadRequest, "日期格式非法")
		return
	}

	if _, ok := sanitize.SafeDate(date); !ok {
		api.Fail(c, api.CodeBadRequest, "日期格式非法，仅允许 YYYYMMDD 或 YYYY-MM-DD")
		return
	}

	// 构造日志路径
	logPath := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	// 读取上下文
	result, err := readContext(logPath, req.RefTime, req.RefLine, before, after)
	if err != nil {
		log.Printf("读取上下文失败 path=%s: %v", logPath, err)
		api.Fail(c, api.CodeServerError, "读取日志文件失败: "+err.Error())
		return
	}

	// 包装为 Paginated 格式：list 放入上下文行，total 为 before + after + 1
	allLines := append(append([]string{result.RefLine}, result.BeforeLines...), result.AfterLines...)
	api.Paginated(c, "查询成功", allLines,
		len(allLines), 1, len(allLines))
}

// extractDateFromRefTime 从 refTime 中提取日期部分（YYYYMMDD）。
func extractDateFromRefTime(refTime string) string {
	// 处理 "2006-01-02T15:04:05Z" / "2006-01-02 15:04:05" / "2006-01-02" 等格式
	t := strings.TrimSpace(refTime)
	if len(t) < 10 {
		return ""
	}
	// 取前 10 个字符: "2006-01-02"
	datePart := t[:10]
	datePart = strings.ReplaceAll(datePart, "-", "")
	if len(datePart) == 8 {
		return datePart
	}
	return ""
}

// normalizeDate 将日期统一为 8 位 YYYYMMDD 格式。
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	// 纯 8 位数字
	if len(s) == 8 {
		return s
	}
	// 10 位 YYYY-MM-DD
	if len(s) == 10 {
		return strings.ReplaceAll(s, "-", "")
	}
	return ""
}

// readContext 从日志文件中读取指定行前后的上下文。
//
// 实现：使用 bufio.Scanner 单次遍历，定位参考行（按时间戳或原文匹配），
// 收集命中前 before 行、命中后 after 行。若参考行不在文件中，
// 返回最近邻匹配结果。
func readContext(logPath, refTime, refLine string, before, after int) (*models.ContextResult, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	// 归一化 refTime，便于子串匹配
	normalizedRef := normalizeTimestamp(refTime)

	// 环形缓冲区保存最近 before 行（用于收集命中前上下文）
	ringBuf := make([]string, 0, before+1)
	var refFound bool
	var refLineText string
	var afterLines []string
	var beforeLines []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	matchLine := -1

	// 第一遍：找到参考行位置
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		// 将当前行加入环形缓冲区
		ringBuf = append(ringBuf, line)
		if len(ringBuf) > before+1 {
			ringBuf = ringBuf[1:]
		}

		// 尝试匹配
		if matchLine < 0 && matchLineContext(line, normalizedRef, refLine) {
			matchLine = lineNum
			refFound = true
			refLineText = line
			// 命中前的行为 ringBuf 中的前 before 行（去掉自身）
			if len(ringBuf) > 1 {
				beforeLines = make([]string, 0, len(ringBuf)-1)
				for i := 0; i < len(ringBuf)-1; i++ {
					beforeLines = append(beforeLines, ringBuf[i])
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描文件失败: %w", err)
	}

	if !refFound {
		// 未找到精确匹配，尝试全文搜索 refLine
		if refLine == "" {
			return nil, fmt.Errorf("未在日志中找到匹配行")
		}
		// 重试：使用原文匹配
		return retryWithExactLine(logPath, refLine, before, after)
	}

	// 已找到参考行并在 ringBuf 中捕获了命中前行
	// 需要再扫描一遍收集命中后行
	if after > 0 {
		afterLines = readAfterLines(logPath, matchLine, after)
	}

	return &models.ContextResult{
		RefTime:     refTime,
		RefLine:     refLineText,
		BeforeLines: beforeLines,
		AfterLines:  afterLines,
		TotalBefore: len(beforeLines),
		TotalAfter:  len(afterLines),
	}, nil
}

// matchLineContext 判断单行日志是否匹配参考行。
func matchLineContext(line, normalizedRef, refLine string) bool {
	// 优先时间戳匹配
	if normalizedRef != "" {
		lineNorm := normalizeTimestamp(line)
		if strings.Contains(lineNorm, normalizedRef) || strings.Contains(line, normalizedRef) {
			return true
		}
	}
	// 原文匹配
	if refLine != "" && strings.Contains(line, refLine) {
		return true
	}
	return false
}

// normalizeTimestamp 将时间戳归一化用于子串匹配（去分隔符、空格、T、Z、毫秒）。
func normalizeTimestamp(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "T", "")
	s = strings.ReplaceAll(s, "Z", "")
	// 去掉 .xxx 毫秒
	if idx := strings.Index(s, "."); idx > 0 {
		s = s[:idx]
	}
	return s
}

// readAfterLines 在文件中从 matchLine 之后读取 after 行。
func readAfterLines(logPath string, matchLine int, after int) []string {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	lines := make([]string, 0, after)
	scanner := bufio.NewScanner(f)
	current := 0
	for scanner.Scan() {
		current++
		if current <= matchLine {
			continue
		}
		lines = append(lines, scanner.Text())
		if len(lines) >= after {
			break
		}
	}
	return lines
}

// retryWithExactLine 使用行原文进行全文匹配（第二遍扫描，精确匹配）。
func retryWithExactLine(logPath, refLine string, before, after int) (*models.ContextResult, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	ringBuf := make([]string, 0, before+1)
	var matchLine int
	var refLineText string
	var beforeLines []string
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		ringBuf = append(ringBuf, line)
		if len(ringBuf) > before+1 {
			ringBuf = ringBuf[1:]
		}
		if matchLine == 0 && strings.Contains(line, refLine) {
			matchLine = lineNum
			refLineText = line
			if len(ringBuf) > 1 {
				beforeLines = make([]string, 0, len(ringBuf)-1)
				for i := 0; i < len(ringBuf)-1; i++ {
					beforeLines = append(beforeLines, ringBuf[i])
				}
			}
		}
	}

	if matchLine == 0 {
		return nil, fmt.Errorf("未在日志中找到匹配行")
	}

	var afterLines []string
	if after > 0 {
		afterLines = readAfterLines(logPath, matchLine, after)
	}

	return &models.ContextResult{
		RefTime:     "",
		RefLine:     refLineText,
		BeforeLines: beforeLines,
		AfterLines:  afterLines,
		TotalBefore: len(beforeLines),
		TotalAfter:  len(afterLines),
	}, nil
}

