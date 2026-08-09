package source

import (
	"testing"
)

func TestCompileQuery_Basic(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		line    string
		want    bool
		wantErr bool
	}{
		{"empty query returns nil fn", "", "anything", true, false},

		{"single term match", "ERROR", "2024-01-02 10:00:00 ERROR happened", true, false},
		{"single term not match", "ERROR", "2024-01-02 10:00:00 INFO happened", false, false},

		{"implicit AND matches both", "ERROR timeout", "ERROR: connection timeout", true, false},
		{"implicit AND missing one", "ERROR timeout", "ERROR: connection failed", false, false},

		{"explicit AND matches", "ERROR AND timeout", "ERROR: connection timeout", true, false},
		{"explicit AND missing", "ERROR AND timeout", "ERROR: no problem", false, false},

		{"explicit OR left matches", "ERROR OR timeout", "ERROR happened", true, false},
		{"explicit OR right matches", "ERROR OR timeout", "timeout happened", true, false},
		{"explicit OR neither matches", "ERROR OR timeout", "INFO ok", false, false},

		{"NOT inverts", "ERROR AND NOT DEBUG", "ERROR happened", true, false},
		{"NOT filters", "ERROR AND NOT timeout", "ERROR: connection timeout", false, false},

		{"field level match", "level:ERROR", "2024-01-02 10:00:00 ERROR happened", true, false},
		{"field level mismatch", "level:ERROR", "2024-01-02 10:00:00 INFO happened", false, false},
		{"field level case insensitive", "level:error", "ERROR happened", true, false},

		{"field traceId match", "traceId:abc123", "trace_id=abc123 error happened", true, false},
		{"field traceId mismatch", "traceId:abc123", "trace_id=xyz happened", false, false},

		{"field module match", "module:auth", "auth_service login failed", true, false},
		{"field module mismatch", "module:auth", "order_service checkout done", false, false},

		{"quoted phrase", `"connection timeout"`, "ERROR: connection timeout", true, false},
		{"quoted phrase mismatch", `"connection timeout"`, "ERROR: timeout occurred", false, false},

		{"parenthesized OR with AND", "(ERROR OR FATAL) AND module:auth", "ERROR auth login", true, false},
		{"parenthesized OR with AND module mismatch", "(ERROR OR FATAL) AND module:auth", "ERROR order done", false, false},

		{"double NOT", "NOT NOT ERROR", "ERROR happened", true, false},

		{"unbalanced parenthesis returns error", "(ERROR OR timeout", "anything", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := CompileQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CompileQuery(%q) error = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			got := fn == nil || fn(tt.line)
			if got != tt.want {
				t.Errorf("query %q on line %q = %v, want %v", tt.query, tt.line, got, tt.want)
			}
		})
	}
}

func TestLineFilterWithQuery(t *testing.T) {
	tests := []struct {
		name  string
		cfg   ScanConfig
		lines []string
		want  []string
	}{
		{
			name:  "query only",
			cfg:   ScanConfig{Query: "ERROR AND timeout"},
			lines: []string{"INFO normal", "ERROR timeout happened", "ERROR alone"},
			want:  []string{"ERROR timeout happened"},
		},
		{
			name:  "query AND key intersection",
			cfg:   ScanConfig{Key: "timeout", Query: "level:ERROR"},
			lines: []string{"ERROR timeout happened", "INFO timeout happened", "ERROR happened"},
			want:  []string{"ERROR timeout happened"},
		},
		{
			name:  "empty query falls back to key-only",
			cfg:   ScanConfig{Key: "timeout"},
			lines: []string{"timeout happened", "no match"},
			want:  []string{"timeout happened"},
		},
		{
			name:  "field level filter query",
			cfg:   ScanConfig{Query: "module:auth AND (ERROR OR FATAL)"},
			lines: []string{"ERROR auth module failed", "ERROR order ok", "FATAL auth panic", "INFO auth login"},
			want:  []string{"ERROR auth module failed", "FATAL auth panic"},
		},
		{
			name:  "NOT in query",
			cfg:   ScanConfig{Query: "timeout AND NOT module:healthcheck"},
			lines: []string{"timeout in healthcheck probe", "timeout in order service"},
			want:  []string{"timeout in order service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := newLineFilter(tt.cfg)
			if err != nil {
				t.Fatalf("newLineFilter error: %v", err)
			}
			var got []string
			for _, line := range tt.lines {
				if f.shouldKeep(line) {
					got = append(got, line)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v (items: %d), want %v (items: %d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMustCompileQuery_Invalid(t *testing.T) {
	fn := MustCompileQuery("(ERROR AND")
	if fn == nil {
		t.Fatal("MustCompileQuery should not return nil")
	}
	if fn("ERROR anything") {
		t.Error("invalid query should return false, got true")
	}
}

func TestIsValidQuery(t *testing.T) {
	tests := []struct {
		query string
		valid bool
	}{
		{"ERROR AND timeout", true},
		{"(ERROR OR timeout)", true},
		{"timeout", true},
		{"level:ERROR", true},
		{"", true},
		{"(AND error", false},
	}
	for _, tt := range tests {
		err := IsValidQuery(tt.query)
		if (err == nil) != tt.valid {
			t.Errorf("IsValidQuery(%q) err=%v, valid=%v", tt.query, err, tt.valid)
		}
	}
}
