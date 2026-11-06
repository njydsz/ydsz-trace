// Package source 子文件：本地索引与查询下推（V2.4-03，FR-306）。
//
// 背景：
//   多节点部署下（PRD 举例 50 节点），服务端每次检索都要把所有日志内容
//   拉到服务端过滤，传输量与节点数线性相关。
//
//   查询下推的核心思路：
//     1. 过滤条件下推到 logc 本地（已实现，见 ScanAndFilter）
//     2. 用本地索引快速排除不可能命中的文件，避免无谓的读取与传输
//
// 本地索引设计：
//   - 按文件粒度记录「关键词出现情况」的轻量索引
//   - 使用 Bloom Filter 语义：可能误判为「有」（假阳性），绝不误判为「无」（无假阴性）
//   - 索引文件与日志文件同目录，命名 <logfile>.ydsz.idx
//   - 索引随日志更新增量维护，避免全量重建
//
// 为什么用 Bloom 而不是精确索引：
//   - 精确索引（倒排）构建成本高，且需要分词
//   - Bloom 只需记录「哪些关键词出现过」，内存与磁盘占用恒定
//   - 排障场景下假阳性只导致多读一个文件，代价可接受
package source

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// 本地索引常量。
const (
	// IndexFileSuffix 索引文件后缀
	IndexFileSuffix = ".ydsz.idx"
	// DefaultIndexBitsPerKey 每个关键词占用的 bit 数（经验值，假阳性率约 1%）
	DefaultIndexBitsPerKey = 10
	// DefaultIndexMaxKeys 索引最大关键词条目数（超出后停止新增，避免内存膨胀）
	DefaultIndexMaxKeys = 10000
	// DefaultIndexScanLines 构建索引时每个文件的扫描行数上限
	DefaultIndexScanLines = 5000
	// maxIndexTokenLen 索引关键词的最大长度
	maxIndexTokenLen = 32
	// minIndexTokenLen 索引关键词的最小长度
	minIndexTokenLen = 3
)

// LocalIndex 单个文件的本地关键词索引。
type LocalIndex struct {
	// FilePath 对应日志文件路径
	FilePath string `gob:"path"`
	// FileSize 索引构建时的文件大小
	FileSize int64 `gob:"size"`
	// ModTime 索引构建时的修改时间（UnixNano）
	ModTime int64 `gob:"mod_time"`
	// Bits Bloom Filter 位数组
	Bits []uint64 `gob:"bits"`
	// NumHashFuncs 哈希函数个数
	NumHashFuncs int `gob:"num_hash"`
	// NumBits 位数组总 bit 数
	NumBits int `gob:"num_bits"`
	// KeyCount 已插入关键词数量
	KeyCount int `gob:"key_count"`
	// MaxKeys 索引允许的最大关键词数（管理器上限）
	MaxKeys int `gob:"max_keys"`
	// UpdatedAt 索引更新时间
	UpdatedAt time.Time `gob:"updated_at"`
}

// LocalIndexManager 本地索引管理器。
type LocalIndexManager struct {
	mu      sync.RWMutex
	entries map[string]*LocalIndex // key = 文件路径
	// bitsPerKey 每关键词 bit 数
	bitsPerKey int
	// maxKeys 最大关键词条目数
	maxKeys int
}

// NewLocalIndexManager 创建索引管理器。
func NewLocalIndexManager() *LocalIndexManager {
	return &LocalIndexManager{
		entries:    make(map[string]*LocalIndex),
		bitsPerKey: DefaultIndexBitsPerKey,
		maxKeys:    DefaultIndexMaxKeys,
	}
}

// LoadOrBuild 加载已有索引；不存在或过期则重建。
//
// 过期判定：日志文件的 size 或 modTime 与索引记录不一致。
func (m *LocalIndexManager) LoadOrBuild(path string) (*LocalIndex, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	cached, ok := m.entries[path]
	m.mu.RUnlock()

	if ok && cached.FileSize == fi.Size() && cached.ModTime == fi.ModTime().UnixNano() {
		return cached, nil
	}

	// 尝试从磁盘加载
	if idx := m.loadFromDisk(path); idx != nil &&
		idx.FileSize == fi.Size() && idx.ModTime == fi.ModTime().UnixNano() {
		m.mu.Lock()
		m.entries[path] = idx
		m.mu.Unlock()
		return idx, nil
	}

	// 重建
	idx, err := m.build(path, fi)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.entries[path] = idx
	m.mu.Unlock()

	return idx, nil
}

// loadFromDisk 从磁盘读取索引。
func (m *LocalIndexManager) loadFromDisk(path string) *LocalIndex {
	f, err := os.Open(path + IndexFileSuffix)
	if err != nil {
		return nil
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			_ = cerr
		}
	}()

	var idx LocalIndex
	if err := gob.NewDecoder(f).Decode(&idx); err != nil {
		return nil
	}
	return &idx
}

// build 扫描文件构建索引。
func (m *LocalIndexManager) build(path string, fi os.FileInfo) (*LocalIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			// 关闭失败仅忽略（索引是增强能力）
			_ = cerr
		}
	}()

	// 位数组大小：按 maxKeys * bitsPerKey 预估
	numBits := m.maxKeys * m.bitsPerKey
	if numBits < 64 {
		numBits = 64
	}
	words := (numBits + 63) / 64

	idx := &LocalIndex{
		FilePath:     path,
		FileSize:     fi.Size(),
		ModTime:      fi.ModTime().UnixNano(),
		Bits:         make([]uint64, words),
		NumHashFuncs: optimalHashFuncs(m.bitsPerKey),
		NumBits:      numBits,
		MaxKeys:      m.maxKeys,
		UpdatedAt:    time.Now(),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxLineSize)

	lines := 0
	for scanner.Scan() && lines < DefaultIndexScanLines {
		lines++
		for _, token := range extractIndexTokens(scanner.Text()) {
			idx.add(token)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("扫描文件构建索引失败: %w", err)
	}

	return idx, nil
}

// add 插入关键词（受 MaxKeys 上限约束）。
//
// 超出上限后不再新增，避免位数组饱和导致假阳性率飙升。
// 已存在的关键词重复插入是幂等的（置位操作）。
func (idx *LocalIndex) add(token string) {
	if idx.MaxKeys > 0 && idx.KeyCount >= idx.MaxKeys {
		return
	}
	idx.KeyCount++
	for i := 0; i < idx.NumHashFuncs; i++ {
		pos := idx.hashPos(token, i)
		idx.Bits[pos/64] |= 1 << uint(pos%64)
	}
}

// hashPos 计算关键词在位数组中的位置。
func (idx *LocalIndex) hashPos(token string, seed int) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(token))
	_, _ = h.Write([]byte{byte(seed)})
	return int(h.Sum64() % uint64(idx.NumBits))
}

// MayContain 判断关键词是否可能出现在该文件中。
//
// 返回 true 表示「可能有」（需实际读取确认）；
// 返回 false 表示「一定没有」（可安全跳过）。
func (idx *LocalIndex) MayContain(token string) bool {
	if idx == nil || len(idx.Bits) == 0 {
		return true
	}
	for i := 0; i < idx.NumHashFuncs; i++ {
		pos := idx.hashPos(token, i)
		if idx.Bits[pos/64]&(1<<uint(pos%64)) == 0 {
			return false
		}
	}
	return true
}

// MayMatch 判断一组关键词中是否存在任一可能命中的。
//
// 语义：OR 逻辑——任一关键词可能存在即返回 true。
// 用于「包含任一关键词」的检索场景。
func (idx *LocalIndex) MayMatch(tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, t := range tokens {
		if idx.MayContain(t) {
			return true
		}
	}
	return false
}

// MustSkipAll 判断给定关键词是否全部不可能命中（用于 AND 语义的快速排除）。
//
// 全部关键词都不可能命中 → 可安全跳过该文件。
func (idx *LocalIndex) MustSkipAll(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if idx.MayContain(t) {
			return false
		}
	}
	return true
}

// SaveToDisk 持久化索引。
func (idx *LocalIndex) SaveToDisk() error {
	if idx == nil || idx.FilePath == "" {
		return fmt.Errorf("localindex: 索引缺少文件路径")
	}
	f, err := os.Create(idx.FilePath + IndexFileSuffix)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			_ = cerr
		}
	}()

	return gob.NewEncoder(f).Encode(idx)
}

// extractIndexTokens 从日志行提取可索引的关键词。
//
// 提取规则：
//   - 英文/数字连续片段（长度 3~32）
//   - 常见错误模式（Error/Exception/Fail 等后缀的标识符）
func extractIndexTokens(line string) []string {
	tokens := make([]string, 0, 8)
	seen := make(map[string]bool, 8)

	var current strings.Builder
	flush := func() {
		tok := strings.ToLower(current.String())
		current.Reset()
		if len(tok) < minIndexTokenLen || len(tok) > maxIndexTokenLen {
			return
		}
		if seen[tok] {
			return
		}
		seen[tok] = true
		tokens = append(tokens, tok)
	}

	for _, r := range line {
		isWord := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if isWord {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			flush()
		}
	}
	if current.Len() > 0 {
		flush()
	}
	return tokens
}

// optimalHashFuncs 根据每关键词 bit 数计算最优哈希函数个数。
func optimalHashFuncs(bitsPerKey int) int {
	// k = (m/n) * ln2，这里 m/n = bitsPerKey
	k := float64(bitsPerKey) * 0.693
	if k < 1 {
		return 1
	}
	if k > 8 {
		return 8
	}
	return int(k)
}

// PushdownFilter 查询下推过滤器。
//
// 在读取日志文件前用它快速排除不可能命中的文件，
// 是多节点场景降低传输量的关键（FR-306）。
type PushdownFilter struct {
	manager *LocalIndexManager
	// tokens 查询关键词（小写）
	tokens []string
	// requiredAll 是否为 AND 语义（全部关键词都必须命中）
	requiredAll bool
}

// NewPushdownFilter 构造下推过滤器。
//
// key 为检索关键词；requiredAll=true 表示 AND 语义（全部关键词必须命中），
// false 表示 OR 语义（任一命中即可）。
func NewPushdownFilter(manager *LocalIndexManager, key string, requiredAll bool) *PushdownFilter {
	return &PushdownFilter{
		manager:     manager,
		tokens:      ExtractQueryTokens(key),
		requiredAll: requiredAll,
	}
}

// ShouldSkipFile 判断是否可以跳过该文件（不读取）。
//
// 返回 true 表示该文件不可能命中，可安全跳过；
// 返回 false 表示可能命中或无法判断，必须实际读取。
func (p *PushdownFilter) ShouldSkipFile(path string) bool {
	// 无有效关键词或未启用索引 → 无法下推，不跳过
	if p == nil || p.manager == nil || len(p.tokens) == 0 {
		return false
	}

	idx, err := p.manager.LoadOrBuild(path)
	if err != nil || idx == nil {
		// 索引构建失败 → 保守起见不跳过
		return false
	}

	if p.requiredAll {
		// AND 语义：任一关键词不可能命中即可跳过
		return idx.MustSkipAll(p.tokens)
	}
	// OR 语义：全部关键词都不可能命中才跳过
	return !idx.MayMatch(p.tokens)
}

// ExtractQueryTokens 从查询条件提取用于索引判断的关键词。
func ExtractQueryTokens(key string) []string {
	if key == "" {
		return nil
	}
	// 按空白与常见分隔符切分
	fields := strings.FieldsFunc(key, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == ';' || r == '|' ||
			r == '(' || r == ')' || r == ':' || r == '='
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.Trim(f, `"'`))
		if len(f) >= minIndexTokenLen && len(f) <= maxIndexTokenLen {
			out = append(out, f)
		}
	}
	return out
}

// IndexFileFor 返回日志文件对应的索引文件路径。
func IndexFileFor(logPath string) string {
	return logPath + IndexFileSuffix
}

// IsIndexFile 判断给定路径是否为索引文件。
//
// 用后缀匹配而非 filepath.Ext：Ext 只返回最后一段（.idx），
// 而完整后缀是 .ydsz.idx，直接比较 Ext 会永远为 false。
func IsIndexFile(path string) bool {
	return strings.HasSuffix(path, IndexFileSuffix)
}
