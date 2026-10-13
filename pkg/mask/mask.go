// Package mask 数据脱敏规则引擎（v2.0）。
//
// 内置常见脱敏策略：手机号、身份证、银行卡、邮箱、IP、姓名、自定义正则。
// 所有函数均为纯函数，线程安全，不依赖外部状态。
package mask

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Masker 脱敏策略枚举。
type Masker int

const (
	// MaskPhone 手机号 前 3 后 4。
	MaskPhone Masker = iota + 1
	// MaskIDCard 身份证 前 4 后 4。
	MaskIDCard
	// MaskBankCard 银行卡 前 4 后 4。
	MaskBankCard
	// MaskEmail 邮箱 ***@域名。
	MaskEmail
	// MaskIP IP 192.***.***.1。
	MaskIP
	// MaskName 姓名 第 2 字起 *。
	MaskName
	// MaskRegex 自定义正则（需配合 MaskWithRule 使用）。
	MaskRegex
)

// 预编译正则表达式。
var (
	rePhone    = regexp.MustCompile(`(\d{3})\d{4}(\d{4})`)
	reIDCard   = regexp.MustCompile(`(\d{4})\d{10}(\w{4})`)
	reBankCard = regexp.MustCompile(`(\d{4})\d+(\d{4})`)
	reEmail    = regexp.MustCompile(`(\w{1})\w*(@\S+)`)
	reIP       = regexp.MustCompile(`(\d{1,3}\.\d{1,3})\.(\d{1,3})\.(\d{1,3})`)
)

// Mask 对输入文本执行脱敏（保留格式，替换敏感位置为 *）。
//
// 当 masker 为 MaskRegex 时，返回原字符串（需使用 MaskWithRule）。
func Mask(input string, masker Masker) string {
	switch masker {
	case MaskPhone:
		return rePhone.ReplaceAllString(input, "$1****$2")
	case MaskIDCard:
		return reIDCard.ReplaceAllString(input, "$1**********$2")
	case MaskBankCard:
		return reBankCard.ReplaceAllString(input, "$1 **** **** $2")
	case MaskEmail:
		return reEmail.ReplaceAllString(input, "$1***$2")
	case MaskIP:
		return reIP.ReplaceAllString(input, "$1.***.***.$3")
	case MaskName:
		return maskName(input)
	default:
		return input
	}
}

// MaskAll 智能检测：对输入应用所有预置策略（去重）。
//
// 按手机、身份证、银行卡、邮箱、IP、姓名顺序依次脱敏。
func MaskAll(input string) string {
	input = Mask(input, MaskPhone)
	input = Mask(input, MaskIDCard)
	input = Mask(input, MaskBankCard)
	input = Mask(input, MaskEmail)
	input = Mask(input, MaskIP)
	input = Mask(input, MaskName)
	return input
}

// MaskWithRule 自定义正则脱敏：regex + replacement（如 `$1****$2`）。
//
// 返回脱敏后字符串或正则编译错误。
func MaskWithRule(input, planRegex, replacement string) (string, error) {
	re, err := regexp.Compile(planRegex)
	if err != nil {
		return "", fmt.Errorf("mask: 正则编译失败: %w", err)
	}
	return re.ReplaceAllString(input, replacement), nil
}

// MaskStruct 对 struct 反射脱敏（根据字段的 `mask:"phone"` 等 tag 指令）。
//
// 支持 tag 值：phone、idcard、bankcard、email、ip、name。
// 仅处理可设置的字符串字段，非字符串字段跳过。
// 返回 map[string]interface{}（字段名 -> 脱敏后值或零值）。
func MaskStruct(v interface{}) interface{} {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return v
	}

	typ := val.Type()
	result := make(map[string]interface{}, typ.NumField())

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fv := val.Field(i)

		if !fv.CanInterface() {
			continue
		}

		maskTag := field.Tag.Get("mask")
		fieldName := jsonTagOrName(field)

		if maskTag == "" || fv.Kind() != reflect.String {
			result[fieldName] = fv.Interface()
			continue
		}

		result[fieldName] = MaskByTag(fv.String(), maskTag)
	}

	return result
}

// MaskByTag 根据 mask tag 值选择脱敏策略。
func MaskByTag(input, tag string) string {
	switch tag {
	case "phone":
		return Mask(input, MaskPhone)
	case "idcard":
		return Mask(input, MaskIDCard)
	case "bankcard":
		return Mask(input, MaskBankCard)
	case "email":
		return Mask(input, MaskEmail)
	case "ip":
		return Mask(input, MaskIP)
	case "name":
		return Mask(input, MaskName)
	default:
		return input
	}
}

// maskName 对姓名脱敏：第 2 字起替换为 *。
// 仅对短文本（<=4 个中文字符，或不含空格的 2-4 字符）生效，
// 避免在多字段拼接字符串上误伤。
func maskName(input string) string {
	// 多词（含空格）不视为姓名，跳过
	if strings.Contains(input, " ") || strings.Contains(input, "\t") {
		return input
	}
	runes := []rune(input)
	if len(runes) <= 1 || len(runes) > 4 {
		return input
	}
	for i := 1; i < len(runes); i++ {
		runes[i] = '*'
	}
	return string(runes)
}

// jsonTagOrName 获取字段 JSON tag 名称，若无则用字段名。
func jsonTagOrName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	if idx := strings.Index(tag, ","); idx != -1 {
		tag = tag[:idx]
	}
	if tag == "-" {
		return field.Name
	}
	return tag
}
