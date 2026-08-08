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

	"github.com/astaxie/beego"
)

// FileController 文件查询控制器
type FileController struct {
	beego.Controller
}

// FileReq 文件查询请求
type FileReq struct {
	Path string `json:"path"`
	Key  string `json:"key"`
	Line int64  `json:"line"`
}

// safeKeyPattern 安全的 key：只允许字母、数字、连字符和下划线
var safeKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,128}$`)

// sanitizeKey 清洗 key 参数，防止路径遍历攻击
// 只允许字母、数字、连字符、下划线，长度限制 1-128
func sanitizeKey(key string) (string, bool) {
	if len(key) == 0 || len(key) > 128 {
		return "", false
	}
	if !safeKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
}

// Query 查询日志文件并返回压缩包下载
func (this *FileController) Query() {
	var file FileReq
	data := this.Ctx.Input.RequestBody
	err := json.Unmarshal(data, &file)
	if err != nil {
		log.Printf("JSON解析失败: %v", err)
		this.Ctx.ResponseWriter.WriteHeader(400)
		return
	}

	// 安全校验：清理 key 参数，防止路径遍历
	safeKey, ok := sanitizeKey(file.Key)
	if !ok {
		log.Printf("非法key参数: %s", file.Key)
		this.Ctx.ResponseWriter.WriteHeader(400)
		return
	}

	// 安全校验：path 不允许包含路径遍历字符
	if strings.Contains(file.Path, "..") {
		log.Printf("非法path参数(包含..): %s", file.Path)
		this.Ctx.ResponseWriter.WriteHeader(400)
		return
	}

	result := ReadString(file.Path, safeKey, file.Line)
	defer func() {
		os.Remove(result)
	}()
	this.Ctx.Output.Download(result)
}

// ReadString 按关键字搜索日志文件，返回匹配上下文行并压缩
func ReadString(filename string, key string, line int64) (file string) {
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

	// 获取临时目录
	temppath := beego.AppConfig.String("temppath")

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

// GetLocalIPv4 获取本机的IPv4地址
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

// Zip 压缩文件
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

// UnZip 解压缩
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
