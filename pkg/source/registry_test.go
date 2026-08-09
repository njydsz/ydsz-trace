package source

import (
	"context"
	"io"
	"testing"
)

// TestRegistry_FileRegistered 验证 FileSource 工厂已注册。
func TestRegistry_FileRegistered(t *testing.T) {
	s, err := CreateSource(FactoryConfig{Type: SourceTypeFile})
	if err != nil {
		t.Fatalf("CreateSource(file) error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil source")
	}
	if s.Info().Type != "file" {
		t.Errorf("expected type=file, got %s", s.Info().Type)
	}
}

// TestRegistry_DockerRegistered 验证 DockerSource 工厂已注册。
func TestRegistry_DockerRegistered(t *testing.T) {
	s, err := CreateSource(FactoryConfig{Type: SourceTypeDocker})
	if err != nil {
		t.Fatalf("CreateSource(docker) error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil source")
	}
	if s.Info().Type != "docker" {
		t.Errorf("expected type=docker, got %s", s.Info().Type)
	}
}

// TestRegistry_UnknownType 验证未注册类型返回错误。
func TestRegistry_UnknownType(t *testing.T) {
	_, err := CreateSource(FactoryConfig{Type: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown source type")
	}
}

// TestRegistry_CustomRegistration 验证外部可注册自定义源。
func TestRegistry_CustomRegistration(t *testing.T) {
	const customType SourceType = "mock"
	RegisterSource(customType, func(cfg FactoryConfig) (Source, error) {
		return &mockSource{}, nil
	})
	s, err := CreateSource(FactoryConfig{Type: customType})
	if err != nil {
		t.Fatalf("CreateSource(mock) error: %v", err)
	}
	if _, ok := s.(*mockSource); !ok {
		t.Fatalf("expected *mockSource, got %T", s)
	}
}

// TestRegisteredTypes 验证注册中心返回已注册类型。
func TestRegisteredTypes(t *testing.T) {
	types := RegisteredTypes()
	if len(types) == 0 {
		t.Fatal("expected at least one registered type")
	}
	seen := map[SourceType]bool{}
	for _, tp := range types {
		seen[tp] = true
	}
	for _, expected := range []SourceType{SourceTypeFile, SourceTypeDocker, SourceTypeK8s} {
		if !seen[expected] {
			t.Errorf("expected %s to be registered", expected)
		}
	}
}

// mockSource 是 registry 测试用的最小 Source 实现。
type mockSource struct{}

func (m *mockSource) Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	return 0, nil
}
func (m *mockSource) Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error {
	return nil
}
func (m *mockSource) Info() SourceInfo {
	return SourceInfo{Type: "mock"}
}
func (m *mockSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent)
	close(ch)
	return ch, nil
}
