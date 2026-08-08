package util

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestZipUnZip_RoundTrip 验证压缩 → 解压后文件内容一致。
func TestZipUnZip_RoundTrip(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	zipFile := filepath.Join(t.TempDir(), "test.zip")

	content := []byte("hello ydsz-trace")
	if err := os.WriteFile(filepath.Join(src, "a.txt"), content, 0644); err != nil {
		t.Fatalf("写源文件失败: %v", err)
	}
	sub := filepath.Join(src, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), content, 0644); err != nil {
		t.Fatalf("写子目录文件失败: %v", err)
	}

	if err := Zip(zipFile, src); err != nil {
		t.Fatalf("Zip 失败: %v", err)
	}
	if err := UnZip(dst, zipFile); err != nil {
		t.Fatalf("UnZip 失败: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatalf("读取解压文件失败: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("解压内容不一致: got %q want %q", got, content)
	}
	gotSub, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("读取解压子目录文件失败: %v", err)
	}
	if string(gotSub) != string(content) {
		t.Fatalf("解压子目录内容不一致: got %q want %q", gotSub, content)
	}
}

// TestUnZip_ZipSlip 验证包含 ../ 的恶意 zip 条目被拦截，不外泄到 dst 之外。
func TestUnZip_ZipSlip(t *testing.T) {
	dst := t.TempDir()
	zipFile := filepath.Join(t.TempDir(), "evil.zip")

	buf, err := createEvilZip()
	if err != nil {
		t.Fatalf("构造恶意 zip 失败: %v", err)
	}
	if err := os.WriteFile(zipFile, buf, 0644); err != nil {
		t.Fatalf("写恶意 zip 失败: %v", err)
	}

	if err := UnZip(dst, zipFile); err != nil {
		t.Fatalf("UnZip 不应返回错误: %v", err)
	}

	escape := filepath.Join(filepath.Dir(dst), "escaped.txt")
	if _, err := os.Stat(escape); err == nil {
		t.Fatalf("zip slip 漏洞：文件逃逸到了 dst 之外: %s", escape)
	}
	if _, err := os.Stat(filepath.Join(dst, "escaped.txt")); err == nil {
		t.Fatalf("恶意条目不应被解压到 dst 内")
	}
}

// createEvilZip 构造含 ../escaped.txt 条名的恶意 zip 字节。
func createEvilZip() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../escaped.txt")
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
