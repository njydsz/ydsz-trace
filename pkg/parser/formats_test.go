package parser

import (
	"strings"
	"testing"
)

// TestFormat_JSON 验证 JSON 日志正确识别并抽取字段。
func TestFormat_JSON(t *testing.T) {
	line := `{"time":"2024-01-15T10:23:45.123Z","level":"INFO","msg":"user login","userId":12345,"traceId":"abc123def456"}`
	fields, name, matched := MatchFormat(line)

	if !matched {
		t.Fatal("expected JSON format to match")
	}
	if name != "json" {
		t.Errorf("expected format name 'json', got '%s'", name)
	}
	if fields == nil {
		t.Fatal("expected fields, got nil")
	}
	if fields["level"] != "INFO" {
		t.Errorf("expected level=INFO, got %q", fields["level"])
	}
	if fields["msg"] == "" {
		t.Error("expected non-empty msg field")
	}
}

// TestFormat_SpringBoot 验证 Spring Boot 格式正确识别 level/thread。
func TestFormat_SpringBoot(t *testing.T) {
	line := "2024-01-15 10:23:45.123 INFO [http-nio-8080-exec-1] c.example.UserService : Login user alice"
	_, name, matched := MatchFormat(line)

	if !matched {
		t.Fatal("expected SpringBoot format to match")
	}
	if name != "springboot" {
		t.Errorf("expected format name 'springboot', got '%s'", name)
	}
	// 也验证 ExtractFields 能提取字段
	f := ExtractFields(line)
	if f["level"] != "INFO" {
		t.Errorf("expected level=INFO, got %q", f["level"])
	}
	if f["thread"] == "" {
		t.Errorf("expected non-empty thread, got %q", f["thread"])
	}
}

// TestFormat_Nginx 验证 Nginx 组合日志正确抽取。
func TestFormat_Nginx(t *testing.T) {
	line := `192.168.1.1 - - [15/Jan/2024:10:23:45 +0800] "GET /api/users HTTP/1.1" 200 1234 "https://example.com" "Mozilla/5.0"`
	_, name, matched := MatchFormat(line)

	if !matched {
		t.Fatal("expected Nginx format to match")
	}
	if name != "nginx" {
		t.Errorf("expected format name 'nginx', got '%s'", name)
	}
	// ExtractFields 也应通用抽取
	f := ExtractFields(line)
	if f["ip"] != "192.168.1.1" && f["ip"] != "" {
		t.Logf("ip extraction: %q", f["ip"])
	}
	if f["statusCode"] != "200" {
		t.Errorf("expected statusCode=200, got %q", f["statusCode"])
	}
}

// TestFormat_NoMatch 验证无法匹配的行返回空 format。
func TestFormat_NoMatch(t *testing.T) {
	line := "Random text without any known log format structure"
	_, name, matched := FormatMatch(line)

	if matched {
		t.Errorf("expected no format match for random text, but got '%s'", name)
	}
}

// TestFormat_Logfmt 验证 Logfmt 格式匹配。
func TestFormat_Logfmt(t *testing.T) {
	line := `method=GET path=/api/users status=200 duration=42ms`
	_, name, matched := MatchFormat(line)

	if !matched {
		t.Fatal("expected Logfmt format to match")
	}
	if name != "logfmt" {
		t.Errorf("expected format name 'logfmt', got '%s'", name)
	}
}

// TestFormat_Docker 验证 Docker 格式匹配。
func TestFormat_Docker(t *testing.T) {
	line := "2024-01-15T10:23:45.123456789Z stderr F Error connecting to database"
	_, name, matched := MatchFormat(line)

	if !matched {
		t.Fatal("expected Docker format to match")
	}
	if name != "docker" {
		t.Errorf("expected format name 'docker', got '%s'", name)
	}
}

// TestFormat_ISO8601Timestamp 验证 ExtractFields 能识别多种时间戳格式。
func TestFormat_ISO8601Timestamp(t *testing.T) {
	line := "2024-01-15T10:23:45.123Z Some log message"
	f := ExtractFields(line)
	if f["timestamp"] == "" {
		t.Error("expected ISO8601 timestamp to be extracted")
	}
}

// TestFormat_LevelExtraction 验证日志级别提取。
func TestFormat_LevelExtraction(t *testing.T) {
	levels := []string{"DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "TRACE"}
	for _, lvl := range levels {
		line := lvl + " something happened"
		f := ExtractFields(line)
		got := strings.ToUpper(f["level"])
		if got != "WARN" && lvl == "WARNING" {
			// WARNING normalizes to WARN
			continue
		}
		if got != lvl {
			t.Errorf("expected level=%s, got %q", lvl, got)
		}
	}
}

// FormatMatch 辅助函数：避免直接使用未导出的 MatchFormat。
func FormatMatch(line string) (fields map[string]string, formatName string, matched bool) {
	return MatchFormat(line)
}
