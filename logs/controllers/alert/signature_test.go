package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	models "ydsz-trace/logs/models"
)

// TestSignWebhookURL_NoSecretUnchanged 无 secret 参数时 URL 不变。
func TestSignWebhookURL_NoSecretUnchanged(t *testing.T) {
	in := "https://example.com/hook?access_token=abc"
	out, err := SignWebhookURL(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != in {
		t.Errorf("URL should be unchanged, got %s", out)
	}
}

// TestSignWebhookURL_AppendsSign 含 secret 时追加 timestamp 与 sign。
func TestSignWebhookURL_AppendsSign(t *testing.T) {
	in := "https://example.com/hook?access_token=abc&secret=SEC123"
	out, err := SignWebhookURL(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == in {
		t.Fatal("URL should be modified")
	}
	// 必须保留原有参数并新增两个
	u := mustParse(t, out)
	if u.Query().Get("access_token") != "abc" {
		t.Error("access_token lost")
	}
	if u.Query().Get("secret") != "SEC123" {
		t.Error("secret lost")
	}
	if u.Query().Get("timestamp") == "" {
		t.Error("timestamp missing")
	}
	if u.Query().Get("sign") == "" {
		t.Error("sign missing")
	}
}

// TestBuildSignature_Deterministic 相同输入产生相同签名。
func TestBuildSignature_Deterministic(t *testing.T) {
	ts := time.UnixMilli(1700000000000)
	_, sign1 := BuildSignature("secret", ts)
	_, sign2 := BuildSignature("secret", ts)
	if sign1 != sign2 {
		t.Errorf("签名应确定: %s vs %s", sign1, sign2)
	}
}

// TestBuildSignature_DifferentSecretDifferentSign 不同 secret 签名不同。
func TestBuildSignature_DifferentSecretDifferentSign(t *testing.T) {
	ts := time.UnixMilli(1700000000000)
	_, s1 := BuildSignature("secret-a", ts)
	_, s2 := BuildSignature("secret-b", ts)
	if s1 == s2 {
		t.Fatal("不同 secret 不应产生相同签名")
	}
}

// TestDingTalkNotifier_DeliversWithSignature 端到端：钉钉渠道带签名投递。
func TestDingTalkNotifier_DeliversWithSignature(t *testing.T) {
	var gotQuery string
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	// 通过 test server URL 注入 secret（模拟真实配置）
	target := srv.URL + "?access_token=t&secret=SEC_TEST"

	notifier := &DingTalkNotifier{}
	rule := models.AlertRule{
		Name:       "test",
		Channel:    ChannelDingTalk,
		WebhookURL: target,
		Threshold:  1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := notifier.Send(ctx, rule, 5, []string{"sample"})
	if result.Err != nil {
		t.Fatalf("send failed: %v", result.Err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("status = %d", result.StatusCode)
	}
	if gotQuery == "" {
		t.Fatal("no query captured")
	}
	// 验证签名参数确实被发送
	q := mustParse(t, "https://x/?"+gotQuery)
	if q.Query().Get("timestamp") == "" || q.Query().Get("sign") == "" {
		t.Errorf("签名参数未发送, query=%s", gotQuery)
	}
	// 验证消息体结构
	if gotBody["msgtype"] != "text" {
		t.Errorf("msgtype = %v", gotBody["msgtype"])
	}
}

// TestWeComNotifier_DeliversWithSignature 端到端：企微渠道带签名投递。
func TestWeComNotifier_DeliversWithSignature(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	target := srv.URL + "?access_token=t&secret=SEC_TEST"
	notifier := &WeComNotifier{}
	rule := models.AlertRule{
		Name:       "test",
		Channel:    ChannelWeCom,
		WebhookURL: target,
		Threshold:  1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := notifier.Send(ctx, rule, 3, nil)
	if result.Err != nil {
		t.Fatalf("send failed: %v", result.Err)
	}
	q := mustParse(t, "https://x/?"+gotQuery)
	if q.Query().Get("sign") == "" {
		t.Errorf("签名参数未发送, query=%s", gotQuery)
	}
}

// TestDoPostJSON_EmptyURL 空 URL 时投递返回错误（避免无意义的网络请求）。
func TestDoPostJSON_EmptyURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result := doPostJSON(ctx, "", map[string]string{"a": "b"}, nil)
	if result.Err == nil {
		t.Fatal("空 URL 应返回错误")
	}
}

// TestSignSecret_NotPresent 无 secret 参数时返回 false。
func TestSignSecret_NotPresent(t *testing.T) {
	if _, ok := SignSecret("https://example.com/hook?access_token=t"); ok {
		t.Fatal("无 secret 时应返回 false")
	}
}

// TestSignSecret_Present secret 存在时正确解析。
func TestSignSecret_Present(t *testing.T) {
	secret, ok := SignSecret("https://example.com/hook?access_token=t&secret=SEC9")
	if !ok {
		t.Fatal("应识别 secret")
	}
	if secret != "SEC9" {
		t.Errorf("secret = %q want SEC9", secret)
	}
}

// mustParse 解析 URL 并在失败时终止测试。
func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return u
}
