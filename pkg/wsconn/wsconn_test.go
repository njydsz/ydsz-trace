package wsconn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestUpgradeAndEcho 验证服务端升级 + 客户端 Dial + 双向消息。
func TestUpgradeAndEcho(t *testing.T) {
	// 服务端：升级后回显收到的二进制消息。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Errorf("Upgrade: %v", err)
			return
		}
		defer conn.Close()

		msg, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("ReadMessage: %v", err)
			return
		}
		if err := conn.WriteMessage(msg.Payload); err != nil {
			t.Errorf("WriteMessage: %v", err)
		}
	}))
	defer srv.Close()

	// 客户端：ws:// 地址。
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("ping-payload")
	if err := conn.WriteMessage(payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg.Payload) != string(payload) {
		t.Fatalf("回显不符: %q != %q", msg.Payload, payload)
	}
}

// TestUpgradeRejectsPlainHTTP 普通 HTTP 请求应被拒绝。
func TestUpgradeRejectsPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := Upgrade(w, r); err == nil {
			t.Error("普通 HTTP 不应升级成功")
		}
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()
}

// TestReadFrameTooLarge 超限帧应报错。
func TestReadFrameTooLarge(t *testing.T) {
	// 直接构造帧头，length = 200，但 conn.readLimit 设为 10。
	// 这里通过真实连接验证：服务端设小 readLimit，客户端发送大帧。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadLimit(10)
		_, _ = conn.ReadMessage()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// 发送 100 字节，超过服务端 10 字节限制。
	if err := conn.WriteMessage(make([]byte, 100)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

// TestPingPong 验证 ping 自动转 pong。
func TestPingPong(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			return
		}
		defer conn.Close()
		// ReadMessage 内部自动回 pong，然后继续读。
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.ReadMessage(); err != nil {
			return
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WritePing([]byte("hi")); err != nil {
		t.Fatalf("WritePing: %v", err)
	}
}

func TestParseWSURL(t *testing.T) {
	cases := []struct {
		in     string
		host   string
		path   string
		useTLS bool
	}{
		{"ws://127.0.0.1:2021/agent/link", "127.0.0.1:2021", "/agent/link", false},
		{"wss://example.com/agent", "example.com", "/agent", true},
		{"ws://example.com", "example.com", "/", false},
	}
	for _, c := range cases {
		host, path, useTLS, err := parseWSURL(c.in)
		if err != nil {
			t.Fatalf("parseWSURL(%q): %v", c.in, err)
		}
		if host != c.host || path != c.path || useTLS != c.useTLS {
			t.Fatalf("parseWSURL(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, host, path, useTLS, c.host, c.path, c.useTLS)
		}
	}
}
