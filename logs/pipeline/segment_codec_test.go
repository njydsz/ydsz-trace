package pipeline

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestEncodeSegmentColumns_Layout 验证列式编码的头部布局与偏移回填正确。
func TestEncodeSegmentColumns_Layout(t *testing.T) {
	ts := []string{"2026-11-03 10:00:00", "2026-11-03 10:00:01"}
	lv := []string{"ERROR", "WARN"}
	msg := []string{"boom", "careful"}

	blob := encodeSegmentColumns(ts, lv, msg)

	if len(blob) < 33 {
		t.Fatalf("blob too short: %d", len(blob))
	}
	if !bytes.HasPrefix(blob, []byte("YDSG")) {
		t.Fatalf("magic mismatch: %q", blob[:4])
	}
	if blob[4] != segmentFormatVersion {
		t.Fatalf("version mismatch: got %d want %d", blob[4], segmentFormatVersion)
	}

	rowCount := binary.BigEndian.Uint32(blob[5:9])
	if int(rowCount) != len(msg) {
		t.Fatalf("rowCount mismatch: got %d want %d", rowCount, len(msg))
	}

	tsCount := binary.BigEndian.Uint32(blob[9:13])
	tsOff := binary.BigEndian.Uint32(blob[13:17])
	lvCount := binary.BigEndian.Uint32(blob[17:21])
	lvOff := binary.BigEndian.Uint32(blob[21:25])
	msgCount := binary.BigEndian.Uint32(blob[25:29])
	msgOff := binary.BigEndian.Uint32(blob[29:33])

	if int(tsCount) != len(ts) {
		t.Errorf("tsCount = %d want %d", tsCount, len(ts))
	}
	if int(lvCount) != len(lv) {
		t.Errorf("lvCount = %d want %d", lvCount, len(lv))
	}
	if int(msgCount) != len(msg) {
		t.Errorf("msgCount = %d want %d", msgCount, len(msg))
	}

	// 偏移必须单调且不重叠。
	if tsOff != 33 {
		t.Errorf("tsOff = %d want 33", tsOff)
	}
	if lvOff < tsOff+uint32(columnLen(ts)) {
		t.Errorf("lvOff %d overlaps ts column end %d", lvOff, tsOff+uint32(columnLen(ts)))
	}
	if msgOff < lvOff+uint32(columnLen(lv)) {
		t.Errorf("msgOff %d overlaps lv column end %d", msgOff, lvOff+uint32(columnLen(lv)))
	}
	if int(msgOff)+columnLen(msg) != len(blob) {
		t.Errorf("message column does not end at blob tail: off=%d len=%d blob=%d",
			msgOff, columnLen(msg), len(blob))
	}
}

// TestEncodeSegmentColumns_Empty 验证空列不 panic 且仍产出合法头部。
func TestEncodeSegmentColumns_Empty(t *testing.T) {
	blob := encodeSegmentColumns(nil, nil, nil)
	if len(blob) != 33 {
		t.Fatalf("empty blob len = %d want 33", len(blob))
	}
	if !bytes.HasPrefix(blob, []byte("YDSG")) {
		t.Fatal("magic missing")
	}
}

// columnLen 计算单列编码后的字节数，用于偏移断言。
func columnLen(values []string) int {
	n := 0
	for _, v := range values {
		n += 4 + len(v)
	}
	return n
}

// TestSegmentHour 验证小时提取的常见格式与降级。
func TestSegmentHour(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2026-11-03 10:00:00", "10"},
		{"2026-11-03T10:00:00", "10"},
		{"2026-11-03 00:00:00", "00"},
		{"2026-11-03 23:59:59", "23"},
		{"", "00"},
		{"short", "00"},
		{"2026-11-03X10:00:00", "00"},
	}
	for _, c := range cases {
		if got := segmentHour(c.in); got != c.want {
			t.Errorf("segmentHour(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

// TestEncodeStringColumn_RoundTripShape 验证列编码的长度前缀可逐条解析。
func TestEncodeStringColumn_RoundTripShape(t *testing.T) {
	values := []string{"", "a", "hello world", "中文测试"}
	blob := encodeStringColumn(values)

	off := 0
	for i, want := range values {
		if off+4 > len(blob) {
			t.Fatalf("row %d: truncated length prefix", i)
		}
		l := int(binary.BigEndian.Uint32(blob[off : off+4]))
		off += 4
		if off+l > len(blob) {
			t.Fatalf("row %d: truncated payload", i)
		}
		got := string(blob[off : off+l])
		if got != want {
			t.Errorf("row %d = %q want %q", i, got, want)
		}
		off += l
	}
	if off != len(blob) {
		t.Errorf("trailing bytes: consumed %d of %d", off, len(blob))
	}
}
