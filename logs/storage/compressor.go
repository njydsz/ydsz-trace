// Package storage 提供 V2.1 列式段存储引擎的底层能力。
//
// 包含数据压缩/解压、段的读写与生命周期管理。
package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// compressorType 标识当前使用的压缩算法，便于排查与日志输出。
const compressorType = "gzip"

// Compress 使用 gzip 压缩原始列式数据。
//
// 当 go.mod 引入 github.com/klauspost/compress/zstd 后，可将内部实现切换为 zstd.EncodeAll，
// 对外签名保持不变，无需调整调用方。
//
// 参数 data 为原始字节，返回压缩后的字节切片。
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("compressor: create gzip writer failed: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("compressor: gzip write failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compressor: gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress 解压由 Compress 产出的 gzip 压缩数据。
//
// 参数 data 为压缩后的字节，返回原始数据。
// 传入 nil 或空切片时直接返回 nil, nil。
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("compressor: create gzip reader failed: %w", err)
	}

	result, err := io.ReadAll(reader)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("compressor: gzip read failed: %w", err)
	}

	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("compressor: gzip reader close failed: %w", err)
	}

	return result, nil
}
