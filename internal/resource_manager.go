package internal

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ResourceManager struct {
	client           client.Client
	thresholdPercent int
	metricsClient    metricsclientset.Interface
}

func NewResourceManager(c client.Client, thresholdPercent int) *ResourceManager {
	if thresholdPercent == 0 {
		thresholdPercent = 85
	}
	// 创建 metrics 客户端
	config := ctrl.GetConfigOrDie()
	var metricsClient metricsclientset.Interface
	if config != nil {
		metricsClient, _ = metricsclientset.NewForConfig(config)
	}

	return &ResourceManager{
		client:           c,
		metricsClient:    metricsClient,
		thresholdPercent: thresholdPercent,
	}
}

func (rm *ResourceManager) GetThreshold() int {
	return rm.thresholdPercent
}

type NodeResourceUsage struct {
	NodeName      string
	CPUPercent    int
	MemoryPercent int
	CPUUsed       int64
	MemUsed       int64
	CPUCapacity   int64
	MemCapacity   int64
}

func (rm *ResourceManager) GetNodeResourceUsage(ctx context.Context, nodeName string) (*NodeResourceUsage, error) {
	usage, err := rm.getNodeMetrics(ctx, nodeName)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to get metrics from API, trying kubectl", "node", nodeName, "error", err)
		usage, err = rm.getNodeMetricsFromKubectl(ctx, nodeName)
		if err != nil {
			log.FromContext(ctx).Error(err, "Failed to get node metrics", "node", nodeName)
			return nil, err
		}
	}
	return usage, nil
}

func (rm *ResourceManager) getNodeMetrics(ctx context.Context, nodeName string) (*NodeResourceUsage, error) {
	cmd := exec.Command("kubectl", "top", "node", nodeName, "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, nodeName) {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				cpuStr := strings.TrimSuffix(fields[1], "m")
				memStr := strings.TrimSuffix(fields[2], "Mi")

				cpu, _ := strconv.Atoi(cpuStr)
				mem, _ := strconv.Atoi(memStr)

				return &NodeResourceUsage{
					NodeName:      nodeName,
					CPUUsed:       int64(cpu),
					MemUsed:       int64(mem),
					CPUPercent:    0,
					MemoryPercent: 0,
				}, nil
			}
		}
	}
	return nil, nil
}

func (rm *ResourceManager) getNodeMetricsFromKubectl(ctx context.Context, nodeName string) (*NodeResourceUsage, error) {
	cmd := exec.Command("kubectl", "top", "node", nodeName, "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, nodeName) {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				cpuStr := strings.ReplaceAll(fields[1], "m", "")
				cpuStr = strings.ReplaceAll(cpuStr, "%", "")
				memStr := strings.ReplaceAll(fields[2], "Mi", "")
				memStr = strings.ReplaceAll(memStr, "%", "")

				cpu, _ := strconv.Atoi(cpuStr)
				mem, _ := strconv.Atoi(memStr)

				return &NodeResourceUsage{
					NodeName:      nodeName,
					CPUUsed:       int64(cpu),
					MemUsed:       int64(mem),
					CPUPercent:    cpu,
					MemoryPercent: mem,
				}, nil
			}
		}
	}
	return nil, nil
}

func (rm *ResourceManager) GetAllNodesResourceUsage(ctx context.Context, nodeNames []string) (map[string]*NodeResourceUsage, error) {
	usages := make(map[string]*NodeResourceUsage)

	// 如果 metrics 客户端不可用，回退到原来的实现
	if rm.metricsClient == nil {
		return rm.fallbackGetAllNodesResourceUsage(ctx, nodeNames)
	}

	// 使用 Kubernetes Metrics API 获取节点指标
	nodeMetricsList, err := rm.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get node metrics from Metrics API, falling back to kubectl top")
		return rm.fallbackGetAllNodesResourceUsage(ctx, nodeNames)
	}

	// 获取节点信息以获取可分配资源
	nodeList := &corev1.NodeList{}
	if err := rm.client.List(ctx, nodeList); err != nil {
		log.FromContext(ctx).Error(err, "Failed to get node list")
		return usages, err
	}

	nodeAllocatable := make(map[string]corev1.ResourceList)
	for _, node := range nodeList.Items {
		nodeAllocatable[node.Name] = node.Status.Allocatable
	}

	// 处理每个节点的指标
	for _, nodeMetrics := range nodeMetricsList.Items {
		nodeName := nodeMetrics.Name

		// 检查是否在指定的节点列表中
		found := len(nodeNames) == 0
		if !found {
			for _, n := range nodeNames {
				if n == nodeName {
					found = true
					break
				}
			}
		}

		if !found {
			continue
		}

		// 获取节点的可分配资源
		allocatable, exists := nodeAllocatable[nodeName]
		if !exists {
			log.FromContext(ctx).Info("Node allocatable not found", "node", nodeName)
			continue
		}

		// 计算 CPU 使用率
		cpuUsage := nodeMetrics.Usage[corev1.ResourceCPU]
		cpuAllocatable := allocatable[corev1.ResourceCPU]
		cpuPercent := 0
		if !cpuAllocatable.IsZero() {
			cpuPercent = int(float64(cpuUsage.MilliValue()) / float64(cpuAllocatable.MilliValue()) * 100)
		}

		// 计算内存使用率
		memUsage := nodeMetrics.Usage[corev1.ResourceMemory]
		memAllocatable := allocatable[corev1.ResourceMemory]
		memPercent := 0
		if !memAllocatable.IsZero() {
			memPercent = int(float64(memUsage.Value()) / float64(memAllocatable.Value()) * 100)
		}

		usages[nodeName] = &NodeResourceUsage{
			NodeName:      nodeName,
			CPUUsed:       cpuUsage.MilliValue(),
			MemUsed:       memUsage.Value(),
			CPUPercent:    cpuPercent,
			MemoryPercent: memPercent,
		}
	}

	return usages, nil
}

// fallbackGetAllNodesResourceUsage 使用 kubectl top 命令作为回退方案
func (rm *ResourceManager) fallbackGetAllNodesResourceUsage(ctx context.Context, nodeNames []string) (map[string]*NodeResourceUsage, error) {
	usages := make(map[string]*NodeResourceUsage)

	cmd := exec.Command("kubectl", "top", "node", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to get node metrics from kubectl top")
		return usages, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		nodeName := fields[0]

		found := false
		if len(nodeNames) == 0 {
			found = true
		} else {
			for _, n := range nodeNames {
				if n == nodeName {
					found = true
					break
				}
			}
		}

		if found {
			// 正确解析 CPU 百分比 (fields[2])
			cpuPercentStr := strings.TrimSuffix(fields[2], "%")
			cpuPercent, err := strconv.Atoi(cpuPercentStr)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to parse CPU percent", "node", nodeName, "value", fields[2])
				continue
			}

			// 正确解析内存百分比 (fields[4])
			memPercentStr := strings.TrimSuffix(fields[4], "%")
			memPercent, err := strconv.Atoi(memPercentStr)
			if err != nil {
				log.FromContext(ctx).Error(err, "Failed to parse memory percent", "node", nodeName, "value", fields[4])
				continue
			}

			// 解析 CPU 使用量 (fields[1]) - 可选，用于记录
			cpuUsedStr := strings.TrimSuffix(fields[1], "m")
			cpuUsed, err := strconv.Atoi(cpuUsedStr)
			if err != nil {
				cpuUsed = 0 // 如果解析失败，设为0
			}

			// 解析内存使用量 (fields[3]) - 可选，用于记录
			memUsedStr := strings.TrimSuffix(fields[3], "Mi")
			memUsed, err := strconv.Atoi(memUsedStr)
			if err != nil {
				memUsed = 0 // 如果解析失败，设为0
			}

			usages[nodeName] = &NodeResourceUsage{
				NodeName:      nodeName,
				CPUUsed:       int64(cpuUsed),
				MemUsed:       int64(memUsed),
				CPUPercent:    cpuPercent,
				MemoryPercent: memPercent,
			}
		}
	}

	return usages, nil
}

func (rm *ResourceManager) IsOverloaded(usage *NodeResourceUsage) bool {
	if usage == nil {
		return false
	}
	return usage.CPUPercent >= rm.thresholdPercent || usage.MemoryPercent >= rm.thresholdPercent
}

func (rm *ResourceManager) GetOverloadedNodes(ctx context.Context, nodeNames []string) ([]string, error) {
	usages, err := rm.GetAllNodesResourceUsage(ctx, nodeNames)
	if err != nil {
		return nil, err
	}

	var overloaded []string
	for name, usage := range usages {
		if rm.IsOverloaded(usage) {
			overloaded = append(overloaded, name)
			log.FromContext(ctx).Info("Node is overloaded", "node", name, "cpu", usage.CPUPercent, "memory", usage.MemoryPercent)
		}
	}
	return overloaded, nil
}
