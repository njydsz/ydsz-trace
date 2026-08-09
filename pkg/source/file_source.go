package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileSource 代表本地文件系统上的日志采集源。
//
// 适用场景：传统部署 / 挂载 volume 的容器 / 开发环境。
// path 参数直接对应 host 上的日志文件绝对路径。
type FileSource struct {
	rootDir string // 可选：限制日志必须在 rootDir 下遍历（安全沙箱）
	info    SourceInfo
}

// FileSourceOption FileSource 的可选配置。
type FileSourceOption func(*FileSource)

// WithRootDir 设置 FileSource 的根目录（只允许此目录下的文件）。
func WithRootDir(dir string) FileSourceOption {
	return func(fs *FileSource) {
		fs.rootDir = dir
	}
}

// NewFileSource 创建 FileSource 实例。
// rootDir 非空时，所有 Read 路径必须在其下（安全校验）。
func NewFileSource(opts ...FileSourceOption) *FileSource {
	fs := &FileSource{
		info: SourceInfo{
			Type:        "file",
			Description: "Local filesystem log source",
			StartedAt:   time.Now(),
		},
	}
	for _, o := range opts {
		o(fs)
	}
	return fs
}

// Read 实现 Source 接口：打开本地日志文件，应用过滤规则，结果写入 output。
func (fs *FileSource) Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	// 安全校验：rootDir 设置时禁止逃逸
	if fs.rootDir != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return 0, fmt.Errorf("无法获取路径绝对位置: %w", err)
		}
		absRoot, err := filepath.Abs(fs.rootDir)
		if err != nil {
			return 0, fmt.Errorf("无法获取根目录绝对位置: %w", err)
		}
		if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
			return 0, fmt.Errorf("路径超出允许的范围: %s", path)
		}
	}

	// 路径校验：防止 ".." 遍历
	if strings.Contains(path, "..") {
		return 0, fmt.Errorf("非法路径(包含..): %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("日志文件不存在: %s", path)
		}
		return 0, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	return ScanAndFilter(f, cfg, output)
}

// Tail 实现 Source 接口：跟踪文件末尾新增内容。
func (fs *FileSource) Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("日志文件不存在: %s", path)
		}
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	tf, err := NewTailFilter(cfg)
	if err != nil {
		return err
	}

	// Seek 到文件末尾
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek 文件末尾失败: %w", err)
	}

	reader := bufio.NewReader(f)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var deadline time.Time
	if cfg.FollowDuration > 0 {
		deadline = time.Now().Add(time.Duration(cfg.FollowDuration) * time.Second)
	}

	for {
		select {
		case <-ctx.Done():
			return nil // ctx 取消表示优雅退出
		case <-ticker.C:
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil
		}

		for {
			line, rerr := reader.ReadString('\n')
			if len(line) > 0 {
				line = strings.TrimRight(line, "\r\n")
				if line != "" && tf.Match(line) {
					if err := callback(line); err != nil {
						return err
					}
				}
			}
			if rerr != nil {
				break
			}
		}
	}
}

// Discover 对 FileSource 是一个空操作（文件模式下目标是静态配置的）。
// 仅返回一个已关闭的 channel。
func (fs *FileSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent)
	close(ch)
	return ch, nil
}

// Info 返回 FileSource 的摘要。
func (fs *FileSource) Info() SourceInfo {
	return fs.info
}

// ResolvePath 校验并返回日志文件的安全路径（对外暴露的辅助函数）。
//
// 如果 rootDir 已设置且 path 不在其下，返回错误；如果 path 含 ".."，返回错误。
func (fs *FileSource) ResolvePath(path string) (string, error) {
	if fs.rootDir == "" {
		return path, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(fs.rootDir)
	if err != nil {
		return "", err
	}
	if abs != rootAbs && !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("路径 %s 超出根目录 %s", path, fs.rootDir)
	}
	return abs, nil
}

// Exists 快速检查文件是否存在。
func (fs *FileSource) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func init() {
	// 日志输出降噪（避免库内部 log 过多）：仅在环境变量 YDSZ_SOURCE_DEBUG=1 时打印。
	if os.Getenv("YDSZ_SOURCE_DEBUG") == "1" {
		log.SetFlags(log.Ltime | log.Lshortfile)
	}
}
