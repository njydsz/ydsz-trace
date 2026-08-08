package util

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Zip 压缩文件或目录到 zip 文件
func Zip(dst, src string) error {
	fw, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	defer func() {
		if err := zw.Close(); err != nil {
			log.Printf("关闭zip writer失败: %v", err)
		}
		// 压缩成功后删除原文件
		os.RemoveAll(src)
	}()

	return filepath.Walk(src, func(path string, fi os.FileInfo, errBack error) error {
		if errBack != nil {
			return errBack
		}

		fh, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}

		fh.Name = strings.TrimPrefix(path, string(filepath.Separator))

		if fi.IsDir() {
			fh.Name += "/"
		}

		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}

		if !fh.Mode().IsRegular() {
			return nil
		}

		fr, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fr.Close()

		n, err := io.Copy(w, fr)
		if err != nil {
			return err
		}
		log.Printf("成功压缩文件：%s, 共写入了 %d 个字符\n", path, n)
		return nil
	})
}

// UnZip 解压缩 zip 文件到目标目录
func UnZip(dst, src string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	if dst != "" {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
	}

	for _, file := range zr.File {
		path := filepath.Join(dst, file.Name)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, file.Mode()); err != nil {
				return err
			}
			continue
		}

		fr, err := file.Open()
		if err != nil {
			return err
		}

		fw, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, file.Mode())
		if err != nil {
			fr.Close()
			return err
		}

		n, err := io.Copy(fw, fr)
		if err != nil {
			fw.Close()
			fr.Close()
			return err
		}
		log.Printf("成功解压 %s，共写入了 %d 个字符\n", path, n)

		fw.Close()
		fr.Close()
	}
	return nil
}

// PathExists 检查路径是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// CreateDir 创建文件夹（递归）
func CreateDir(folderPath string) error {
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		return os.MkdirAll(folderPath, 0755)
	}
	return nil
}
