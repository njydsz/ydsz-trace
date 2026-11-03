// Package spool 提供本地磁盘缓冲队列（V2.2-09，FR-103）。
//
// 背景：
//   服务端不可达时（网络分区、服务端重启），采集/分析端若直接丢弃数据，
//   会造成故障期间的日志缺失——而故障期的日志恰恰最有价值。
//   本包提供一个有界本地队列：生产者写入磁盘文件，消费者在链路恢复后补传。
//
// 设计要点：
//   - 有界：超过容量上限时返回 ErrQueueFull（由调用方决定丢弃策略），避免磁盘写满
//   - 持久：数据落盘，进程重启不丢失
//   - 单写单读：通过互斥锁保证并发安全
//   - 幂等：消费者按序读取，失败可重试（读指针不推进）
//
// 文件格式：
//
//	[4B magic "YDSP"][4B version][4B flags]
//	[8B payload length][payload bytes][8B payload length][payload bytes]...
//
// 读取采用「顺序扫描 + 读指针」模式：写指针追加，读指针前进。
// 当读指针追上写指针时文件视为已消费完，可重置复用。
package spool

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// 文件格式常量。
const (
	// Magic 文件头标识（"YDSP" = Ydsz Spool）。
	Magic = "YDSP"
	// FormatVersion 当前格式版本。
	FormatVersion = 1
	// HeaderSize 文件头长度（magic + version + flags）。
	HeaderSize = 12
	// LengthBytes 每条记录的长度字段字节数。
	LengthBytes = 8
	// MaxPayloadSize 单条记录最大负载（16MB，防止异常数据撑爆内存）。
	MaxPayloadSize = 16 * 1024 * 1024
)

// 错误定义。
var (
	// ErrQueueFull 队列已满（调用方应丢弃最旧数据或阻塞）。
	ErrQueueFull = errors.New("spool: 队列已满")
	// ErrPayloadTooLarge 单条负载超过上限。
	ErrPayloadTooLarge = errors.New("spool: 负载超过上限")
	// ErrClosed 队列已关闭。
	ErrClosed = errors.New("spool: 队列已关闭")
)

// DefaultCapacity 默认队列容量（条数）。
const DefaultCapacity = 10000

// Queue 有界磁盘缓冲队列。
type Queue struct {
	mu sync.Mutex

	path     string
	capacity int   // 最大记录条数
	weight   int   // 当前记录条数
	head     int64 // 读指针（字节偏移）
	tail     int64 // 写指针（字节偏移）
	file     *os.File
	closed   bool
}

// QueueStats 队列状态统计。
type QueueStats struct {
	// Count 当前待消费记录数
	Count int `json:"count"`
	// Capacity 容量上限
	Capacity int `json:"capacity"`
	// FileSize 底层文件大小（字节）
	FileSize int64 `json:"fileSize"`
	// Path 队列文件路径
	Path string `json:"path"`
}

// OpenQueue 打开或创建队列。
//
// path 为空时使用默认路径 <YDSZ_SPOOL_DIR>/queue.bin。
// capacity <= 0 时使用 DefaultCapacity。
func OpenQueue(path string, capacity int) (*Queue, error) {
	if path == "" {
		path = defaultPath()
	}
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("spool: 创建队列目录失败: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("spool: 打开队列文件失败: %w", err)
	}

	q := &Queue{
		path:     path,
		capacity: capacity,
		file:     f,
		head:     0,
		tail:     0,
	}

	if err := q.init(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return q, nil
}

// defaultPath 解析默认队列文件路径。
func defaultPath() string {
	if v := os.Getenv("YDSZ_SPOOL_DIR"); v != "" {
		return filepath.Join(v, "queue.bin")
	}
	if v := os.Getenv("YDSZ_DATA_DIR"); v != "" {
		return filepath.Join(v, "spool.bin")
	}
	return "spool.bin"
}

// init 初始化文件头并恢复读写指针。
func (q *Queue) init() error {
	fi, err := q.file.Stat()
	if err != nil {
		return fmt.Errorf("spool: 获取队列文件状态失败: %w", err)
	}

	if fi.Size() == 0 {
		if err := q.writeHeader(); err != nil {
			return err
		}
		q.head = HeaderSize
		q.tail = HeaderSize
		return nil
	}

	if fi.Size() < HeaderSize {
		return fmt.Errorf("spool: 队列文件损坏（过短 %d 字节）", fi.Size())
	}

	magic := make([]byte, 4)
	if _, err := q.file.ReadAt(magic, 0); err != nil {
		return fmt.Errorf("spool: 读取队列头失败: %w", err)
	}
	if string(magic) != Magic {
		return fmt.Errorf("spool: 队列文件格式不匹配 (magic=%q)", string(magic))
	}

	return q.recoverCursors()
}

// writeHeader 写入文件头。
func (q *Queue) writeHeader() error {
	buf := make([]byte, HeaderSize)
	copy(buf[0:4], Magic)
	binary.BigEndian.PutUint32(buf[4:8], FormatVersion)
	binary.BigEndian.PutUint32(buf[8:12], 0) // flags 保留
	if _, err := q.file.WriteAt(buf, 0); err != nil {
		return fmt.Errorf("spool: 写入队列头失败: %w", err)
	}
	return nil
}

// recoverCursors 扫描文件恢复读写指针。
//
// 从头部顺序读取每条记录的长度字段，遇 0 或非法值即视为写指针位置。
func (q *Queue) recoverCursors() error {
	offset := int64(HeaderSize)
	weight := 0

	for {
		lenBuf := make([]byte, LengthBytes)
		n, err := q.file.ReadAt(lenBuf, offset)
		if err != nil || n < LengthBytes {
			break
		}

		length := binary.BigEndian.Uint64(lenBuf)
		if length > MaxPayloadSize {
			break
		}

		// length == 0 表示空负载记录，属合法数据（非哨兵）
		offset += LengthBytes + int64(length)
		weight++

		// 防御性上限，避免损坏文件导致无限循环
		if weight > q.capacity*10 {
			break
		}
	}

	q.head = HeaderSize
	q.tail = offset
	q.weight = weight
	return nil
}

// Enqueue 写入一条记录。
//
// 队列已满时返回 ErrQueueFull（调用方需决定丢弃策略）。
func (q *Queue) Enqueue(payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrClosed
	}
	if q.weight >= q.capacity {
		return ErrQueueFull
	}

	lenBuf := make([]byte, LengthBytes)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(payload)))
	if _, err := q.file.WriteAt(lenBuf, q.tail); err != nil {
		return fmt.Errorf("spool: 写入长度字段失败: %w", err)
	}
	if _, err := q.file.WriteAt(payload, q.tail+LengthBytes); err != nil {
		return fmt.Errorf("spool: 写入负载失败: %w", err)
	}

	q.tail += LengthBytes + int64(len(payload))
	q.weight++
	return nil
}

// Dequeue 读取一条记录并推进读指针。
//
// 队列为空闲时返回 (nil, io.EOF)。
// 读取失败时返回 error 且不推进指针，调用方可重试。
func (q *Queue) Dequeue() ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil, ErrClosed
	}
	if q.head >= q.tail {
		return nil, io.EOF
	}

	lenBuf := make([]byte, LengthBytes)
	if _, err := q.file.ReadAt(lenBuf, q.head); err != nil {
		return nil, fmt.Errorf("spool: 读取长度字段失败: %w", err)
	}
	length := binary.BigEndian.Uint64(lenBuf)
	if length > MaxPayloadSize {
		return nil, fmt.Errorf("spool: 记录长度非法 (%d)", length)
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := q.file.ReadAt(payload, q.head+LengthBytes); err != nil {
			return nil, fmt.Errorf("spool: 读取负载失败: %w", err)
		}
	}

	q.head += LengthBytes + int64(length)
	q.weight--
	return payload, nil
}

// Peek 读取一条记录但不推进读指针（供「读取-处理-提交」模式使用）。
func (q *Queue) Peek() ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil, ErrClosed
	}
	if q.head >= q.tail {
		return nil, io.EOF
	}

	lenBuf := make([]byte, LengthBytes)
	if _, err := q.file.ReadAt(lenBuf, q.head); err != nil {
		return nil, fmt.Errorf("spool: 读取长度字段失败: %w", err)
	}
	length := binary.BigEndian.Uint64(lenBuf)
	if length > MaxPayloadSize {
		return nil, fmt.Errorf("spool: 记录长度非法 (%d)", length)
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := q.file.ReadAt(payload, q.head+LengthBytes); err != nil {
			return nil, fmt.Errorf("spool: 读取负载失败: %w", err)
		}
	}
	return payload, nil
}

// Commit 提交读取进度（在业务确认处理成功后调用）。
//
// 读指针追上写指针时重置文件，避免文件无限增长。
func (q *Queue) Commit() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrClosed
	}
	if q.head < q.tail {
		return nil
	}

	if err := q.file.Truncate(0); err != nil {
		return fmt.Errorf("spool: 截断队列文件失败: %w", err)
	}
	if err := q.writeHeader(); err != nil {
		return err
	}
	q.head = HeaderSize
	q.tail = HeaderSize
	q.weight = 0
	return nil
}

// Stats 返回队列统计。
func (q *Queue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	fi, err := q.file.Stat()
	var size int64
	if err == nil {
		size = fi.Size()
	}
	return QueueStats{
		Count:    q.weight,
		Capacity: q.capacity,
		FileSize: size,
		Path:     q.path,
	}
}

// Close 关闭队列（落盘由 Close 内部完成）。
func (q *Queue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil
	}
	q.closed = true
	if err := q.file.Sync(); err != nil {
		_ = q.file.Close()
		return fmt.Errorf("spool: 同步队列文件失败: %w", err)
	}
	return q.file.Close()
}
