// Package config 提供轻量级 INI 配置文件读取，替代 beego.AppConfig。
// 配置文件格式与 beego 兼容：`key = value`，支持 `#`/`;` 注释。
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 保存解析后的键值配置
type Config struct {
	values map[string]string
	file   string
}

// NewDefault 返回使用内置默认值的空配置
func NewDefault() *Config {
	return &Config{
		values: make(map[string]string),
		file:   "",
	}
}

// Load 读取指定路径的 INI 配置文件
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

// File 返回配置文件路径
func (c *Config) File() string {
	return c.file
}

// String 返回字符串配置，环境变量优先（可选），其次配置文件
func (c *Config) String(key string) string {
	return c.values[key]
}

// StringOr 返回字符串配置，key 不存在时返回默认值
func (c *Config) StringOr(key, def string) string {
	if v, ok := c.values[key]; ok && v != "" {
		return v
	}
	return def
}

// Int 返回整数配置，解析失败或不存在时返回默认值
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

// Bool 返回布尔配置，解析失败或不存在时返回默认值
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

// String 全局便捷方法
func String(path, key string) (string, error) {
	c, err := Load(path)
	if err != nil {
		return "", err
	}
	return c.String(key), nil
}

// EnvOrConfig 环境变量优先，其次配置文件
func EnvOrConfig(envKey, configValue, def string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if configValue != "" {
		return configValue
	}
	return def
}

// DSN 构建 MySQL 连接串
func (c *Config) DSN(host, port, user, pwd, database string, parseTime bool) string {
	parse := ""
	if parseTime {
		parse = "&parseTime=true&loc=Local"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8%s", user, pwd, host, port, database, parse)
}
