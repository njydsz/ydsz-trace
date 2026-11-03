package spool

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// newTestQueue 在临时目录创建队列。
func newTestQueue(t *testing.T, capacity int) *Queue {
	t.Helper()
	q, err := OpenQueue(filepath.Join(t.TempDir(), "q.bin"), capacity)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}

// TestQueue_EnqueueDequeue 基本读写顺序。
func TestQueue_EnqueueDequeue(t *testing.T) {
	q := newTestQueue(t, 10)

	for i := 0; i < 5; i++ {
		if err := q.Enqueue([]byte{byte(i)}); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	for i := 0; i < 5; i++ {
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if len(got) != 1 || got[0] != byte(i) {
			t.Errorf("item %d = %v want [%d]", i, got, i)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, io.EOF) {
		t.Errorf("空队列应返回 EOF, got %v", err)
	}
}

// TestQueue_PayloadIntegrity 复杂负载（含空字节、中文）不损坏。
func TestQueue_PayloadIntegrity(t *testing.T) {
	q := newTestQueue(t, 5)
	cases := [][]byte{
		[]byte("plain text"),
		[]byte{0x00, 0x01, 0x02, 0x00},
		[]byte("中文日志内容，含换行\n和制表符\t"),
		[]byte(""),
	}

	for i, c := range cases {
		if err := q.Enqueue(c); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	for i, want := range cases {
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("item %d = %q want %q", i, got, want)
		}
	}
}

// TestQueue_CapacityLimit 超过容量返回 ErrQueueFull。
func TestQueue_CapacityLimit(t *testing.T) {
	q := newTestQueue(t, 3)
	for i := 0; i < 3; i++ {
		if err := q.Enqueue([]byte("x")); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if err := q.Enqueue([]byte("overflow")); !errors.Is(err, ErrQueueFull) {
		t.Errorf("应返回 ErrQueueFull, got %v", err)
	}
}

// TestQueue_PayloadTooLarge 超限负载被拒绝。
func TestQueue_PayloadTooLarge(t *testing.T) {
	q := newTestQueue(t, 10)
	big := make([]byte, MaxPayloadSize+1)
	if err := q.Enqueue(big); !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("应返回 ErrPayloadTooLarge, got %v", err)
	}
}

// TestQueue_PeekDoesNotAdvance Peek 不推进读指针。
func TestQueue_PeekDoesNotAdvance(t *testing.T) {
	q := newTestQueue(t, 5)
	_ = q.Enqueue([]byte("first"))

	a, err := q.Peek()
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	b, err := q.Peek()
	if err != nil {
		t.Fatalf("peek again: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("Peek 不应推进指针")
	}

	// 读一次后仍应能读到同一条
	c, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if !bytes.Equal(a, c) {
		t.Error("Dequeue 应返回 Peek 过的同一条")
	}
}

// TestQueue_ReopenRecoversData 重开后数据可恢复（重启补传场景）。
func TestQueue_ReopenRecoversData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.bin")

	q1, err := OpenQueue(path, 10)
	if err != nil {
		t.Fatalf("open q1: %v", err)
	}
	_ = q1.Enqueue([]byte("a"))
	_ = q1.Enqueue([]byte("b"))
	if err := q1.Close(); err != nil {
		t.Fatalf("close q1: %v", err)
	}

	q2, err := OpenQueue(path, 10)
	if err != nil {
		t.Fatalf("open q2: %v", err)
	}
	defer func() { _ = q2.Close() }()

	got, err := q2.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if !bytes.Equal(got, []byte("a")) {
		t.Errorf("got %q want a", got)
	}
}

// TestQueue_CommitResetsWhenDrained 全部消费后 Commit 重置文件。
func TestQueue_CommitResetsWhenDrained(t *testing.T) {
	q := newTestQueue(t, 10)
	_ = q.Enqueue([]byte("data"))
	before := q.Stats().FileSize
	if before <= HeaderSize {
		t.Fatalf("file should have grown, size=%d", before)
	}

	if _, err := q.Dequeue(); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if err := q.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	after := q.Stats()
	if after.Count != 0 {
		t.Errorf("count = %d want 0", after.Count)
	}
	if after.FileSize != HeaderSize {
		t.Errorf("file size = %d want %d", after.FileSize, HeaderSize)
	}
}

// TestQueue_CommitWithPendingKeepsData 仍有未消费数据时 Commit 不重置。
func TestQueue_CommitWithPendingKeepsData(t *testing.T) {
	q := newTestQueue(t, 10)
	_ = q.Enqueue([]byte("a"))
	_ = q.Enqueue([]byte("b"))

	if _, err := q.Dequeue(); err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	// 还剩一条，Commit 不应重置
	if err := q.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue after commit: %v", err)
	}
	if !bytes.Equal(got, []byte("b")) {
		t.Errorf("got %q want b", got)
	}
}

// TestQueue_CloseIdempotent 重复关闭不 panic。
func TestQueue_CloseIdempotent(t *testing.T) {
	q, err := OpenQueue(filepath.Join(t.TempDir(), "q.bin"), 5)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Errorf("second close should be nil, got %v", err)
	}
}

// TestQueue_OperationsAfterClose 关闭后操作返回 ErrClosed。
func TestQueue_OperationsAfterClose(t *testing.T) {
	q, err := OpenQueue(filepath.Join(t.TempDir(), "q.bin"), 5)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = q.Close()

	if err := q.Enqueue([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Errorf("enqueue after close = %v want ErrClosed", err)
	}
	if _, err := q.Dequeue(); !errors.Is(err, ErrClosed) {
		t.Errorf("dequeue after close = %v want ErrClosed", err)
	}
	if err := q.Commit(); !errors.Is(err, ErrClosed) {
		t.Errorf("commit after close = %v want ErrClosed", err)
	}
}

// TestQueue_CorruptedMagic 损坏文件返回错误。
func TestQueue_CorruptedMagic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(path, []byte("XXXX!!!!!!!!"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := OpenQueue(path, 5); err == nil {
		t.Fatal("损坏 magic 应返回错误")
	}
}

// TestQueue_TooShortFile 过短文件返回错误。
func TestQueue_TooShortFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(path, []byte("YD"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := OpenQueue(path, 5); err == nil {
		t.Fatal("过短文件应返回错误")
	}
}

// TestQueue_EmptyPayload 空负载可正常读写。
func TestQueue_EmptyPayload(t *testing.T) {
	q := newTestQueue(t, 5)
	if err := q.Enqueue([]byte{}); err != nil {
		t.Fatalf("enqueue empty: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v want empty", got)
	}
}

// TestQueueStats 统计信息准确性。
func TestQueueStats(t *testing.T) {
	q := newTestQueue(t, 5)
	if s := q.Stats(); s.Capacity != 5 || s.Count != 0 {
		t.Errorf("initial stats = %+v", s)
	}
	_ = q.Enqueue([]byte("x"))
	if s := q.Stats(); s.Count != 1 {
		t.Errorf("after enqueue count = %d want 1", s.Count)
	}
}
