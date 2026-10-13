// Package mask 脱敏规则引擎单元测试。
package mask

import (
	"testing"
)

// TestMask_Phone 验证手机号前 3 后 4 脱敏。
func TestMask_Phone(t *testing.T) {
	got := Mask("手机号: 13812345678", MaskPhone)
	expected := "手机号: 138****5678"
	if got != expected {
		t.Errorf("MaskPhone got %q, want %q", got, expected)
	}
}

// TestMask_IDCard 验证身份证前 4 后 4 脱敏。
func TestMask_IDCard(t *testing.T) {
	got := Mask("身份证: 110101199003077777", MaskIDCard)
	expected := "身份证: 1101**********7777"
	if got != expected {
		t.Errorf("MaskIDCard got %q, want %q", got, expected)
	}
}

// TestMask_BankCard 验证银行卡前 4 后 4 脱敏。
func TestMask_BankCard(t *testing.T) {
	got := Mask("卡号: 6225888888888888", MaskBankCard)
	expected := "卡号: 6225 **** **** 8888"
	if got != expected {
		t.Errorf("MaskBankCard got %q, want %q", got, expected)
	}
}

// TestMask_Email 验证邮箱脱敏。
func TestMask_Email(t *testing.T) {
	got := Mask("邮箱: alice@example.com", MaskEmail)
	expected := "邮箱: a***@example.com"
	if got != expected {
		t.Errorf("MaskEmail got %q, want %q", got, expected)
	}
}

// TestMask_IP 验证 IP 脱敏。
func TestMask_IP(t *testing.T) {
	got := Mask("来自 192.168.1.100 的请求", MaskIP)
	expected := "来自 192.168.***.***.100 的请求"
	if got != expected {
		t.Errorf("MaskIP got %q, want %q", got, expected)
	}
}

// TestMask_Name 验证姓名脱敏。
func TestMask_Name(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"张三", "张*"},
		{"李四五", "李**"},
		{"王", "王"},
		{"欧阳修", "欧**"},
	}
	for _, tt := range tests {
		got := Mask(tt.input, MaskName)
		if got != tt.expected {
			t.Errorf("MaskName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// TestMaskAll 验证所有预置策略集成脱敏。
func TestMaskAll(t *testing.T) {
	input := "用户 张三 手机 13812345678 邮箱 zhang@example.com IP 10.0.0.1"
	got := MaskAll(input)
	// 应该同时脱敏手机、IP、姓名、邮箱
	if got == input {
		t.Error("MaskAll 未对输入做任何修改")
	}
	// 手机号应该被脱敏
	if !containsStr(got, "138****5678") {
		t.Errorf("MaskAll 未脱敏手机号: %s", got)
	}
	// 邮箱应该被脱敏
	if !containsStr(got, "z***@example.com") {
		t.Errorf("MaskAll 未脱敏邮箱: %s", got)
	}
}

// TestMaskWithRule 验证自定义正则脱敏。
func TestMaskWithRule(t *testing.T) {
	got, err := MaskWithRule("password=secret123", `(password=)\w+`, `$1******`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "password=******"
	if got != expected {
		t.Errorf("MaskWithRule got %q, want %q", got, expected)
	}
}

// TestMaskWithRule_InvalidRegex 验证非法正则有错。
func TestMaskWithRule_InvalidRegex(t *testing.T) {
	_, err := MaskWithRule("input", `[invalid`, `$1`)
	if err == nil {
		t.Error("非法正则应返回错误")
	}
}

type personForTest struct {
	Name  string `json:"name" mask:"name"`
	Phone string `json:"phone" mask:"phone"`
	Age   int    `json:"age"`
	Email string `json:"email" mask:"email"`
}

// TestMaskStruct 验证结构体反射脱敏。
func TestMaskStruct(t *testing.T) {
	p := personForTest{
		Name:  "张三丰",
		Phone: "13812345678",
		Age:   30,
		Email: "zhang@example.com",
	}

	result := MaskStruct(p)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("MaskStruct 应返回 map[string]interface{}")
	}

	if m["name"] != "张**" {
		t.Errorf("name 脱敏错误: %v", m["name"])
	}
	if m["phone"] != "138****5678" {
		t.Errorf("phone 脱敏错误: %v", m["phone"])
	}
	if m["age"] != 30 {
		t.Errorf("age 应保持不变: %v", m["age"])
	}
	if m["email"] != "z***@example.com" {
		t.Errorf("email 脱敏错误: %v", m["email"])
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchSub(s, sub)
}

func searchSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
