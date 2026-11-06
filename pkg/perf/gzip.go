package perf

import (
	"bytes"
	"compress/gzip"
)

// gzipBytes 压缩字节切片为 gzip 格式。
//
// 用于日志冷数据归档（V2.4-04），压缩级别取默认值，
// 在压缩率与 CPU 开销间取平衡。
func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// 写入失败仅忽略：索引/压缩是增强能力，不应影响主流程
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}
