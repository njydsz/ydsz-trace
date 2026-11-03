package source

import (
	"os"
)

// SourceType 支持的源类型标识。
type SourceType string

const (
	SourceTypeFile     SourceType = "file"
	SourceTypeDocker   SourceType = "docker"
	SourceTypeK8s      SourceType = "k8s"
	SourceTypeJournald SourceType = "journald"
	SourceTypeSyslog   SourceType = "syslog"
	SourceTypeHTTPPush SourceType = "http_push"
)

// FactoryConfig 工厂的通用配置映射。
type FactoryConfig struct {
	// Type 源类型。
	Type SourceType
	// LogLevel 日志级别（可选）
	LogLevel string
	// 其他类型特定的 KV 选项
	Options map[string]string
}

// CreateSource 根据工厂配置创建对应的 Source。
//
// 通过 options 传递各类型的配置：
//   - FileSource: options["root_dir"] = "/var/log/app"
//   - DockerSource: options["socket"] = "/var/run/docker.sock",
//     options["container_label"] = "ydsz-trace/collect=true",
//     options["app_name_key"] = "ydsz-trace/app-name"
//   - K8sSource: options["node_name"] = "worker-01",
//     options["namespace"] = "",
//     options["discovery_anno"] = "ydsz-trace/collect"
//   - JournaldSource: options["unit"] = "app.service", options["priority"] = "6"
//   - SyslogSource: options["listen"] = ":5140", options["protocol"] = "udp"
//   - HTTPPushSource: options["listen"] = ":5141", options["path"] = "/logs"
//
// 实现已改为注册表模式：各 Source 类型在 init() 中通过 RegisterSource 注册，
// 新类型只需注册自身，无需修改本函数。
func CreateSource(cfg FactoryConfig) (Source, error) {
	return ResolveSource(cfg)
}

// CreateSourceFromEnv 从环境变量构造 Source。
//
// 读取 YDSZ_LOG_SOURCE 决定类型，其余映射到各实现的选项。
func CreateSourceFromEnv() (Source, error) {
	srcType := SourceType(os.Getenv("YDSZ_LOG_SOURCE"))
	if srcType == "" {
		srcType = SourceTypeFile // 默认传统文件模式（兼容现有部署）
	}

	options := map[string]string{}
	switch srcType {
	case SourceTypeFile:
		if v := os.Getenv("YDSZ_LOG_ROOT_DIR"); v != "" {
			options["root_dir"] = v
		}
	case SourceTypeDocker:
		if v := os.Getenv("YDSZ_DOCKER_SOCKET"); v != "" {
			options["socket"] = v
		}
		if v := os.Getenv("YDSZ_DISCOVERY_LABEL"); v != "" {
			options["container_label"] = v
		}
		if v := os.Getenv("YDSZ_APP_NAME_KEY"); v != "" {
			options["app_name_key"] = v
		}
	case SourceTypeK8s:
		if v := os.Getenv("YDSZ_NODE_NAME"); v != "" {
			options["node_name"] = v
		}
		if v := os.Getenv("YDSZ_POD_NAMESPACE"); v != "" {
			options["namespace"] = v
		}
		if v := os.Getenv("YDSZ_DISCOVERY_ANNOTATION"); v != "" {
			options["discovery_anno"] = v
		}
	case SourceTypeJournald:
		if v := os.Getenv("YDSZ_JOURNALD_UNIT"); v != "" {
			options["unit"] = v
		}
		if v := os.Getenv("YDSZ_JOURNALD_PRIORITY"); v != "" {
			options["priority"] = v
		}
		if v := os.Getenv("YDSZ_JOURNALD_SINCE"); v != "" {
			options["since"] = v
		}
		if v := os.Getenv("YDSZ_JOURNALD_BIN"); v != "" {
			options["journalctl"] = v
		}
	case SourceTypeSyslog:
		if v := os.Getenv("YDSZ_SYSLOG_LISTEN"); v != "" {
			options["listen"] = v
		}
		if v := os.Getenv("YDSZ_SYSLOG_PROTOCOL"); v != "" {
			options["protocol"] = v
		}
	case SourceTypeHTTPPush:
		if v := os.Getenv("YDSZ_HTTP_PUSH_LISTEN"); v != "" {
			options["listen"] = v
		}
		if v := os.Getenv("YDSZ_HTTP_PUSH_PATH"); v != "" {
			options["path"] = v
		}
	}

	return CreateSource(FactoryConfig{
		Type:    srcType,
		Options: options,
	})
}

// getOpt 从 map 取 key，缺失返回 fallback。
func getOpt(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok && v != "" {
		return v
	}
	return fallback
}
