package agentlink

import (
	"encoding/json"
	"testing"

	"ydsz-trace/pkg/agentlink"
)

// TestRegisterAccept_Marshal 验证注册应答可序列化并包含 items。
func TestRegisterAccept_Marshal(t *testing.T) {
	accept := agentlink.RegisterAccept{
		ClientID:   42,
		ServerTime: 1700000000,
		Items: []agentlink.ItemSpec{
			{ItemID: 1, ItemName: "app", LogPath: "/var/log/", LogPrefix: "app", Status: "1"},
		},
	}
	data, err := json.Marshal(accept)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got agentlink.RegisterAccept
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.ClientID != 42 {
		t.Errorf("ClientID = %d, want 42", got.ClientID)
	}
	if len(got.Items) != 1 || got.Items[0].ItemID != 1 {
		t.Errorf("Items = %+v", got.Items)
	}
}

// TestServer_SetBatchHandler 验证批次回调可被设置。
func TestServer_SetBatchHandler(t *testing.T) {
	s := NewServer("test-key")
	called := false
	s.SetBatchHandler(func(batch *agentlink.LogBatch) {
		called = true
	})
	if s.onBatch == nil {
		t.Fatal("onBatch not set")
	}
	s.onBatch(&agentlink.LogBatch{BatchNo: "b1"})
	if !called {
		t.Error("handler not called")
	}
}

// TestServer_OnlineCount 验证在线计数。
func TestServer_OnlineCount(t *testing.T) {
	s := NewServer("test-key")
	if s.OnlineCount() != 0 {
		t.Fatalf("OnlineCount = %d, want 0", s.OnlineCount())
	}
}

// TestServer_PushConfig_Offline 验证离线客户端下发返回错误。
func TestServer_PushConfig_Offline(t *testing.T) {
	s := NewServer("test-key")
	err := s.PushConfig(999, &agentlink.ConfigPush{SamplingRate: 0.5})
	if err == nil {
		t.Error("expected error for offline client")
	}
}

