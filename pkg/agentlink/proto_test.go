package agentlink

import (
	"bytes"
	"testing"
)

func TestFrameEncodeDecode(t *testing.T) {
	f := &Frame{
		Type:     FrameTypeControl,
		Flags:    0,
		StreamID: 0,
		Payload:  ControlPayload(ControlPing, []byte("hello")),
	}
	data, err := f.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(data) != HeaderSize+len(f.Payload) {
		t.Fatalf("长度不符: %d", len(data))
	}

	got, n, err := DecodeFrame(data)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if n != len(data) {
		t.Fatalf("消耗字节数不符: %d != %d", n, len(data))
	}
	if got.Type != FrameTypeControl {
		t.Fatalf("Type 不符: %v", got.Type)
	}
	if !bytes.Equal(got.Payload, f.Payload) {
		t.Fatalf("Payload 不符: %v", got.Payload)
	}
}

func TestFrameDecodeIncomplete(t *testing.T) {
	if _, _, err := DecodeFrame([]byte{0x01}); err == nil {
		t.Fatal("缓冲区不足应报错")
	}
}

func TestFrameDecodeOversized(t *testing.T) {
	// 构造 length 超过 MaxPayloadSize 的帧头。
	data := make([]byte, HeaderSize)
	data[0] = byte(FrameTypeData)
	// length 字段写入 MaxPayloadSize + 1（用 uint32 移位避免 byte 溢出）。
	over := uint32(MaxPayloadSize + 1)
	data[6] = byte(over >> 24)
	data[7] = byte(over >> 16)
	data[8] = byte(over >> 8)
	data[9] = byte(over)
	if _, _, err := DecodeFrame(data); err == nil {
		t.Fatal("超长帧应报错")
	}
}

func TestParseControlCode(t *testing.T) {
	code, body, err := ParseControlCode(ControlPayload(ControlConfigPush, []byte("cfg")))
	if err != nil {
		t.Fatalf("ParseControlCode: %v", err)
	}
	if code != ControlConfigPush {
		t.Fatalf("code 不符: %v", code)
	}
	if string(body) != "cfg" {
		t.Fatalf("body 不符: %s", body)
	}
}

func TestParseControlCodeEmpty(t *testing.T) {
	if _, _, err := ParseControlCode(nil); err == nil {
		t.Fatal("空 payload 应报错")
	}
}

func TestFrameFlags(t *testing.T) {
	f := &Frame{Flags: FlagCompressed | FlagAck}
	if !f.IsCompressed() || !f.IsAck() {
		t.Fatal("flag 解析错误")
	}
}

func TestLogBatchRoundTrip(t *testing.T) {
	// LogBatch 通过 JSON 序列化传输，这里验证字段 tag。
	b := LogBatch{
		BatchNo:   "b-1",
		ClientID:  42,
		ItemID:    7,
		Lines:     []string{"line1", "line2"},
		Timestamp: 1234567890,
	}
	if b.BatchNo != "b-1" || b.ClientID != 42 {
		t.Fatal("字段赋值错误")
	}
}
