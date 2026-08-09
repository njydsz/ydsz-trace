package source

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileSource 代表本地文件系统上的日志采集源。
//
// 适用场景：传统部署 / 挂载 volume 的容器 / 开发环境。
// path 参数直接对应 host 上的日志文件绝对路径（或者一条 glob 模式，见 Read）。
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

// Read 实现 Source 接口：打开本地日志文件，应用过滤规则，结果写入 output。//
// 路径解析规则（优先级）：
//  1. 若 path 精确指向一个已存在的文件 → 单文件读取（保持历史行为）
//  2. 否则若 path 是 glob 模式 → 展开匹配多个文件，按 logrotate 逆序聚合读取
//     （匹配顺序：数字后缀越大越靠前，即更旧的日志先读取）
//  3. 均不匹配 → 返回 "无匹配日志文件" 错误
//
// 支持 .gz 结尾的日志文件透明解压（logrotate 常产生 *.log.N.gz 归档）。
func (fs *FileSource) Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	candidates, err := fs.resolveCandidates(path)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, fmt.Errorf("无匹配日志文件: %s", path)
	}

	var total int64
	for _, p := range candidates {
		n, err := readOneFile(ctx, p, cfg, output)
		if err != nil {
			log.Printf("[source]读取 %s 失败: %v", p, err)
			continue
		}
		total += n
	}
	return total, nil
}

// resolveCandidates 根据单文件 / glob 展开多个候选文件。
// 结果保证按 logrotate 时间逆序（更旧的日志先输出）。
func (fs *FileSource) resolveCandidates(path string) ([]string, error) {
	// 1) 精确单文件 → 直接读取
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		if err := fs.checkSafe(path); err != nil {
			return nil, err
		}
		return []string{path}, nil
	}

	// 2) glob 扩张
	if !isGlobPattern(path) {
		return nil, fmt.Errorf("日志文件不存在: %s", path)
	}
	matches, err := filepath.Glob(path)
	if err != nil {
		return nil, fmt.Errorf("glob 表达式非法: %s: %w", path, err)
	}
	// 过滤非法路径 / 目录，按 logrotate 逆序
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if fi, err := os.Stat(m); err != nil || fi.IsDir() {
			continue
		}
		if err := fs.checkSafe(m); err != nil {
			log.Printf("[source]跳过不安全的 glob 匹配: %s: %v", m, err)
			continue
		}
		out = append(out, m)
	}
	sortLogrotateDesc(out)
	return out, nil
}

// checkSafe 安全校验：rootDir 沙箱 + 路径遍历防护。
func (fs *FileSource) checkSafe(path string) error {
	// 路径遍历防护：任何候选路径禁止含 ".."
	if strings.Contains(path, "..") {
		return fmt.Errorf("非法路径(包含..): %s", path)
	}
	if fs.rootDir == "" {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("无法获取路径绝对位置: %w", err)
	}
	absRoot, err := filepath.Abs(fs.rootDir)
	if err != nil {
		return fmt.Errorf("无法获取根目录绝对位置: %w", err)
	}
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return fmt.Errorf("路径超出允许的范围: %s", path)
	}
	return nil
}

// readOneFile 打开单个文件（支持 .gz 透明解密），逐行应用过滤并写入 output。
// 读完后自动关闭底层文件。返回写入的总字节数（含文件间的分隔空行）。
func readOneFile(_ context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	rc, err := openLogFile(path)
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	// 包裹一个计数 Writer，方便汇总实际写入字节数（含分隔行）。
	cw := &countingWriter{w: output}
	if _, err := ScanAndFilter(rc, cfg, cw); err != nil {
		return cw.n, err
	}
	// 多个文件之间用一行空行分隔，避免日志跨文件"融合"。
	if _, werr := cw.Write([]byte("\n")); werr != nil {
		return cw.n, werr
	}
	return cw.n, nil
}

// countingWriter 在 io.Writer 之上累计写入字节数。
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// openLogFile 打开一个日志文件：
//
//   - 路径以 .gz 结尾 → 按 gzip 透明解压打开
//   - 否则普通打开
//
// 返回的 ReadCloser 在 Close 时会一并关闭底层文件。
func openLogFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("日志文件不存在: %s", path)
		}
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gr, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip 解压失败: %w", err)
		}
		return &gzipFileReader{file: f, gz: gr}, nil
	}
	return f, nil
}

// gzipFileReader 将 *gzip.Reader 与 *os.File 一起生命周期管理：
// Close 时先关 gzip 再关底层文件。
type gzipFileReader struct {
	file *os.File
	gz   *gzip.Reader
}

func (g *gzipFileReader) Read(p []byte) (int, error) { return g.gz.Read(p) }
func (g *gzipFileReader) Close() error {
	var firstErr error
	if err := g.gz.Close(); err != nil {
		firstErr = err
	}
	if err := g.file.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// isGlobPattern 简单判断路径是否包含 glob 元字符。
func isGlobPattern(path string) bool {
	return strings.ContainsAny(path, "*?[]{}")
}

// sortLogrotateDesc 将文件列表按 logrotate 数字后缀逆序排序（suffix 越大越靠前 = 更旧的日志）。
// 不匹配数字后缀的条目（如 app.log）视为后缀 0，排在最后（最新日志）。
// 排序是原地修改。
func sortLogrotateDesc(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		return extractLogrotateSuffix(files[i]) > extractLogrotateSuffix(files[j])
	})
}

// extractLogrotateSuffix 提取 logrotate 风格数字后缀，未匹配返回 0。
//
// 支持的典型格式：
//   - app.log.1   → 1
//   - app.log.2.gz → 2
//   - app.log.10.bz2 → 10
//   - app.log      → 0（当前最活跃日志）
func extractLogrotateSuffix(path string) int {
	base := filepath.Base(path)
	// 先剥离已知压缩扩展名
	for _, ext := range []string{".gz", ".bz2", ".zst", ".zip"} {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			base = base[:len(base)-len(ext)]
			break
		}
	}
	idx := strings.LastIndex(base, ".")
	if idx < 0 || idx == len(base)-1 {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(base[idx+1:], "%d", &n); err != nil {
		return 0
	}
	return n
}

// Tail 实现 Source 接口：跟踪文件末尾新增内容。
// 注意：Tail 仅支持单个具体文件；glob 路径请用 Read。
func (fs *FileSource) Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error {
	if isGlobPattern(path) {
		return fmt.Errorf("Tail 不支持 glob 路径（请使用具体文件）: %s", path)
	}
	if strings.Contains(path, "..") && fs.rootDir != "" {
		return fmt.Errorf("非法path参数(包含..): %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("日志文件不存在: %s", path)
		}
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	if err := fs.checkSafe(path); err != nil {
		return err
	}

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
// 注：ResolvePath 也不接受 glob 模式，仅用于单文件校验。
func (fs *FileSource) ResolvePath(path string) (string, error) {
	if isGlobPattern(path) {
		return "", fmt.Errorf("ResolvePath 不支持 glob 模式: %s", path)
	}
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
	// 注册 FileSource 工厂，让 CreateSource 按需构造。
	RegisterSource(SourceTypeFile, func(cfg FactoryConfig) (Source, error) {
		opts := []FileSourceOption{}
		if v, ok := cfg.Options["root_dir"]; ok {
			opts = append(opts, WithRootDir(v))
		}
		return NewFileSource(opts...), nil
	})
}
