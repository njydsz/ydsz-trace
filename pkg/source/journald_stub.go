//go:build windows

package source

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrJournaldUnsupported Windows 无 systemd journal。
var ErrJournaldUnsupported = errors.New("journald: Windows 不支持 systemd journal")

// JournaldSource Windows 平台占位实现。
//
// 保持接口一致性，调用时返回明确错误而非 panic，
// 便于上层给出「该平台不支持」的可读提示。
type JournaldSource struct {
	info SourceInfo
}

// NewJournaldSource 创建占位实例。
func NewJournaldSource(_ FactoryConfig) *JournaldSource {
	return &JournaldSource{info: SourceInfo{
		Type:        string(SourceTypeJournald),
		Description: "systemd journal (unsupported on Windows)",
		StartedAt:   time.Now(),
	}}
}

// Info 实现 Source 接口。
func (js *JournaldSource) Info() SourceInfo { return js.info }

// Read 实现 Source 接口：返回不支持错误。
func (js *JournaldSource) Read(_ context.Context, _ string, _ ScanConfig, _ io.Writer) (int64, error) {
	return 0, ErrJournaldUnsupported
}

// Tail 实现 Source 接口：返回不支持错误。
func (js *JournaldSource) Tail(_ context.Context, _ string, _ TailConfig, _ func(line string) error) error {
	return ErrJournaldUnsupported
}

// Discover 实现 Source 接口：返回空快照。
func (js *JournaldSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent, 1)
	ch <- DiscoveryEvent{Type: "snapshot"}
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func init() {
	RegisterSource(SourceTypeJournald, func(_ FactoryConfig) (Source, error) {
		return NewJournaldSource(FactoryConfig{}), nil
	})
}
