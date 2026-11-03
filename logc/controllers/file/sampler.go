// Package file 子文件：端侧采样器接入（V2.2-11，FR-105）。
//
// 背景：
//   采样器在 logc 主流程创建（依赖环境变量），file 控制器需要在读取日志时
//   应用采样。Go 中 main 包变量无法被其他包引用，因此通过包级 holder 传递。
//
// 并发安全：
//   holder 在启动阶段写入一次，运行期只读；Sampler 自身并发安全。
package file

import "ydsz-trace/pkg/perf"

// samplerHolder 采样器持有者（由 main 在启动时注入）。
var samplerHolder struct {
	Sampler *perf.Sampler
}

// SetSampler 注入全局采样器（幂等，重复调用覆盖）。
func SetSampler(s *perf.Sampler) {
	samplerHolder.Sampler = s
}

// sampler 返回当前采样器（可能为 nil）。
func sampler() *perf.Sampler {
	return samplerHolder.Sampler
}
