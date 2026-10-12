// Package sanitize 提供输入白名单过滤函数，防止路径穿越与注入攻击。
//
// 所有外部可控输入（文件路径片段、搜索关键词拼接路径前）必须经过本包校验。
// 校验规则基于白名单正则：仅允许字母、数字、点、连字符、下划线。
package sanitize

import (
	"fmt"
	"regexp"
)

// safeFilenamePattern 安全文件名白名单：字母、数字、点、连字符、下划线，长度 1-64。
var safeFilenamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

// safeDatePattern 日期格式白名单：YYYY-MM-DD 或 YYYYMMDD，长度 8-10。
var safeDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$|^\d{8}$`)

// SafeFilename 校验文件名/路径片段是否安全。
// 空字符串返回 false；匹配白名单返回清洗后字符串与 true。
func SafeFilename(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if !safeFilenamePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// SafeDate 校验日期格式是否安全。
// 支持 YYYY-MM-DD（10 字符）与 YYYYMMDD（8 字符），防止路径穿越。
func SafeDate(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if !safeDatePattern.MatchString(s) {
		return "", false
	}
	return s, true
}

// MustSafeFilename 校验失败时返回带上下文的 error。
func MustSafeFilename(s string) (string, error) {
	if cleaned, pass := SafeFilename(s); !pass {
		return "", fmt.Errorf("sanitize: invalid filename %q", s)
	} else {
		return cleaned, nil
	}
}
