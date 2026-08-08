// Package file 提供日志文件搜索与下载能力。
//
// 关键安全措施：
//   - key 参数白名单校验（仅字母/数字/连字符/下划线）
//   - path 参数禁止包含 ".."
//   - 所有文件拼接使用 filepath.Join
package file

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// FileReq 文件查询请求体。
type FileReq struct {
	// Path 日志文件的绝对路径
	Path string `json:"path"`
	// Key 搜索关键字（同时用于生成临时文件名）
	Key string `json:"key"`
	// Line 命中行后追加读取的上下文行数
	Line int64 `json:"line"`
}

// safeKeyPattern 合法 key 白名单：字母、数字、连字符、下划线，长度 1-128。
var safeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,128}$`)

// sanitizeKey 校验 key 参数，防止路径遍历与恶意输入。
//
// 返回清洗后的 key 与是否合法。
func sanitizeKey(key string) (string, bool) {
	if len(key) == 0 || len(key) > 128 {
		return "", false
	}
	if !safeKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

// Query 处理日志文件查询请求：搜索 → 过滤上下文 → zip 压缩 → 下载。
//
// 响应：application/octet-stream（zip 文件）。
// 安全措施：调用 sanitizeKey 与 path ".." 校验，参数非法直接返回 400。
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	var fileReq FileReq
	data, err := c.GetRawData()
	if err != nil {
		log.Printf("读取请求体失败: %v", err)
		c.Status(400)
		return
	}
	err = json.Unmarshal(data, &fileReq)
	if err != nil {
		log.Printf("JSON解析失败: %v", err)
		c.Status(400)
		return
	}

	// 安全校验：清理 key 参数，防止路径遍历
	safeKey, ok := sanitizeKey(fileReq.Key)
	if !ok {
		log.Printf("非法key参数: %s", fileReq.Key)
		c.Status(400)
		return
	}

	// 安全校验：path 不允许包含路径遍历字符
	if strings.Contains(fileReq.Path, "..") {
		log.Printf("非法path参数(包含..): %s", fileReq.Path)
		c.Status(400)
		return
	}

	result := ReadString(fileReq.Path, safeKey, fileReq.Line, cfg.StringOr("temppath", "./temp/logc/"))
	defer func() {
		os.Remove(result)
	}()

	if result == "" {
		c.Status(404)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filepath.Base(result)+`"`)
	c.File(result)
}

// ReadString 按关键字搜索日志文件并输出到临时文件，再压缩为 zip 返回路径。
//
// 行为：
//   - 每次命中关键字所在行及其后 line 行写入结果
//   - 若后续命中在当前上下文窗口内，则扩展窗口
//   - 结果文件位于 temppath/key.log
//
// 返回 zip 文件路径；失败返回空字符串。
func ReadString(filename string, key string, line int64, temppath string) (file string) {
	startTime := time.Now()
	defer func(startTime time.Time) {
		log.Printf("共耗时：%s\n", time.Since(startTime))
	}(startTime)

	f, err := os.Open(filename)
	if err != nil {
		log.Printf("打开文件失败: %v", err)
		return ""
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var lineBegin int64 = 0
	var lineFirst int64 = 0
	var lineOver int64 = 0

	// 确保临时目录存在
	if err := os.MkdirAll(temppath, 0755); err != nil {
		log.Printf("创建临时目录失败: %v", err)
		return ""
	}

	// 安全：使用 filepath.Join 拼接路径
	safeFilename := filepath.Join(temppath, key+".log")
	dstFile, err := os.OpenFile(safeFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("创建临时文件失败: %v", err)
		return ""
	}

	bufWriter := bufio.NewWriter(dstFile)

	// 首次遍历记录出现行号
	for {
		str, err := r.ReadString('\n')
		lineBegin++
		if err != nil {
			break
		}
		if strings.Contains(str, key) {
			if lineFirst == 0 && lineOver == 0 {
				lineFirst = lineBegin
				lineOver = lineFirst + line
				log.Printf("lineFirst:%d\n", lineFirst)
				log.Printf("lineBegin:%d\n", lineBegin)
				log.Printf("lineOver:%d\n", lineOver)
			}
		}
		if lineBegin < lineOver {
			bufWriter.WriteString(str)
		}
		if lineBegin == lineOver {
			bufWriter.WriteString(str)
			bufWriter.WriteString("\n")
		}
		if lineOver > 0 && lineBegin > lineOver {
			if strings.Contains(str, key) {
				if lineFirst != 0 && lineOver != 0 {
					lineFirst = lineBegin
					lineOver = lineFirst + line
					log.Printf("lineFirst:%d\n", lineFirst)
					log.Printf("lineBegin:%d\n", lineBegin)
					log.Printf("lineOver:%d\n", lineOver)
				}
			}
		}
	}
	bufWriter.Flush()
	dstFile.Close()

	log.Printf("查找耗时：%s\n", time.Since(startTime))

	// 压缩
	startTimeThree := time.Now()
	ip := GetLocalIPv4()
	dst := filepath.Join(temppath, ip+".zip")
	if err := Zip(dst, safeFilename); err != nil {
		log.Printf("压缩失败: %v", err)
		return ""
	}
	log.Printf("压缩耗时：%s\n", time.Since(startTimeThree))
	return dst
}

// GetLocalIPv4 返回本机第一个非 loopback 的 IPv4 地址。
//
// 未找到时返回 "unknown"。
func GetLocalIPv4() (ip string) {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		log.Println("net.Interfaces failed, err:", err.Error())
		return "unknown"
	}

	for i := 0; i < len(netInterfaces); i++ {
		if (netInterfaces[i].Flags & net.FlagUp) != 0 {
			addrs, _ := netInterfaces[i].Addrs()
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return ipnet.IP.String()
					}
				}
			}
		}
	}
	return "unknown"
}

// Zip 将 src 文件/目录压缩为 dst zip 文件；压缩成功后删除 src。
//
// 注意：与 pkg/util.Zip 重复，建议后续收敛到统一实现。
func Zip(dst, src string) (err error) {
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
		os.Remove(src)
	}()

	return filepath.Walk(src, func(path string, fi os.FileInfo, errBack error) (err error) {
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
		log.Printf("成功压缩文件：%s, 共写入了 %d 个字符的数据\n", path, n)
		return nil
	})
}

// UnZip 将 src zip 解压到 dst 目录。
//
// 注意：当前实现缺少 zip slip 路径穿越校验，与 pkg/util.UnZip 存在安全差异。
//       生产使用推荐统一为 pkg/util.UnZip。
func UnZip(dst, src string) (err error) {
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
		log.Printf("成功解压 %s，共写入了 %d 个字符的数据\n", path, n)

		fw.Close()
		fr.Close()
	}
	return nil
}
