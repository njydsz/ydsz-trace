package middleware

import "encoding/base64"

// tryDecode 尝试按指定配置解码 base64。
//
// pad=true 使用标准字母表（含填充），url=true 使用 URL 安全字母表。
// 三者组合覆盖 SkyWalking sw8 头常见的三种编码变体。
func tryDecode(s string, pad, url bool) (string, bool) {
	var enc *base64.Encoding
	switch {
	case pad && !url:
		enc = base64.StdEncoding
	case !pad && !url:
		enc = base64.RawStdEncoding
	case !pad && url:
		enc = base64.RawURLEncoding
	default:
		enc = base64.URLEncoding
	}

	decoded, err := enc.DecodeString(s)
	if err != nil {
		return "", false
	}
	return string(decoded), true
}
