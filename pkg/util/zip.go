package util

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Zip 将 src（文件或目录）压缩为 dst 指向的 zip 文件。
//
// 注意：
//   - 压缩成功后会自动删除原文件/目录
//   - 使用 defer 确保 zip.Writer.Close 仅调用一次，避免中央目录损坏
//   - 仅写入相对路径，避免绝对路径写入 zip 条目
func Zip(dst, src string) error {
	fw, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	// 注意：zw.Close() 必须且只能调用一次，否则会重复写中央目录导致 zip 损坏
	walkErr := func() error {
		defer func() {
			if cerr := zw.Close(); cerr != nil {
				log.Printf("关闭zip writer失败: %v", cerr)
			}
		}()
		return filepath.Walk(src, func(path string, fi os.FileInfo, errBack error) error {
			if errBack != nil {
				return errBack
			}

			fh, hdrErr := zip.FileInfoHeader(fi)
			if hdrErr != nil {
				return hdrErr
			}

			// 仅写入相对路径，避免把绝对路径（含盘符）写进 zip 条目
			rel, relErr := filepath.Rel(src, path)
			if relErr != nil {
				return relErr
			}
			fh.Name = filepath.ToSlash(rel)
			if fi.IsDir() {
				fh.Name += "/"
			}

			w, createErr := zw.CreateHeader(fh)
			if createErr != nil {
				return createErr
			}

			if !fh.Mode().IsRegular() {
				return nil
			}

			fr, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			defer fr.Close()

			if _, copyErr := io.Copy(w, fr); copyErr != nil {
				return copyErr
			}
			return nil
		})
	}()
	if walkErr != nil {
		return walkErr
	}

	// 压缩成功后删除原文件
	if err := os.RemoveAll(src); err != nil {
		log.Printf("删除原文件失败: %v", err)
	}
	return nil
}

// UnZip 将 src zip 文件解压到 dst 目录。
//
// 安全：
//   - 校验条目路径始终位于 dst 内，防止 zip 路径穿越（zip slip）
//   - 目录条目会递归创建，普通文件按 entry 权限还原
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
		// 防止 zip slip 路径穿越：确保解压目标始终在 dst 之内
		path := filepath.Join(dst, file.Name)
		if dst != "" {
			cleanDst := filepath.Clean(dst)
			if path != cleanDst && !strings.HasPrefix(path, cleanDst+string(filepath.Separator)) {
				log.Printf("跳过非法路径条目（疑似 zip slip）: %s", file.Name)
				continue
			}
		}

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

// PathExists 检查路径是否存在。
//
// 返回：(存在?, 错误)。错误仅反映权限/IO 问题，不存在视为 (false, nil)。
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

// CreateDir 递归创建目录（若已存在则不操作）。
func CreateDir(folderPath string) error {
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		return os.MkdirAll(folderPath, 0755)
	}
	return nil
}
