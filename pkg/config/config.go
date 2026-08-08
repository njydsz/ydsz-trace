// Package config 提供轻量级 INI 配置文件读取，替代 beego.AppConfig。
//
// 配置文件格式与 beego 兼容：
//
//	key = value              # 行尾注释
//	key2 = value2            ; 分号注释也支持
//	[section]                # 节标题会被忽略（仅作兼容）
//
// 特性：
//   - 支持 # 和 ; 两种行尾注释
//   - section 头会被跳过（beego 兼容）
//   - 环境变量可通过 EnvOrConfig 优先覆盖
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Config 保存解析后的键值配置。
//
// 并发安全：读取阶段完成后 values 不再修改，可多 goroutine 读。
type Config struct {
	values map[string]string
	file   string
}

// NewDefault 返回空配置（使用内置默认值）。
//
// 适用于配置文件不存在或加载失败时的降级。
func NewDefault() *Config {
	return &Config{
		values: make(map[string]string),
		file:   "",
	}
}

// Load 读取并解析 INI 配置文件。
//
// 解析规则：
//   - 跳过空行、# 或 ; 开头的注释行、[section] 头
//   - 按首个 = 分割键值
//   - 行尾注释需以 # 或 ; 前空白开头才识别为注释
//
// 返回 *Config 与可能的文件打开错误。
func Load(path string) (*Config, error) {	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := &Config{
		values: make(map[string]string),
		file:   path,
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		// 忽略 section 头（beego 配置一般不用，兼容处理）
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// 去掉行尾注释（# 或 ; 前面有空白）
		if i := strings.IndexAny(value, "#;"); i > 0 {
			// 仅当注释符前是空白时才是注释
			if value[i-1] == ' ' || value[i-1] == '\t' {
				value = strings.TrimSpace(value[:i])
			}
		}
		c.values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// File 返回配置文件路径，用于日志打印与调试。
func (c *Config) File() string {
	return c.file
}

// String 返回字符串配置值，不存在时返回空字符串。
//
// 若需要环境变量优先覆盖请使用 EnvOrConfig 工具函数。
func (c *Config) String(key string) string {
	return c.values[key]
}

// StringOr 返回字符串配置，key 不存在或为空时返回 def。
func (c *Config) StringOr(key, def string) string {
	if v, ok := c.values[key]; ok && v != "" {
		return v
	}
	return def
}

// Int 返回整数配置，解析失败或不存在时返回 def。
func (c *Config) Int(key string, def int) int {
	v, ok := c.values[key]
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// Bool 返回布尔配置（true/false/1/0/yes/no），失败或不存在返回 def。
func (c *Config) Bool(key string, def bool) bool {
	v, ok := c.values[key]
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

// String 是全局便捷方法：加载指定配置文件并读取字符串值。
//
// 适用于只需读取单个配置项的场景，频繁调用建议缓存 *Config。
func String(path, key string) (string, error) {
	c, err := Load(path)
	if err != nil {
		return "", err
	}
	return c.String(key), nil
}

// EnvOrConfig 按优先级取值：环境变量 > 配置文件 > 默认值。
//
// 便于在容器化部署时通过环境变量动态覆盖静态配置。
func EnvOrConfig(envKey, configValue, def string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if configValue != "" {
		return configValue
	}
	return def
}

