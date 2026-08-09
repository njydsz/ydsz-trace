package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// K8sSource 通过 K8s API 发现和采集 Pod 容器日志。
//
// 适用场景：K8s 集群（DaemonSet 模式部署，每个节点一个 logc 实例）。
type K8sSource struct {
	client     *kubernetes.Clientset
	config     K8sConfig
	info       SourceInfo
	mu         sync.RWMutex
	pods       map[string]*k8sTarget // podUID/container -> target
}

// K8sConfig K8sSource 的配置。
type K8sConfig struct {
	// NodeName 当前节点名（用于 field selector 过滤本节点 Pod）
	NodeName string
	// Namespace 为空时所有命名空间
	Namespace string
	// DiscoveryAnno 触发采集的 Pod 注解键（如 ydsz-trace/collect）
	DiscoveryAnno string
	// AppGroupKey 应用分组键（annotations 或 labels）
	AppGroupKey string
	// AppNameKey 应用名称键
	AppNameKey string
	// BackOffMax Watch 断连最大退避间隔
	BackOffMax time.Duration
}

// k8sTarget 内部的 Pod 采集目标。
type k8sTarget struct {
	podName       string
	podUID        string
	containerName string
	namespace     string
	appGroup      string
	appName       string
}

// NewK8sSource 使用 in-cluster 配置或本地 kubeconfig 创建 K8sSource。
func NewK8sSource(cfg K8sConfig) (*K8sSource, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("创建 in-cluster rest 配置失败（且未支持 kubeconfig fallback）: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("创建 K8s client 失败: %w", err)
	}

	if cfg.BackOffMax <= 0 {
		cfg.BackOffMax = 30 * time.Second
	}
	if cfg.DiscoveryAnno == "" {
		cfg.DiscoveryAnno = "ydsz-trace/collect"
	}
	if cfg.AppGroupKey == "" {
		cfg.AppGroupKey = "ydsz-trace/app-group"
	}
	if cfg.AppNameKey == "" {
		cfg.AppNameKey = "ydsz-trace/app-name"
	}
	if cfg.NodeName == "" {
		cfg.NodeName = os.Getenv("YDSZ_NODE_NAME")
	}

	return &K8sSource{
		client: client,
		config: cfg,
		pods:   make(map[string]*k8sTarget),
		info: SourceInfo{
			Type:        "k8s",
			Description: fmt.Sprintf("K8s source (node=%s, ns=%s)", cfg.NodeName, cfg.Namespace),
			StartedAt:   time.Now(),
		},
	}, nil
}

// Read 实现 Source 接口：通过 pods/log API 读取容器日志。
func (ks *K8sSource) Read(ctx context.Context, path string, cfg ScanConfig, output io.Writer) (int64, error) {
	namespace, podName, container, err := parseK8sPath(path)
	if err != nil {
		return 0, err
	}

	// 拉取历史日志（最近 50000 行）
	tailLines := int64(50000)
	opts := &corev1.PodLogOptions{
		Container:  container,
		TailLines:  &tailLines,
		Timestamps: false,
	}

	req := ks.client.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return 0, fmt.Errorf("请求 pods/log 失败: %w", err)
	}
	defer stream.Close()

	return ScanAndFilter(stream, cfg, output)
}

// Tail 实现 Source 接口：持续跟踪容器日志流。
func (ks *K8sSource) Tail(ctx context.Context, path string, cfg TailConfig, callback func(line string) error) error {
	namespace, podName, container, err := parseK8sPath(path)
	if err != nil {
		return err
	}

	tf, err := NewTailFilter(cfg)
	if err != nil {
		return err
	}

	opts := &corev1.PodLogOptions{
		Container:  container,
		Follow:     true,
		Timestamps: false,
	}
	req := ks.client.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("请求 pods/log stream 失败: %w", err)
	}
	defer stream.Close()

	reader := bufio.NewReader(stream)
	var deadline time.Time
	if cfg.FollowDuration > 0 {
		deadline = time.Now().Add(time.Duration(cfg.FollowDuration) * time.Second)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("读取日志流错误: %w", err)
			}
			// EOF 后短暂等待再尝试重连
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

// Discover：监听 Pod 事件（create/modify/delete），通过 channel 推送变化。
func (ks *K8sSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent, 100)

	// 首次快照
	snapshot, err := ks.listTargets(ctx)
	if err == nil {
		ks.mu.Lock()
		ks.pods = make(map[string]*k8sTarget, len(snapshot))
		for _, t := range snapshot {
			key := t.podUID + "/" + t.containerName
			ks.pods[key] = t
		}
		ks.mu.Unlock()
		ch <- DiscoveryEvent{Type: "snapshot", Targets: ks.toDiscoveryTargets(snapshot), EventTime: time.Now()}
	}

	go ks.watchPods(ctx, ch)
	return ch, nil
}

// Info 返回摘要。
func (ks *K8sSource) Info() SourceInfo {
	return ks.info
}

// watchPods 持续监听本节点的 Pod 事件。
func (ks *K8sSource) watchPods(ctx context.Context, ch chan<- DiscoveryEvent) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			close(ch)
			return
		default:
		}

		err := ks.streamPodWatch(ctx, ch)
		if err != nil {
			log.Printf("[k8s-source] Watch 断连: %v，%v 后重试", err, backoff)
			select {
			case <-ctx.Done():
				close(ch)
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > ks.config.BackOffMax {
				backoff = ks.config.BackOffMax
			}
		} else {
			backoff = time.Second
		}
	}
}

// streamPodWatch 对节点 Pod 列表发起 Watch 并推送事件。
func (ks *K8sSource) streamPodWatch(ctx context.Context, ch chan<- DiscoveryEvent) error {
	var fieldSelector string
	if ks.config.NodeName != "" {
		fieldSelector = fmt.Sprintf("spec.nodeName=%s", ks.config.NodeName)
	}

	watcher, err := ks.client.CoreV1().Pods(ks.config.Namespace).Watch(ctx,
		metav1.ListOptions{
			FieldSelector: fieldSelector,
			Watch:         true,
		})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				ks.handlePodUpdate(pod, ch)
			case watch.Deleted:
				ks.handlePodDelete(pod, ch)
			}
		}
	}
}

func (ks *K8sSource) handlePodUpdate(pod *corev1.Pod, ch chan<- DiscoveryEvent) {
	targets := ks.extractTargets(pod)
	if len(targets) == 0 {
		return
	}

	for _, t := range targets {
		key := t.podUID + "/" + t.containerName
		ks.mu.Lock()
		_, existed := ks.pods[key]
		ks.pods[key] = t
		ks.mu.Unlock()

		evtType := "add"
		if existed {
			evtType = "update"
		}
		ch <- DiscoveryEvent{
			Type:      evtType,
			Targets:   []DiscoveryTarget{ks.toDiscoveryTarget(t)},
			EventTime: time.Now(),
		}
	}
}

func (ks *K8sSource) handlePodDelete(pod *corev1.Pod, ch chan<- DiscoveryEvent) {
	podKey := string(pod.UID)
	var removed []DiscoveryTarget

	ks.mu.Lock()
	newPods := make(map[string]*k8sTarget, len(ks.pods))
	for key, target := range ks.pods {
		if strings.HasPrefix(key, podKey+"/") {
			removed = append(removed, ks.toDiscoveryTarget(target))
		} else {
			newPods[key] = target
		}
	}
	ks.pods = newPods
	ks.mu.Unlock()

	if len(removed) > 0 {
		ch <- DiscoveryEvent{Type: "remove", Targets: removed, EventTime: time.Now()}
	}
}

// extractTargets 从一个 Pod 中提取采集目标。
func (ks *K8sSource) extractTargets(pod *corev1.Pod) []*k8sTarget {
	// 检查注解
	if pod.Annotations[ks.config.DiscoveryAnno] != "true" {
		return nil
	}

	// 跳过自身（daemonset 自己的 Pod）
	if pod.Labels["app"] == "ydsz-trace-logc" {
		return nil
	}

	appName := ""
	if ks.config.AppNameKey != "" {
		appName = pod.Annotations[ks.config.AppNameKey]
	}
	if appName == "" {
		appName = pod.Labels["app"]
	}
	if appName == "" {
		appName = pod.Name
	}

	appGroup := ""
	if ks.config.AppGroupKey != "" {
		appGroup = pod.Annotations[ks.config.AppGroupKey]
	}

	var targets []*k8sTarget
	for _, c := range pod.Spec.Containers {
		targets = append(targets, &k8sTarget{
			podName:       pod.Name,
			podUID:        string(pod.UID),
			containerName: c.Name,
			namespace:     pod.Namespace,
			appGroup:      appGroup,
			appName:       appName,
		})
	}
	return targets
}

func (ks *K8sSource) listTargets(ctx context.Context) ([]*k8sTarget, error) {
	var fieldSelector string
	if ks.config.NodeName != "" {
		fieldSelector = fmt.Sprintf("spec.nodeName=%s", ks.config.NodeName)
	}

	podList, err := ks.client.CoreV1().Pods(ks.config.Namespace).List(ctx,
		metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return nil, err
	}

	var targets []*k8sTarget
	for i := range podList.Items {
		pod := &podList.Items[i]
		t := ks.extractTargets(pod)
		targets = append(targets, t...)
	}
	return targets, nil
}

func (ks *K8sSource) toDiscoveryTarget(t *k8sTarget) DiscoveryTarget {
	logPath := fmt.Sprintf("k8s://%s/%s/%s", t.namespace, t.podName, t.containerName)
	return DiscoveryTarget{
		Identity:    t.podUID + "/" + t.containerName,
		DisplayName: t.appName + "/" + t.containerName,
		SourceType:  "k8s",
		LogPath:     logPath,
		Labels: map[string]string{
			"pod":       t.podName,
			"namespace": t.namespace,
			"container": t.containerName,
			"appGroup":  t.appGroup,
		},
	}
}

func (ks *K8sSource) toDiscoveryTargets(targets []*k8sTarget) []DiscoveryTarget {
	result := make([]DiscoveryTarget, 0, len(targets))
	for _, t := range targets {
		result = append(result, ks.toDiscoveryTarget(t))
	}
	return result
}

// parseK8sPath 解析 "k8s://namespace/pod/container" 格式。
func parseK8sPath(path string) (namespace, podName, container string, err error) {
	if !strings.HasPrefix(path, "k8s://") {
		err = fmt.Errorf("无法识别的 k8s 路径格式: %s", path)
		return
	}
	trimmed := strings.TrimPrefix(path, "k8s://")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 3 {
		err = fmt.Errorf("k8s 路径格式应为 k8s://namespace/pod/container: %s", path)
		return
	}
	return parts[0], parts[1], parts[2], nil
}

func init() {
	if os.Getenv("YDSZ_SOURCE_DEBUG") == "1" {
		log.SetFlags(log.Ltime | log.Lshortfile)
	}
	// 注册 K8sSource 工厂。
	RegisterSource(SourceTypeK8s, func(cfg FactoryConfig) (Source, error) {
		kcfg := K8sConfig{
			NodeName:      getOpt(cfg.Options, "node_name", os.Getenv("YDSZ_NODE_NAME")),
			Namespace:     getOpt(cfg.Options, "namespace", ""),
			DiscoveryAnno: getOpt(cfg.Options, "discovery_anno", "ydsz-trace/collect"),
			AppGroupKey:   getOpt(cfg.Options, "app_group_key", "ydsz-trace/app-group"),
			AppNameKey:    getOpt(cfg.Options, "app_name_key", "ydsz-trace/app-name"),
		}
		return NewK8sSource(kcfg)
	})
}
