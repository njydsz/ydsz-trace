package util

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	key := "YDSZ_TEST_GETENV"
	os.Unsetenv(key)
	if got := GetEnv(key, "default"); got != "default" {
		t.Fatalf("未设置时应返回默认: got %q", got)
	}
	t.Setenv(key, "value")
	if got := GetEnv(key, "default"); got != "value" {
		t.Fatalf("已设置时应返回值: got %q", got)
	}
}
