// Package wsconn 提供最小化 WebSocket（RFC 6455）服务端/客户端实现。
//
// 设计约束（P1 长连接通道）：
//   - 零外部依赖（标准库 only），保持「单二进制」部署形态
//   - 支持二进制帧、ping/pong 保活、close 优雅关闭
//   - 单条连接上多路复用由上层 agentlink 帧协议的 streamID 负责
//
// 本实现刻意只覆盖日志通道所需的最小集合，不实现扩展协商、
// 分片消息重组等高级特性（日志帧由上层保证单帧完整）。
package wsconn

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebSocket 协议常量。
const (
	// wsMagic GUID（RFC 6455 §1.3）。
	wsMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// defaultHandshakeTimeout 握手阶段超时。
	defaultHandshakeTimeout = 10 * time.Second
	// defaultReadLimit 单帧上限（16 MiB，与 agentlink 对齐）。
	defaultReadLimit = 16 * 1024 * 1024
)

// opCode WebSocket 操作码。
type opCode byte

const (
	opContinuation opCode = 0x0
	opText         opCode = 0x1
	opBinary       opCode = 0x2
	opClose        opCode = 0x8
	opPing         opCode = 0x9
	opPong         opCode = 0xA
)

// Message 一条 WebSocket 消息。
type Message struct {
	OPCode  opCode
	Payload []byte
}

// Conn 一条 WebSocket 连接。
//
// 并发模型：
//   - 读：单个 goroutine 独占（ReadMessage）
//   - 写：WriteMessage 通过 writeMu 串行化
type Conn struct {
	conn      net.Conn
	br        *bufio.Reader
	writeMu   sync.Mutex
	readLimit int64
	closeOnce sync.Once
	closed    chan struct{}
}

// Close 关闭底层连接。
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.conn.Close()
	})
	return err
}

// Closed 返回连接是否已关闭。
func (c *Conn) Closed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// SetReadLimit 设置单帧最大字节数（防御内存攻击）。
func (c *Conn) SetReadLimit(n int64) {
	c.readLimit = n
}

// SetReadDeadline 设置读取截止时间。
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入截止时间。
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// WriteMessage 发送一条二进制消息（并发安全）。
func (c *Conn) WriteMessage(payload []byte) error {
	return c.writeFrame(opBinary, payload)
}

// WritePing 发送 ping 控制帧。
func (c *Conn) WritePing(payload []byte) error {
	return c.writeFrame(opPing, payload)
}

// WritePong 发送 pong 控制帧。
func (c *Conn) WritePong(payload []byte) error {
	return c.writeFrame(opPong, payload)
}

// WriteClose 发送 close 控制帧。
func (c *Conn) WriteClose(reason string) error {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload, 1000) // normal closure
	copy(payload[2:], reason)
	return c.writeFrame(opClose, payload)
}

// writeFrame 编码并写入一个帧。
func (c *Conn) writeFrame(code opCode, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var header []byte
	// FIN=1，无扩展。
	header = append(header, byte(0x80)|byte(code))

	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(n))
	case n <= 0xFFFF:
		header = append(header, 126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(n))
		header = append(header, ext...)
	default:
		header = append(header, 127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(n))
		header = append(header, ext...)
	}

	if _, err := c.conn.Write(header); err != nil {
		return fmt.Errorf("写帧头失败: %w", err)
	}
	if n == 0 {
		return nil
	}
	if _, err := c.conn.Write(payload); err != nil {
		return fmt.Errorf("写帧体失败: %w", err)
	}
	return nil
}

// ReadMessage 读取一条消息（阻塞）。
//
// 自动响应 ping → pong；收到 close 时返回 opClose 消息后关闭连接。
// 调用方不应并发调用本方法。
func (c *Conn) ReadMessage() (*Message, error) {
	for {
		code, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch code {
		case opPing:
			if err := c.WritePong(payload); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			// 回送 close 后关闭（RFC 6455 §5.5.1）。
			_ = c.WriteClose("")
			return &Message{OPCode: opClose, Payload: payload}, nil
		case opBinary, opText:
			return &Message{OPCode: code, Payload: payload}, nil
		default:
			return nil, fmt.Errorf("不支持的 opCode: 0x%x", code)
		}
	}
}

// readFrame 读取单个帧（不分片重组）。
func (c *Conn) readFrame() (opCode, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return 0, nil, err
	}

	fin := hdr[0]&0x80 != 0
	code := opCode(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	length := int64(hdr[1] & 0x7F)

	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}

	if !fin {
		return 0, nil, errors.New("不支持分片帧（上层应保证单帧完整）")
	}
	if length > c.readLimit {
		return 0, nil, fmt.Errorf("帧过大: %d > %d", length, c.readLimit)
	}
	if code == opContinuation {
		return 0, nil, errors.New("不支持 continuation 帧")
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return code, payload, nil
}

// Upgrade HTTP 连接为 WebSocket（服务端侧）。
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("缺少 Upgrade: websocket 头")
	}
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		return nil, errors.New("缺少 Connection: upgrade 头")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("缺少 Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("ResponseWriter 不支持 Hijack")
	}
	netConn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	// 计算 accept key。
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(wsMagic))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := netConn.Write([]byte(resp)); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	c := &Conn{
		conn:      netConn,
		br:        brw.Reader,
		readLimit: defaultReadLimit,
		closed:    make(chan struct{}),
	}
	return c, nil
}

// Dial 建立 WebSocket 客户端连接。
func Dial(urlStr string, tlsCfg *tls.Config) (*Conn, error) {
	host, path, useTLS, err := parseWSURL(urlStr)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: defaultHandshakeTimeout}
	var netConn net.Conn
	if useTLS {
		if tlsCfg == nil {
			tlsCfg = &tls.Config{ServerName: hostOnly(host)}
		} else if tlsCfg.ServerName == "" {
			// 拷贝一份避免修改调用方配置。
			copied := tlsCfg.Clone()
			copied.ServerName = hostOnly(host)
			tlsCfg = copied
		}
		netConn, err = tls.DialWithDialer(dialer, "tcp", host, tlsCfg)
	} else {
		netConn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return nil, err
	}

	// 生成随机 key。
	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes[:])

	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := netConn.Write([]byte(req)); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	br := bufio.NewReader(netConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("读取握手响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = netConn.Close()
		return nil, fmt.Errorf("握手失败: HTTP %d", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		_ = netConn.Close()
		return nil, errors.New("服务端未确认 Upgrade")
	}

	return &Conn{
		conn:      netConn,
		br:        br,
		readLimit: defaultReadLimit,
		closed:    make(chan struct{}),
	}, nil
}

// parseWSURL 解析 ws/wss URL 为 host 与 path。
func parseWSURL(raw string) (host, path string, useTLS bool, err error) {
	switch {
	case strings.HasPrefix(raw, "ws://"):
		useTLS = false
		raw = strings.TrimPrefix(raw, "ws://")
	case strings.HasPrefix(raw, "wss://"):
		useTLS = true
		raw = strings.TrimPrefix(raw, "wss://")
	default:
		return "", "", false, errors.New("URL 必须以 ws:// 或 wss:// 开头")
	}

	idx := strings.Index(raw, "/")
	if idx < 0 {
		return raw, "/", useTLS, nil
	}
	return raw[:idx], raw[idx:], useTLS, nil
}

// hostOnly 去掉 host:port 中的端口。
func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// headerContainsToken 判断 Connection 头是否包含指定 token（逗号分隔）。
func headerContainsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
