package source

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// dockerAPIVersion 默认与 Docker Engine 兼容的 API 版本。
	dockerAPIVersion = "v1.41"
)

// DockerClient 封装对 Docker API 的高层调用（专为日志场景裁剪）。
type DockerClient struct {
	socketPath string
	apiVersion string
	httpClient *http.Client
}

// NewDockerClient 构建一个使用 Unix socket（或 Windows named pipe）的 Docker client。
func NewDockerClient(socketPath string) *DockerClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &DockerClient{
		socketPath: socketPath,
		apiVersion: dockerAPIVersion,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Minute, // 日志流可能持续很久
		},
	}
}

// NewDockerClientWindows 返回使用 named pipe 的 Docker client（Windows 环境）。
func NewDockerClientWindows(pipeName string) *DockerClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return winNetDial(ctx, pipeName)
		},
	}
	return &DockerClient{
		socketPath: pipeName,
		apiVersion: dockerAPIVersion,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Minute,
		},
	}
}

// do 发起 Docker API 请求；对非 200/101 状态码返回错误。
func (d *DockerClient) do(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	reqURL := fmt.Sprintf("http://localhost/%s%s", d.apiVersion, path)
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker API 请求失败 %s %s: %w", method, path, err)
	}
	return resp, nil
}

// ContainerSummary Docker 容器的精简信息。
type ContainerSummary struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	Labels map[string]string `json:"Labels"`
	State  string            `json:"State"`
	Mounts []MountPoint      `json:"Mounts"`
}

// MountPoint 容器的挂载点信息。
type MountPoint struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

// ListContainers 列出所有满足 label filter 的容器。
// filterLabel 格式："key=value"，为空则返回全部。
func (d *DockerClient) ListContainers(ctx context.Context, runningOnly bool, filterLabel string) ([]ContainerSummary, error) {
	filters := map[string]bool{}
	if runningOnly {
		filters["status"] = true // placeholder；下面用 query param
	}
	query := url.Values{}
	filterMap := map[string][]string{}
	if filterLabel != "" {
		filterMap["label"] = []string{filterLabel}
	}
	filtersJSON, _ := json.Marshal(filterMap)
	query.Set("filters", string(filtersJSON))

	resp, err := d.do(ctx, "GET", "/containers/json?"+query.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list containers 返回 %d", resp.StatusCode)
	}

	// Docker API 返回的 containers 数组字段与 ContainerSummary 对应
	var rawContainers []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
		State  string            `json:"State"`
		Mounts []MountPoint      `json:"Mounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawContainers); err != nil {
		return nil, fmt.Errorf("decode containers: %w", err)
	}

	var result []ContainerSummary
	for _, c := range rawContainers {
		if runningOnly && c.State != "running" {
			continue
		}
		result = append(result, ContainerSummary{
			ID:     c.ID,
			Names:  c.Names,
			Image:  c.Image,
			Labels: c.Labels,
			State:  c.State,
			Mounts: c.Mounts,
		})
	}
	return result, nil
}

// GetLogs 返回容器日志的流。
// 使用 follow=false, tail=0 获取完整历史；
// follow=true 获取后续增量（注意：日志流 HTTP 状态码是 200/101）。
func (d *DockerClient) GetLogs(ctx context.Context, containerID string, follow bool, tailLines int64) (io.ReadCloser, error) {
	params := url.Values{}
	params.Set("stdout", "true")
	params.Set("stderr", "true")
	params.Set("timestamps", "false")
	params.Set("follow", fmt.Sprintf("%t", follow))
	if tailLines > 0 {
		params.Set("tail", fmt.Sprintf("%d", tailLines))
	}
	params.Set("since", "0")
	path := fmt.Sprintf("/containers/%s/logs?%s", containerID, params.Encode())

	resp, err := d.do(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return nil, fmt.Errorf("get logs 返回 %d", resp.StatusCode)
	}
	return &dockerLogReadCloser{body: resp.Body}, nil
}

// dockerLogReadCloser 处理 Docker 日志的 multiplexing header，
// 把底层流转换为 plain 文本行流。
type dockerLogReadCloser struct {
	body   io.ReadCloser
	buf    [8]byte
	remain []byte // 单次 header 读取后剩余的字节
}

func (d *dockerLogReadCloser) Read(p []byte) (int, error) {
	// 如果上次 header 读取后还有未消费数据
	if len(d.remain) > 0 {
		n := copy(p, d.remain)
		d.remain = d.remain[n:]
		return n, nil
	}

	// 读取 8 字节 multiplexing header
	if _, err := io.ReadFull(d.body, d.buf[:]); err != nil {
		return 0, err
	}

	size := uint32(d.buf[4])<<24 | uint32(d.buf[5])<<16 | uint32(d.buf[6])<<8 | uint32(d.buf[7])
	if size == 0 {
		return 0, nil
	}

	// 读取 payload
	buf := make([]byte, size)
	if _, err := io.ReadFull(d.body, buf); err != nil {
		return 0, err
	}

	n := copy(p, buf)
	if n < len(buf) {
		d.remain = buf[n:]
	}
	return n, nil
}

func (d *dockerLogReadCloser) Close() error {
	return d.body.Close()
}

// dockerEvent Docker 事件结构（简化字段）。
type dockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

// DockerSource 实现 Source 接口的 Docker 模式。
type DockerSource struct {
	client         *DockerClient
	config         DockerConfig
	info           SourceInfo
	mu             sync.RWMutex
	containers     map[string]*dockerTarget
	eventCallback  func(DiscoveryEvent) // 变更事件回调
}

// DockerConfig DockerSource 配置。
type DockerConfig struct {
	SocketPath         string        // Unix socket 路径或 Windows pipe 名
	ContainerLabel     string        // 自动发现容器的 label 过滤（如 ydsz-trace/collect=true）
	AppGroupKey        string        // 应用分组标签键
	AppGroupNameKey    string        // 应用名标签键
	DefaultTailLines   int64         // Hist拉取最近行数
	BackOffMax         time.Duration // watch 断连重试最大间隔
	UseWindowsPipe     bool          // Windows 环境标志
}

// dockerTarget Docker 容器的采集目标。
type dockerTarget struct {
	containerID string
	name        string
	image       string
	appGroup    string
	appName     string
	labels      map[string]string
	state       string
}

// NewDockerSource 构建 DockerSource 实例。
func NewDockerSource(cfg DockerConfig) (*DockerSource, error) {
	var client *DockerClient
	if cfg.UseWindowsPipe {
		client = NewDockerClientWindows(cfg.SocketPath)
	} else {
		client = NewDockerClient(cfg.SocketPath)
	}

	if cfg.BackOffMax <= 0 {
		cfg.BackOffMax = 30 * time.Second
	}
	if cfg.DefaultTailLines <= 0 {
		cfg.DefaultTailLines = 50000
	}

	return &DockerSource{
		client:     client,
		config:     cfg,
		containers: make(map[string]*dockerTarget),
		info: SourceInfo{
			Type:        "docker",
			Description: fmt.Sprintf("Docker source via %s", cfg.SocketPath),
			StartedAt:   time.Now(),
		},
	}, nil
}

// SetEventCallback 设置外部监听容器变更的回调。
func (ds *DockerSource) SetEventCallback(cb func(DiscoveryEvent)) {
	ds.eventCallback = cb
}

// Read 实现 Source 接口：读容器 stdout 或挂载卷中的日志文件。
//
// path 格式：
//   - "container:<containerID>"：按容器 ID 读取 stdout 输出
//   - "container:<containerID>?file=/path/in/container"：按容器 ID 读取内部文件
//   - "name:<containerName>"：按容器名读取
func (ds *DockerSource) Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	containerID, err := ds.resolveContainer(path)
	if err != nil {
		return 0, err
	}

	rc, err := ds.client.GetLogs(ctx, containerID, false, ds.config.DefaultTailLines)
	if err != nil {
		return 0, fmt.Errorf("获取日志流失败: %w", err)
	}
	defer rc.Close()

	return ScanAndFilter(rc, cfg, output)
}

// Tail 实现 Source 接口：持续跟踪容器 stdout。
func (ds *DockerSource) Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error {
	containerID, err := ds.resolveContainer(path)
	if err != nil {
		return err
	}

	tf, err := NewTailFilter(cfg)
	if err != nil {
		return err
	}

	// follow=true 获取持续流
	rc, err := ds.client.GetLogs(ctx, containerID, true, 0)
	if err != nil {
		return fmt.Errorf("获取日志流失败: %w", err)
	}
	defer rc.Close()

	reader := bufio.NewReader(rc)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("读取日志错误: %w", err)
			}
			// EOF 表示容器停止或日志流关闭；等待后重试
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if tf.Match(line) {
			if err := callback(line); err != nil {
				return err
			}
		}
	}
}

// Discover 发现 Docker 事件（create/start/die/destroy），
// 通过 channel 推送 DiscoveryEvent。
func (ds *DockerSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent, 100)

	// 首次全量快照
	snapshot, err := ds.listTargets(ctx)
	if err == nil {
		ds.mu.Lock()
		ds.containers = make(map[string]*dockerTarget, len(snapshot))
		for _, t := range snapshot {
			ds.containers[t.containerID] = t
		}
		ds.mu.Unlock()
		ch <- DiscoveryEvent{Type: "snapshot", Targets: ds.toDiscoveryTargets(snapshot), EventTime: time.Now()}
	}

	// 启动 watch goroutine
	go ds.watchEvents(ctx, ch)

	return ch, nil
}

// Info 返回摘要。
func (ds *DockerSource) Info() SourceInfo {
	return ds.info
}

// watchEvents 监听 Docker 事件流，推送容器变更。
func (ds *DockerSource) watchEvents(ctx context.Context, ch chan<- DiscoveryEvent) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			close(ch)
			return
		default:
		}

		err := ds.streamEvents(ctx, ch)
		if err != nil {
			log.Printf("[docker-source] events stream 错误: %v，%v 后重试", err, backoff)
			select {
			case <-ctx.Done():
				close(ch)
				return
			case <-time.After(backoff):
			}
			if backoff < ds.config.BackOffMax {
				backoff *= 2
				if backoff > ds.config.BackOffMax {
					backoff = ds.config.BackOffMax
				}
			}
		} else {
			backoff = time.Second
		}
	}
}

// streamEvents 持续读取 Docker event stream。
func (ds *DockerSource) streamEvents(ctx context.Context, ch chan<- DiscoveryEvent) error {
	filters := map[string]interface{}{
		"type": []string{"container"},
	}
	filtersJSON, _ := json.Marshal(filters)
	query := url.Values{}
	query.Set("filters", string(filtersJSON))
	path := "/events?" + query.Encode()

	resp, err := ds.client.do(ctx, "GET", path, nil, map[string]string{"Accept": "application/json"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("events stream returned %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("read event: %w", err)
		}
		line = trimBytes(line)
		if len(line) == 0 {
			continue
		}

		var event dockerEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		switch event.Action {
		case "start", "create":
			targets, _ := ds.listTargets(ctx)
			// 找出新增的容器
			for _, t := range targets {
				ds.mu.Lock()
				if _, exists := ds.containers[t.containerID]; !exists {
					ds.containers[t.containerID] = t
					ds.mu.Unlock()
					ch <- DiscoveryEvent{
						Type:      "add",
						Targets:   []DiscoveryTarget{ds.toDiscoveryTarget(t)},
						EventTime: time.Now(),
					}
				} else {
					ds.mu.Unlock()
				}
			}
		case "die", "stop", "destroy", "delete":
			ds.mu.Lock()
			if target, exists := ds.containers[event.Actor.ID]; exists {
				delete(ds.containers, event.Actor.ID)
				ds.mu.Unlock()
				ch <- DiscoveryEvent{
					Type:      "remove",
					Targets:   []DiscoveryTarget{ds.toDiscoveryTarget(target)},
					EventTime: time.Now(),
				}
			} else {
				ds.mu.Unlock()
			}
		}
	}
}

// listTargets 列出所有满足条件的容器。
func (ds *DockerSource) listTargets(ctx context.Context) ([]*dockerTarget, error) {
	containers, err := ds.client.ListContainers(ctx, true, ds.config.ContainerLabel)
	if err != nil {
		return nil, err
	}

	var targets []*dockerTarget
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		t := &dockerTarget{
			containerID: c.ID,
			name:        name,
			image:       c.Image,
			labels:      c.Labels,
			state:       c.State,
		}
		if ds.config.AppGroupKey != "" {
			t.appGroup = c.Labels[ds.config.AppGroupKey]
		}
		if ds.config.AppGroupNameKey != "" {
			t.appName = c.Labels[ds.config.AppGroupNameKey]
		}
		if t.appName == "" {
			t.appName = name
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// resolveContainer 从 path 解析容器 ID 并返回完整 64 字符 ID。
func (ds *DockerSource) resolveContainer(path string) (string, error) {
	// container:<id>
	id := path
	if strings.HasPrefix(path, "container:") {
		id = strings.TrimPrefix(path, "container:")
	} else if strings.HasPrefix(path, "name:") {
		name := strings.TrimPrefix(path, "name:")
		containers, err := ds.client.ListContainers(context.Background(), true, "")
		if err != nil {
			return "", fmt.Errorf("查找容器失败: %w", err)
		}
		for _, c := range containers {
			for _, n := range c.Names {
				if strings.TrimPrefix(n, "/") == name {
					return c.ID, nil
				}
			}
		}
		return "", fmt.Errorf("未找到名为 %s 的容器", name)
	}

	if len(id) == 0 {
		return "", fmt.Errorf("container ID 不能为空")
	}
	return id, nil
}

// toDiscoveryTarget 转换为统一 DiscoveryTarget。
func (ds *DockerSource) toDiscoveryTarget(t *dockerTarget) DiscoveryTarget {
	return DiscoveryTarget{
		Identity:    t.containerID,
		DisplayName: t.appName,
		SourceType:  "docker",
		LogPath:     "container:" + t.containerID,
		Labels: map[string]string{
			"name":     t.name,
			"image":    t.image,
			"appGroup": t.appGroup,
		},
	}
}

func (ds *DockerSource) toDiscoveryTargets(targets []*dockerTarget) []DiscoveryTarget {
	result := make([]DiscoveryTarget, 0, len(targets))
	for _, t := range targets {
		result = append(result, ds.toDiscoveryTarget(t))
	}
	return result
}

func trimBytes(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func init() {
	if os.Getenv("YDSZ_SOURCE_DEBUG") == "1" {
		log.SetFlags(log.Ltime | log.Lshortfile)
	}
}
