package file

import "testing"

func TestNormalizeLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"WARN", "WARN"},
		{"warn", "WARN"},
		{"WARNING", "WARN"},
		{"warning", "WARN"},
		{"INFO", "INFO"},
		{"info", "INFO"},
		{"DEBUG", "DEBUG"},
		{"ERROR", "ERROR"},
		{"FATAL", "FATAL"},
		{"unknown", "UNKNOWN"},
	}
	for _, tt := range tests {
		got := normalizeLevel(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeLevel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseTimeToSeconds(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"00:00", 0},
		{"00:00:00", 0},
		{"01:00", 3600},
		{"12:30", 45000},
		{"12:30:45", 45045},
		{"23:59:59", 86399},
		{"", -1},
		{"invalid", -1},
		{"25:00", -1}, // invalid hour
		{"12:60", -1}, // invalid minute
	}
	for _, tt := range tests {
		got := parseTimeToSeconds(tt.input)
		if got != tt.expected {
			t.Errorf("parseTimeToSeconds(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestExtractTimeFromLogLine(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"2024-01-02 10:30:45 INFO some message", 37845},
		{"2024-01-02 10:30 INFO msg", 37800},
		{"2024/01/02 10:30:45 ERROR msg", 37845},
		{"10:30:45 DEBUG log", 37845},
		{"10:30 INFO log", 37800},
		{"no timestamp here", -1},
		{"", -1},
	}
	for _, tt := range tests {
		got := extractTimeFromLogLine(tt.line)
		if got != tt.expected {
			t.Errorf("extractTimeFromLogLine(%q) = %d, want %d", tt.line, got, tt.expected)
		}
	}
}

func TestMatchLogLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		key      string
		regex    bool
		level    string
		expected bool
	}{
		{"plain contains", "INFO something happened", "INFO", false, "", true},
		{"plain not contains", "INFO something happened", "ERROR", false, "", false},
		{"level filter match", "INFO something", "", false, "INFO", true},
		{"level filter no match", "INFO something", "", false, "ERROR", false},
		{"regex match", "Error: code 500", `code \d+`, true, "", true},
		{"regex no match", "Error: code abc", `code \d+`, true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchLogLine(tt.line, tt.key, tt.regex, nil, tt.level)
			if got != tt.expected {
				t.Errorf("matchLogLine() = %v, want %v", got, tt.expected)
			}
		})
	}
}
