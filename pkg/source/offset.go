// Package source 子文件：读取位点持久化（V2.2-08，FR-102）。
//
// 背景：
//   logc 重启后若没有位点记录，会从头重读整个日志文件——既浪费 IO，
//   又会让同一批日志被重复分析。本模块以 (inode, path) 双键持久化
//   读取位点，重启后从断点续读。
//
// inode 的意义：
//   日志系统普遍使用 logrotate，轮转后原路径指向新文件。
//   仅按 path 记录 offset 会在轮转后把新文件的前 N 字节当成"已读"而跳过。
//   因此必须同时校验 inode：inode 变化 → 视为新文件 → 从 0 开始。
//
// 存储设计：
//   - 内存 map + 定期落盘（JSON 文件），避免每读一行就写磁盘
//   - 落盘路径通过 YDSZ_OFFSET_FILE 配置，默认 <data>/offsets.json
//   - 写盘采用「临时文件 + rename」保证原子性，避免崩溃时损坏
package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// OffsetRecord 单个文件的读取位点。
type OffsetRecord struct {
	// Path 文件路径（冗余存储，便于人工排查）
	Path string `json:"path"`
	// Inode 文件 inode（轮转检测的核心依据）
	Inode uint64 `json:"inode"`
	// Size 记录位点时的文件大小（用于判断文件是否被截断）
	Size int64 `json:"size"`
	// Offset 已读取的字节偏移
	Offset int64 `json:"offset"`
	// ModTime 文件修改时间（UnixNano，用于辅助判断轮转）
	ModTime int64 `json:"modTime"`
	// UpdatedAt 位点更新时间（Unix 秒）
	UpdatedAt int64 `json:"updatedAt"`
}

// OffsetStore 位点存储（并发安全）。
type OffsetStore struct {
	mu      sync.RWMutex
	records map[string]*OffsetRecord // key = path
	path    string                   // 落盘文件路径
	// dirty 标记是否有未落盘的修改，避免无变化时频繁写盘
	dirty bool
}

// DefaultOffsetFileEnv 位点文件路径环境变量名。
const DefaultOffsetFileEnv = "YDSZ_OFFSET_FILE"

// DefaultOffsetFileName 默认位点文件名。
const DefaultOffsetFileName = "offsets.json"

// NewOffsetStore 创建位点存储。
//
// filePath 为空时按以下顺序解析：
//  1. 环境变量 YDSZ_OFFSET_FILE
//  2. 环境变量 YDSZ_DATA_DIR + /offsets.json
//  3. 当前目录 offsets.json
func NewOffsetStore(filePath string) *OffsetStore {
	if filePath == "" {
		filePath = resolveOffsetPath()
	}
	s := &OffsetStore{
		records: make(map[string]*OffsetRecord),
		path:    filePath,
	}
	// 启动时尽力加载，失败不阻塞（首次运行为空位点属正常）
	_ = s.load()
	return s
}

// resolveOffsetPath 解析默认位点文件路径。
func resolveOffsetPath() string {
	if v := os.Getenv(DefaultOffsetFileEnv); v != "" {
		return v
	}
	if v := os.Getenv("YDSZ_DATA_DIR"); v != "" {
		return filepath.Join(v, DefaultOffsetFileName)
	}
	return DefaultOffsetFileName
}

// Get 读取指定位点；不存在时返回 (nil, false)。
func (s *OffsetStore) Get(path string) (*OffsetRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[path]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

// Put 写入/更新位点。
func (s *OffsetStore) Put(rec *OffsetRecord) {
	if rec == nil || rec.Path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec.UpdatedAt == 0 {
		rec.UpdatedAt = time.Now().Unix()
	}
	cp := *rec
	s.records[rec.Path] = &cp
	s.dirty = true
}

// Clear 删除指定位点（文件删除或重置时使用）。
func (s *OffsetStore) Clear(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[path]; ok {
		delete(s.records, path)
		s.dirty = true
	}
}

// Flush 将内存位点落盘。
//
// 采用临时文件 + rename 保证原子性；落盘失败仅记录日志不影响读取。
func (s *OffsetStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil
	}

	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化位点失败: %w", err)
	}

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建位点目录失败: %w", err)
		}
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写入临时位点文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		// 清理临时文件，避免残留（忽略清理错误）
		_ = os.Remove(tmp)
		return fmt.Errorf("原子替换位点文件失败: %w", err)
	}

	s.dirty = false
	return nil
}

// load 从磁盘加载位点。
func (s *OffsetStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var records map[string]*OffsetRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("解析位点文件失败: %w", err)
	}
	if s.records == nil {
		s.records = make(map[string]*OffsetRecord)
	}
	for k, v := range records {
		s.records[k] = v
	}
	return nil
}

// Count 返回当前位点数量（用于监控/日志）。
func (s *OffsetStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// ResolveResumeOffset 计算应从哪个偏移开始读取。
//
// 这是位点续读的核心判定逻辑：
//
//   - 无历史位点 → 返回 0（从头读）
//   - inode 与当前文件不一致 → 返回 0（logrotate 轮转，视为新文件）
//   - 当前文件大小 < 记录的 Size → 返回 0（文件被截断/重建）
//   - 否则返回记录的 Offset（断点续读）
//
// 参数 fileSize 为当前文件的实际大小。
func ResolveResumeOffset(rec *OffsetRecord, currentInode uint64, fileSize int64) int64 {
	if rec == nil {
		return 0
	}
	if rec.Inode != 0 && currentInode != 0 && rec.Inode != currentInode {
		return 0
	}
	if fileSize < rec.Offset {
		return 0
	}
	if rec.Size > 0 && fileSize < rec.Size {
		return 0
	}
	return rec.Offset
}

// FileInode 获取文件的 inode（跨平台的统一入口）。
//
// 非 Unix 平台返回 0，此时 ResolveResumeOffset 的 inode 校验会被跳过，
// 退化为仅按 path + size 判断（Windows 下 logrotate 少见，可接受）。
func FileInode(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return inodeOf(info)
}

// FormatOffset 将偏移格式化为可读字符串（用于日志）。
func FormatOffset(off int64) string {
	return strconv.FormatInt(off, 10)
}
