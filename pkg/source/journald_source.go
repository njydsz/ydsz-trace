//go:build !windows

package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

// JournaldSource 通过 journalctl 读取 systemd journal 日志（V2.2-10，FR-101）。
//
// 实现方式：
//   调用系统 journalctl 并以 JSON 输出，解析后转为统一日志行。
//   之所以不直接走 D-Bus（sd-journal）：
//     - 避免引入 cgo 依赖，保持单二进制可移植
//     - journalctl 在所有 systemd 发行版上可用，行为一致
//     - 便于测试（可替换命令路径）
//
// 字段映射（journal JSON → 日志行）：
//
//	<PRIORITY><_HOSTNAME> <SYSLOG_IDENTIFIER>[<_PID>]: <MESSAGE>
//
// PRIORITY: 0=EMERG 1=ALERT 2=CRIT 3=ERR 4=WARNING 5=NOTICE 6=INFO 7=DEBUG
//
// 支持的配置项（FactoryConfig.Options）：
//   - unit         : 只读取指定 unit（可重复，逗号分隔）
//   - priority     : 最低优先级（0~7，数字越小越严重）
//   - since        : 起始时间（如 "-1h"、"2026-11-03 10:00:00"）
//   - journalctl  : journalctl 可执行路径（默认 "journalctl"）
type JournaldSource struct {
	info    SourceInfo
	unit    []string
	priority string
	since   string
	binPath string
}

// NewJournaldSource 创建 journald 源。
func NewJournaldSource(cfg FactoryConfig) *JournaldSource {
	js := &JournaldSource{
		binPath:  "journalctl",
		priority: "6", // 默认 INFO 及以上
		info: SourceInfo{
			Type:        string(SourceTypeJournald),
			Description: "systemd journal (via journalctl)",
			StartedAt:   time.Now(),
		},
	}

	if v, ok := cfg.Options["unit"]; ok && v != "" {
		for _, u := range strings.Split(v, ",") {
			if u = strings.TrimSpace(u); u != "" {
				js.unit = append(js.unit, u)
			}
		}
	}
	if v, ok := cfg.Options["priority"]; ok && v != "" {
		js.priority = v
	}
	if v, ok := cfg.Options["since"]; ok && v != "" {
		js.since = v
	}
	if v, ok := cfg.Options["journalctl"]; ok && v != "" {
		js.binPath = v
	}
	return js
}

// Info 实现 Source 接口。
func (js *JournaldSource) Info() SourceInfo { return js.info }

// Read 实现 Source 接口：导出匹配时间范围的 journal 记录。
func (js *JournaldSource) Read(ctx context.Context, _ string, cfg ScanConfig, output io.Writer) (int64, error) {
	args := []string{"-o", "json", "-p", js.priority, "--no-pager", "-q"}
	for _, u := range js.unit {
		args = append(args, "-u", u)
	}
	if js.since != "" {
		args = append(args, "--since", js.since)
	}

	cmd := exec.CommandContext(ctx, js.binPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("journald: 创建管道失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("journald: 启动 journalctl 失败: %w", err)
	}
	defer func() {
		// 等待进程结束，避免僵尸进程
		if werr := cmd.Wait(); werr != nil {
			log.Printf("[source] journalctl 退出: %v", werr)
		}
	}()

	// journalctl 输出为逐行 JSON，先解析为统一日志行再走通用过滤
	reader := newJournalReader(stdout)
	return ScanAndFilter(reader, cfg, output)
}

// Tail 实现 Source 接口：实时跟踪 journal（-f）。
func (js *JournaldSource) Tail(ctx context.Context, _ string, cfg TailConfig, callback func(line string) error) error {
	args := []string{"-o", "json", "-p", js.priority, "--no-pager", "-q", "-f"}
	for _, u := range js.unit {
		args = append(args, "-u", u)
	}

	cmd := exec.CommandContext(ctx, js.binPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("journald: 创建管道失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("journald: 启动 journalctl 失败: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), MaxLineSize)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := journalLineToText(scanner.Bytes())
			if line == "" {
				continue
			}
			if err := callback(line); err != nil {
				return
			}
		}
	}()

	<-ctx.Done()
	_ = cmd.Process.Kill()
	<-done
	return ctx.Err()
}

// Discover 实现 Source 接口：journald 无容器/Pod 概念，返回空快照。
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
	RegisterSource(SourceTypeJournald, func(cfg FactoryConfig) (Source, error) {
		return NewJournaldSource(cfg), nil
	})
}
