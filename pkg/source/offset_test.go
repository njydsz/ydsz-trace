package source

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOffsetStore_PutGet 基本读写。
func TestOffsetStore_PutGet(t *testing.T) {
	s := &OffsetStore{records: make(map[string]*OffsetRecord)}
	rec := &OffsetRecord{Path: "/var/log/app.log", Inode: 12345, Size: 1000, Offset: 500}
	s.Put(rec)

	got, ok := s.Get("/var/log/app.log")
	if !ok {
		t.Fatal("应能读取位点")
	}
	if got.Offset != 500 {
		t.Errorf("offset = %d want 500", got.Offset)
	}
	if got.Inode != 12345 {
		t.Errorf("inode = %d want 12345", got.Inode)
	}
}

// TestOffsetStore_Clear 删除位点。
func TestOffsetStore_Clear(t *testing.T) {
	s := &OffsetStore{records: make(map[string]*OffsetRecord)}
	s.Put(&OffsetRecord{Path: "/a.log", Offset: 10})
	s.Clear("/a.log")
	if _, ok := s.Get("/a.log"); ok {
		t.Fatal("位点应已删除")
	}
}

// TestOffsetStore_FlushAndReload 落盘后可重新加载。
func TestOffsetStore_FlushAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offsets.json")

	s := &OffsetStore{records: make(map[string]*OffsetRecord), path: path}
	s.Put(&OffsetRecord{Path: "/var/log/x.log", Inode: 999, Size: 2048, Offset: 1024, ModTime: 123})

	if err := s.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	// 新实例加载同一文件
	s2 := &OffsetStore{records: make(map[string]*OffsetRecord), path: path}
	if err := s2.load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	got, ok := s2.Get("/var/log/x.log")
	if !ok {
		t.Fatal("重载后应能读取位点")
	}
	if got.Offset != 1024 {
		t.Errorf("offset = %d want 1024", got.Offset)
	}
	if got.Inode != 999 {
		t.Errorf("inode = %d want 999", got.Inode)
	}
}

// TestOffsetStore_FlushNoDirtySkip 无修改时跳过落盘。
func TestOffsetStore_FlushNoDirtySkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offsets.json")

	s := &OffsetStore{records: make(map[string]*OffsetRecord), path: path}
	if err := s.Flush(); err != nil {
		t.Fatalf("empty flush should be no-op: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("无修改时不应创建位点文件")
	}
}

// TestResolveResumeOffset_NoRecord 无位点从头读。
func TestResolveResumeOffset_NoRecord(t *testing.T) {
	if off := ResolveResumeOffset(nil, 1, 1000); off != 0 {
		t.Errorf("offset = %d want 0", off)
	}
}

// TestResolveResumeOffset_SameInode 同 inode 续读。
func TestResolveResumeOffset_SameInode(t *testing.T) {
	rec := &OffsetRecord{Inode: 100, Size: 1000, Offset: 400}
	if off := ResolveResumeOffset(rec, 100, 2000); off != 400 {
		t.Errorf("offset = %d want 400", off)
	}
}

// TestResolveResumeOffset_InodeChanged logrotate 轮转后从头读。
func TestResolveResumeOffset_InodeChanged(t *testing.T) {
	rec := &OffsetRecord{Inode: 100, Size: 1000, Offset: 400}
	if off := ResolveResumeOffset(rec, 200, 500); off != 0 {
		t.Errorf("inode 变化应从头读, got %d", off)
	}
}

// TestResolveResumeOffset_FileTruncated 文件被截断从头读。
func TestResolveResumeOffset_FileTruncated(t *testing.T) {
	rec := &OffsetRecord{Inode: 100, Size: 2000, Offset: 1500}
	// 当前文件比记录的 offset 还小 → 说明被截断/重建
	if off := ResolveResumeOffset(rec, 100, 800); off != 0 {
		t.Errorf("文件截断应从头读, got %d", off)
	}
}

// TestResolveResumeOffset_SizeShrunk 文件小于记录 size 从头读。
func TestResolveResumeOffset_SizeShrunk(t *testing.T) {
	rec := &OffsetRecord{Inode: 100, Size: 5000, Offset: 100}
	if off := ResolveResumeOffset(rec, 100, 3000); off != 0 {
		t.Errorf("文件大小收缩应从头读, got %d", off)
	}
}

// TestResolveResumeOffset_ZeroInodeFallback 两端 inode 为 0 时跳过校验。
func TestResolveResumeOffset_ZeroInodeFallback(t *testing.T) {
	rec := &OffsetRecord{Inode: 0, Size: 1000, Offset: 400}
	// inode 未知（Windows 或 stat 失败）→ 退化为 size 判断，仍续读
	if off := ResolveResumeOffset(rec, 0, 2000); off != 400 {
		t.Errorf("inode 未知时应续读, got %d", off)
	}
}

// TestOffsetStore_GetReturnsCopy 返回副本，外部修改不影响内部状态。
func TestOffsetStore_GetReturnsCopy(t *testing.T) {
	s := &OffsetStore{records: make(map[string]*OffsetRecord)}
	s.Put(&OffsetRecord{Path: "/a.log", Offset: 10})

	got, _ := s.Get("/a.log")
	got.Offset = 9999

	again, _ := s.Get("/a.log")
	if again.Offset != 10 {
		t.Errorf("应返回副本, got %d", again.Offset)
	}
}

// TestOffsetStore_PutNilSafe nil 与空路径不 panic。
func TestOffsetStore_PutNilSafe(t *testing.T) {
	s := &OffsetStore{records: make(map[string]*OffsetRecord)}
	s.Put(nil)
	s.Put(&OffsetRecord{Path: ""})
	if s.Count() != 0 {
		t.Errorf("count = %d want 0", s.Count())
	}
}

// TestNewOffsetStore_FromEnv 从环境变量解析路径。
func TestNewOffsetStore_FromEnv(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "custom-offsets.json")
	t.Setenv(DefaultOffsetFileEnv, target)

	s := NewOffsetStore("")
	if s.path != target {
		t.Errorf("path = %q want %q", s.path, target)
	}
}

// TestNewOffsetStore_LoadMissingFile 位点文件不存在时不报错。
func TestNewOffsetStore_LoadMissingFile(t *testing.T) {
	s := NewOffsetStore(filepath.Join(t.TempDir(), "nope.json"))
	if s.Count() != 0 {
		t.Errorf("count = %d want 0", s.Count())
	}
}
