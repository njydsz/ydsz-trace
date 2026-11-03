// Package alert 子文件：IM 机器人签名（V2.2-05）。
//
// 背景：
//   钉钉与企微群机器人开启「加签」安全设置后，请求必须携带 sign / timestamp，
//   否则返回 400000（签名不匹配）。原实现未处理，导致安全配置开启时投递失败。
//
// 签名算法：
//   string_to_sign = "{timestamp}\n{secret}"
//   sign           = base64(hmac_sha256(string_to_sign))
//
// 参考：
//   - 钉钉：https://open.dingtalk.com/document/robots/customize-robot-security-settings
//   - 企微：https://developer.work.weixin.qq.com/document/path/91770
package alert

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// SignSecret 从 webhook URL 中解析加签密钥。
//
// 约定：密钥通过 query 参数 `secret` 携带，例如
//
//	https://oapi.dingtalk.com/robot/send?access_token=xxx&secret=SECxxxx
//
// 返回 (secret, true)；URL 无 secret 参数时返回 ("", false)。
func SignSecret(webhookURL string) (string, bool) {
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", false
	}
	secret := u.Query().Get("secret")
	if secret == "" {
		return "", false
	}
	return secret, true
}

// BuildSignature 生成加签参数（timestamp + sign）。
//
// 钉钉与企微的签名串构造一致（timestamp\nsecret），此处统一实现。
// timestamp 为毫秒级 Unix 时间戳字符串。
func BuildSignature(secret string, ts time.Time) (timestamp string, sign string) {
	ms := ts.UnixMilli()
	timestamp = strconv.FormatInt(ms, 10)

	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	sign = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return timestamp, sign
}

// ApplySignature 将签名参数附加到 webhook URL 的 query 上。
//
// 已有签名参数时会被覆盖（避免重复拼接导致签名失效）。
func ApplySignature(webhookURL, secret string) (string, error) {
	u, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("解析 webhook URL 失败: %w", err)
	}

	ts, sign := BuildSignature(secret, time.Now())
	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// SignWebhookURL 便捷函数：若 URL 含 secret 参数则加签，否则原样返回。
func SignWebhookURL(webhookURL string) (string, error) {
	secret, ok := SignSecret(webhookURL)
	if !ok {
		return webhookURL, nil
	}
	return ApplySignature(webhookURL, secret)
}
