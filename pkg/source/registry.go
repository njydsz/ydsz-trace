// Package source 子模块：source 类型注册表。
//
// 提供插件化注册机制：新的 Source 包在 init() 中调用 RegisterSource 注册
// 自身构造函数，即可被 CreateSource 按需实例化，无需改动 factory.go 的 switch。
//
// 现有内置源：
//   - SourceTypeFile   → file_source.go   (NewFileSource)
//   - SourceTypeDocker → docker_source.go (NewDockerSource)
//   - SourceTypeK8s    → k8s_source.go    (NewK8sSource)
package source

import (
	"fmt"
	"sync"
)

// SourceFactory 根据 FactoryConfig 构造特定类型的 Source。
// 各 Source 实现在 init() 中注册自己的工厂函数。
type SourceFactory func(cfg FactoryConfig) (Source, error)

// sourceRegistry 全局源类型 → 工厂的映射，并发安全。
var (
	sourceRegistry   = map[SourceType]SourceFactory{}
	sourceRegistryMu sync.RWMutex
)

// RegisterSource 注册 Source 类型对应的工厂函数。
//
// 相同类型重复注册会覆盖（后者优先），便于测试替换。
// 通常在各 Source 实现文件的 init() 中调用。
func RegisterSource(t SourceType, factory SourceFactory) {
	sourceRegistryMu.Lock()
	defer sourceRegistryMu.Unlock()
	sourceRegistry[t] = factory
}

// ResolveSource 根据 FactoryConfig 查找已注册的工厂并构造 Source。
// 未注册的类型返回错误。
func ResolveSource(cfg FactoryConfig) (Source, error) {
	sourceRegistryMu.RLock()
	factory, ok := sourceRegistry[cfg.Type]
	sourceRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("未注册的 source 类型: %s", cfg.Type)
	}
	return factory(cfg)
}

// RegisteredTypes 返回当前已注册的所有 source 类型（便于诊断/文档）。
func RegisteredTypes() []SourceType {
	sourceRegistryMu.RLock()
	defer sourceRegistryMu.RUnlock()
	out := make([]SourceType, 0, len(sourceRegistry))
	for t := range sourceRegistry {
		out = append(out, t)
	}
	return out
}
