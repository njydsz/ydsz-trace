package util

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagZip(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0644)
	dst := filepath.Join(t.TempDir(), "o.zip")
	if err := Zip(dst, src); err != nil {
		t.Fatalf("Zip err: %v", err)
	}
	fi, _ := os.Stat(dst)
	t.Logf("zip size=%d", fi.Size())

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open reader err: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		t.Logf("entry: %q", f.Name)
	}

	dstDir := t.TempDir()
	if err := UnZip(dstDir, dst); err != nil {
		t.Fatalf("UnZip err: %v", err)
	}
	t.Logf("UnZip OK")
}
