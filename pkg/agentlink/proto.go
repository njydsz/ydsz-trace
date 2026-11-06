// Package agentlink 定义 logc ↔ logs 长连接通道的帧协议。
//
// 设计目标（P1：对标 frp 的控制面/数据面复用）：
//   - 单条 WebSocket 长连接上同时承载控制流与数据流
//   - 帧格式：[type:1B][flags:1B][streamID:4B][length:4B][payload:N]
//   - type 区分控制帧与数据帧；streamID 用于多路复用（0 = 控制流）
//
// 为什么用 WebSocket 而非裸 TCP：
//  1. 复用现有 HTTP 端口与 TLS/mTLS 基础设施，无需额外监听端口
//  2. 天然穿透企业代理与防火墙（443/HTTPS）
//  3. 与现有 Gin 路由集成简单
package agentlink

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// FrameType 帧类型。
type FrameType byte

const (
	// FrameTypeControl 控制帧：心跳、注册、配置下发、ACK。
	FrameTypeControl FrameType = 0x01
	// FrameTypeData 数据帧：日志批量上报、查询下推结果。
	FrameTypeData FrameType = 0x02
	// FrameTypeClose 关闭帧：优雅断开，携带原因。
	FrameTypeClose FrameType = 0x03
)

// Frame flags 位。
const (
	// FlagCompressed payload 经 gzip 压缩。
	FlagCompressed byte = 0x01
	// FlagAck 该帧为对端帧的确认。
	FlagAck byte = 0x02
)

// ControlCode 控制帧的子命令。
type ControlCode byte

const (
	// ControlPing 客户端心跳（服务端回 ControlPong）。
	ControlPing ControlCode = 0x01
	// ControlPong 服务端心跳应答。
	ControlPong ControlCode = 0x02
	// ControlRegister 客户端注册（首次连接握手）。
	ControlRegister ControlCode = 0x03
	// ControlRegisterOK 服务端注册成功应答。
	ControlRegisterOK ControlCode = 0x04
	// ControlConfigPush 服务端下发配置变更。
	ControlConfigPush ControlCode = 0x05
	// ControlModeSwitch 服务端下发长连接模式切换（P1-5 灰度）。
	ControlModeSwitch ControlCode = 0x06
	// ControlLogBatch 客户端上报日志批次（数据面复用）。
	ControlLogBatch ControlCode = 0x10
	// ControlLogACK 服务端确认日志批次（携带批次号）。
	ControlLogACK ControlCode = 0x11
)

// HeaderSize 帧头长度：type(1) + flags(1) + streamID(4) + length(4) = 10 字节。
const HeaderSize = 10

// MaxPayloadSize 单帧 payload 上限（16 MiB），防止内存攻击。
const MaxPayloadSize = 16 * 1024 * 1024

// Frame 长连接帧。
type Frame struct {
	Type     FrameType
	Flags    byte
	StreamID uint32
	Payload  []byte
}

// Encode 将帧编码为字节流。
func (f *Frame) Encode() ([]byte, error) {
	if len(f.Payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload 过大: %d > %d", len(f.Payload), MaxPayloadSize)
	}
	buf := make([]byte, HeaderSize+len(f.Payload))
	buf[0] = byte(f.Type)
	buf[1] = f.Flags
	binary.BigEndian.PutUint32(buf[2:6], f.StreamID)
	binary.BigEndian.PutUint32(buf[6:10], uint32(len(f.Payload)))
	copy(buf[HeaderSize:], f.Payload)
	return buf, nil
}

// DecodeFrame 从字节流解析帧头，返回帧与消耗的字节数。
//
// 调用方需保证 buf 至少包含 HeaderSize 字节；payload 由调用方按需读取。
func DecodeFrame(buf []byte) (*Frame, int, error) {
	if len(buf) < HeaderSize {
		return nil, 0, errors.New("缓冲区不足以解析帧头")
	}
	length := binary.BigEndian.Uint32(buf[6:10])
	if length > MaxPayloadSize {
		return nil, 0, fmt.Errorf("帧 payload 过大: %d", length)
	}
	total := HeaderSize + int(length)
	if len(buf) < total {
		return nil, 0, errors.New("缓冲区不足以解析完整帧")
	}
	f := &Frame{
		Type:     FrameType(buf[0]),
		Flags:    buf[1],
		StreamID: binary.BigEndian.Uint32(buf[2:6]),
		Payload:  buf[HeaderSize:total],
	}
	return f, total, nil
}

// ControlPayload 构造控制帧 payload：code(1B) + body。
func ControlPayload(code ControlCode, body []byte) []byte {
	out := make([]byte, 1+len(body))
	out[0] = byte(code)
	copy(out[1:], body)
	return out
}

// ParseControlCode 从控制帧 payload 解析子命令。
func ParseControlCode(payload []byte) (ControlCode, []byte, error) {
	if len(payload) < 1 {
		return 0, nil, errors.New("控制帧 payload 为空")
	}
	return ControlCode(payload[0]), payload[1:], nil
}

// IsCompressed 判断帧是否标记为压缩。
func (f *Frame) IsCompressed() bool {
	return f.Flags&FlagCompressed != 0
}

// IsAck 判断帧是否为确认帧。
func (f *Frame) IsAck() bool {
	return f.Flags&FlagAck != 0
}

// LogBatch 一批待上报日志（P1-3 数据面载荷）。
//
// Lines 为原始日志行数组；服务端按 ClientID + ItemID 落库。
type LogBatch struct {
	// BatchNo 批次号（客户端生成，服务端 ACK 回传，用于幂等去重）。
	BatchNo string `json:"batchNo"`
	// ClientID 客户端 ID（注册时由服务端分配）。
	ClientID int64 `json:"clientId"`
	// ItemID 日志项 ID。
	ItemID int64 `json:"itemId"`
	// Lines 日志行。
	Lines []string `json:"lines"`
	// Timestamp 批次生成时间。
	Timestamp int64 `json:"timestamp"`
}

// ConfigPush 配置下发载荷（P1-4）。
type ConfigPush struct {
	// Version 配置版本号（单调递增，客户端可据此去重）。
	Version int64 `json:"version"`
	// SamplingRate 采样率（0.0~1.0，1.0 表示全量）。
	SamplingRate float64 `json:"samplingRate,omitempty"`
	// MaskTemplates 脱敏模板 ID 列表。
	MaskTemplates []string `json:"maskTemplates,omitempty"`
	// MultilinePatterns 多行合并规则。
	MultilinePatterns []string `json:"multilinePatterns,omitempty"`
	// EffectiveAt 生效时间（Unix 秒）。
	EffectiveAt int64 `json:"effectiveAt"`
}

// ItemSpec 下发给 agent 的日志项配置（P1-3）。
//
// agent 依据该配置在本地 tail 对应文件，增量读取后经长连接上报。
// 字段与 models.TItem 对齐，但仅暴露 agent 必需的采集信息。
type ItemSpec struct {
	// ItemID 日志项 ID（上报时回传，服务端据此落库）。
	ItemID int64 `json:"itemId"`
	// ItemName 日志项名称（用于本地日志与去重）。
	ItemName string `json:"itemName"`
	// LogPath 日志目录。
	LogPath string `json:"logPath"`
	// LogPrefix 文件名前缀。
	LogPrefix string `json:"logPrefix"`
	// LogSuffix 文件名后缀。
	LogSuffix string `json:"logSuffix"`
	// Status 启用状态（"1"=启用）。
	Status string `json:"status"`
}

// RegisterAccept 注册成功应答载荷（P1-3）。
//
// 服务端在 RegisterOK 中回传该 client 下全部启用的日志项，
// agent 据此启动本地 tail 扫描，实现"控制面带数据面配置"。
type RegisterAccept struct {
	// ClientID 服务端分配的客户端 ID。
	ClientID int64 `json:"clientId"`
	// ServerTime 服务端时间（Unix 秒）。
	ServerTime int64 `json:"serverTime"`
	// Items 该客户端下启用的日志项列表。
	Items []ItemSpec `json:"items"`
	// AgentMode 长连接模式（P1-5）：push / pull，空为階置默认。
	AgentMode string `json:"agentMode,omitempty"`
}
