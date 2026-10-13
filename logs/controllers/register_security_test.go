// Package controllers_test 包含 /client/register 注册鉴权安全测试（FR-1308）。
//
// 验证注册接口的鉴权机制：
//   - 无鉴权信息返回 401
//   - 错误密钥/签名返回 401
//   - 重放攻击（过期时间戳）返回 401
//   - 合法密钥或鉴权通过注册流程
package controllers_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testRegisterKey 测试用注册密钥（≥16 字符以触发签名验证）。
const testRegisterKey = "test-register-key-2026-secure"

// computeHMAC 计算 HMAC-SHA1 签名（与 logc 侧 computeHMAC 保持一致）。
func computeHMAC(secret, vkey string, timestamp int64) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "." + vkey))
	return hex.EncodeToString(mac.Sum(nil))
}

// computeAndRegisterBody 计算签名后构造注册请求体。
func computeAndRegisterBody(secret, vkey string, timestamp int64) []byte {
	m := map[string]interface{}{
		"key":       vkey,
		"timestamp": timestamp,
		"signature": computeHMAC(secret, vkey, timestamp),
	}
	b, _ := json.Marshal(m)
	return b
}

// registerBody 构造自定义注册请求体。
func registerBody(vkey string, timestamp int64, signature string) []byte {
	m := map[string]interface{}{
		"key":       vkey,
		"timestamp": timestamp,
		"signature": signature,
	}
	b, _ := json.Marshal(m)
	return b
}

// drainBody 读取并关闭响应体，返回字符串用于错误诊断。
func drainBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// TestRegister_NoAuth 验证空请求体注册返回 401。
func TestRegister_NoAuth(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusUnauthorized {
		t.Errorf("无鉴权注册: 期望 401, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_WrongPlaintextKey 验证错误明文密钥注册返回 401。
func TestRegister_WrongPlaintextKey(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	body := registerBody("wrong-key", time.Now().Unix(), "")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/client/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusUnauthorized {
		t.Errorf("错误明文密钥: 期望 401, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_ReplayAttack 验证过期时间戳签名被拒绝（防重放攻击）。
func TestRegister_ReplayAttack(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	stale := time.Now().Add(-10 * time.Minute).Unix()
	body := computeAndRegisterBody(testRegisterKey, "any-vkey", stale)
	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusUnauthorized {
		t.Errorf("重放攻击(过期时间戳): 期望 401, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_FutureTimestamp 验证超未来时间戳注册被拒绝。
func TestRegister_FutureTimestamp(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	future := time.Now().Add(10 * time.Minute).Unix()
	body := computeAndRegisterBody(testRegisterKey, "any-vkey", future)
	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusUnauthorized {
		t.Errorf("未来时间戳: 期望 401, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_TamperedSignature 验证篡改签名后注册返回 401。
func TestRegister_TamperedSignature(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	now := time.Now().Unix()
	validSig := computeHMAC(testRegisterKey, "my-vkey", now)
	tampered := validSig[:len(validSig)-1] + "x"
	body := registerBody("my-vkey", now, tampered)
	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusUnauthorized {
		t.Errorf("篡改签名: 期望 401, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_ValidHMACPassesAuth 验证合法 HMAC 签名通过鉴权层。
// 传统客户端未预注册 → 期望 403（鉴权通过但业务层拒绝）。
func TestRegister_ValidHMACPassesAuth(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	now := time.Now().Unix()
	body := computeAndRegisterBody(testRegisterKey, "hmac-vkey", now)
	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	code := resp.StatusCode
	_ = resp.Body.Close()

	// 401 = 鉴权层失败（签名校验未通过）→ 测试失败
	if code == http.StatusUnauthorized {
		t.Fatalf("合法 HMAC 签名应通过鉴权层, 得到 401")
	}
	// 服务端错误也不应出现
	if code == http.StatusInternalServerError {
		t.Fatalf("合法请求不应出现 500 错误")
	}
}

// TestRegister_DockerVM_ValidHMAC 验证 Docker 虚拟客户端 HMAC 签名注册成功。
func TestRegister_DockerVM_ValidHMAC(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	t.Setenv("YDSZ_REGISTER_KEY", testRegisterKey)

	now := time.Now().Unix()
	m := map[string]interface{}{
		"key":          "docker-vkey",
		"timestamp":    now,
		"signature":    computeHMAC(testRegisterKey, "docker-vkey", now),
		"source_type":  "docker",
		"identity":     "container-test-001",
		"display_name": "测试容器",
		"log_path":     "/var/log/app.log",
		"labels":       map[string]string{"env": "test"},
	}
	body, _ := json.Marshal(m)

	resp, err := http.Post(srv.URL+"/client/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := resp.StatusCode; got != http.StatusOK {
		t.Errorf("Docker HMAC 注册: 期望 200, 得到 %d, body=%s", got, drainBody(resp))
	} else {
		_ = resp.Body.Close()
	}
}

// TestRegister_HMAC_Deterministic 验证 HMAC 签名计算的确定性与唯一性。
func TestRegister_HMAC_Deterministic(t *testing.T) {
	ts := time.Now().Unix()
	sig1 := computeHMAC(testRegisterKey, "vkey-a", ts)
	sig2 := computeHMAC(testRegisterKey, "vkey-a", ts)

	if sig1 != sig2 {
		t.Fatalf("HMAC 签名应具确定性: %q != %q", sig1, sig2)
	}
	if sig3 := computeHMAC(testRegisterKey, "vkey-b", ts); sig1 == sig3 {
		t.Fatalf("不同 vkey 应产生不同签名")
	}
	if len(sig1) != 40 {
		t.Fatalf("SHA1 HMAC 签名应为 40 字符 hex, 得到 %d", len(sig1))
	}
}
