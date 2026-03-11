package internal

import (
	"context"
	"fmt"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeManager struct {
	client client.Client
}

func NewNodeManager(c client.Client) *NodeManager {
	return &NodeManager{client: c}
}

type NodeInfo struct {
	Name              string
	Zone              string
	Labels            map[string]string
	Taints            []corev1.Taint
	IsReady           bool
	CPUAllocatable    resource.Quantity
	MemoryAllocatable resource.Quantity
	CPUUsed           resource.Quantity
	MemoryUsed        resource.Quantity
	CPUPercent        int
	MemoryPercent     int
}

func (nm *NodeManager) GetAllNodes(ctx context.Context, nodeSelector map[string]string) ([]NodeInfo, error) {
	nodeList := &corev1.NodeList{}
	if err := nm.client.List(ctx, nodeList, client.MatchingLabels(nodeSelector)); err != nil {
		return nil, err
	}

	nodes := make([]NodeInfo, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodeInfo := nm.convertToNodeInfo(node)
		nodes = append(nodes, nodeInfo)
	}
	return nodes, nil
}

func (nm *NodeManager) convertToNodeInfo(node corev1.Node) NodeInfo {
	nodeInfo := NodeInfo{
		Name:    node.Name,
		Labels:  node.Labels,
		Taints:  node.Spec.Taints,
		IsReady: isNodeReady(node),
		Zone:    node.Labels[nodeoperatorv1alpha1.LabelZone],
	}

	allocatable := node.Status.Allocatable
	if allocatable != nil {
		if cpu := allocatable.Cpu(); cpu != nil {
			nodeInfo.CPUAllocatable = *cpu
		}
		if memory := allocatable.Memory(); memory != nil {
			nodeInfo.MemoryAllocatable = *memory
		}
	}

	return nodeInfo
}

func isNodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (nm *NodeManager) UpdateNodeLabels(ctx context.Context, nodeName string, labelsToAdd map[string]string, labelsToRemove []string) error {
	node := &corev1.Node{}
	if err := nm.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	for key, value := range labelsToAdd {
		node.Labels[key] = value
	}

	for _, key := range labelsToRemove {
		delete(node.Labels, key)
	}

	return nm.client.Update(ctx, node)
}

func (nm *NodeManager) UpdateNodeTaints(ctx context.Context, nodeName string, taints []corev1.Taint) error {
	node := &corev1.Node{}
	if err := nm.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	node.Spec.Taints = taints
	return nm.client.Update(ctx, node)
}

func (nm *NodeManager) GetNode(ctx context.Context, nodeName string) (*NodeInfo, error) {
	node := &corev1.Node{}
	if err := nm.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return nil, err
	}

	nodeInfo := nm.convertToNodeInfo(*node)
	return &nodeInfo, nil
}

func (nm *NodeManager) HasLabel(node *corev1.Node, labelKey, labelValue string) bool {
	if node.Labels == nil {
		return false
	}
	return node.Labels[labelKey] == labelValue
}

func (nm *NodeManager) HasTaint(node *corev1.Node, taintKey string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == taintKey {
			return true
		}
	}
	return false
}

func (nm *NodeManager) GetPrimaryNodes(nodes []NodeInfo) []NodeInfo {
	var primaries []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
			primaries = append(primaries, node)
		}
	}
	return primaries
}

func (nm *NodeManager) GetBackupNodes(nodes []NodeInfo) []NodeInfo {
	var backups []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
			node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" {
			backups = append(backups, node)
		}
	}
	return backups
}

func (nm *NodeManager) GetDedicatedNodes(nodes []NodeInfo) []NodeInfo {
	var dedicated []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelDedicated] == "true" {
			dedicated = append(dedicated, node)
		}
	}
	return dedicated
}

func (nm *NodeManager) GetFailedNodes(nodes []NodeInfo) []NodeInfo {
	var failed []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelFailed] == "true" {
			failed = append(failed, node)
		}
	}
	return failed
}

func (nm *NodeManager) GetPromotedNodes(nodes []NodeInfo) []NodeInfo {
	var promoted []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPromoted] == "true" {
			promoted = append(promoted, node)
		}
	}
	return promoted
}

func (nm *NodeManager) GetOverloadedNodes(nodes []NodeInfo) []NodeInfo {
	var overloaded []NodeInfo
	for _, node := range nodes {
		if nm.HasTaintFromList(node.Taints, nodeoperatorv1alpha1.TaintOverloaded) {
			overloaded = append(overloaded, node)
		}
	}
	return overloaded
}

func (nm *NodeManager) HasTaintFromList(taints []corev1.Taint, taintKey string) bool {
	for _, taint := range taints {
		if taint.Key == taintKey {
			return true
		}
	}
	return false
}

func (nm *NodeManager) AddPaasLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Adding paas label to node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, map[string]string{
		nodeoperatorv1alpha1.LabelPaas: "true",
	}, nil)
}

func (nm *NodeManager) RemovePaasLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Removing paas label from node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, nil, []string{nodeoperatorv1alpha1.LabelPaas})
}

func (nm *NodeManager) AddFailedLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Adding failed label to node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, map[string]string{
		nodeoperatorv1alpha1.LabelFailed: "true",
	}, nil)
}

func (nm *NodeManager) RemoveFailedLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Removing failed label from node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, nil, []string{nodeoperatorv1alpha1.LabelFailed})
}

func (nm *NodeManager) AddPromotedLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Adding promoted label to node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, map[string]string{
		nodeoperatorv1alpha1.LabelPromoted: "true",
	}, nil)
}

func (nm *NodeManager) RemovePromotedLabel(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Removing promoted label from node", "node", nodeName)
	return nm.UpdateNodeLabels(ctx, nodeName, nil, []string{nodeoperatorv1alpha1.LabelPromoted})
}

func (nm *NodeManager) AddOverloadedTaint(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Adding overloaded taint to node", "node", nodeName)
	node := &corev1.Node{}
	if err := nm.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	newTaints := append(node.Spec.Taints, corev1.Taint{
		Key:    nodeoperatorv1alpha1.TaintOverloaded,
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	})

	return nm.UpdateNodeTaints(ctx, nodeName, newTaints)
}

func (nm *NodeManager) RemoveOverloadedTaint(ctx context.Context, nodeName string) error {
	log.FromContext(ctx).Info("Removing overloaded taint from node", "node", nodeName)
	node := &corev1.Node{}
	if err := nm.client.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return err
	}

	var newTaints []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Key != nodeoperatorv1alpha1.TaintOverloaded {
			newTaints = append(newTaints, taint)
		}
	}

	return nm.UpdateNodeTaints(ctx, nodeName, newTaints)
}

func (nm *NodeManager) CalculateAvailableResources(node NodeInfo) (cpu resource.Quantity, mem resource.Quantity) {
	cpu = node.CPUAllocatable
	mem = node.MemoryAllocatable
	return
}

// SelectBestBackupNode 从备用节点中选择最优的节点进行升级
// 根据资源可用性和可用区策略选择最适合升级为主用节点的备用节点
// 选择策略：
//  1. 优先选择同一可用区的节点（避免跨可用区迁移）
//  2. 选择资源最充足的节点（CPU 优先，内存次之）
//  3. 排除指定的可用区（用于避免在故障可用区中选择）
//
// 参数:
//   - ctx: 上下文，用于日志记录和取消操作
//   - backupNodes: 可用的备用节点列表
//   - excludeZones: 需要排除的可用区列表
//
// 返回值:
//   - *NodeInfo: 选中的最优备用节点，如果没有合适的节点返回 nil
func (nm *NodeManager) SelectBestBackupNode(ctx context.Context, backupNodes []NodeInfo, excludeZones []string) *NodeInfo {
	var bestNode *NodeInfo
	var maxScore int64 = -1

	for _, node := range backupNodes {
		if nm.contains(excludeZones, node.Zone) {
			continue
		}

		cpuVal := node.CPUAllocatable.MilliValue()
		memVal := node.MemoryAllocatable.Value()
		score := cpuVal/1000 + memVal/1024/1024/100

		if score > maxScore {
			maxScore = score
			bestNode = &node
		}
	}

	if bestNode == nil && len(backupNodes) > 0 {
		var maxBackupCount int64 = -1
		zoneCounts := make(map[string]int64)
		for _, node := range backupNodes {
			zoneCounts[node.Zone]++
		}
		for zone, count := range zoneCounts {
			if count > maxBackupCount {
				maxBackupCount = count
				for _, node := range backupNodes {
					if node.Zone == zone {
						bestNode = &node
						break
					}
				}
			}
		}
	}

	if bestNode == nil && len(backupNodes) > 0 {
		bestNode = &backupNodes[0]
	}

	return bestNode
}

func (nm *NodeManager) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func PrintNodeInfo(nodes []NodeInfo) string {
	str := fmt.Sprintf("Total nodes: %d\n", len(nodes))
	for _, node := range nodes {
		labels := ""
		for k, v := range node.Labels {
			labels += fmt.Sprintf("%s=%s,", k, v)
		}
		str += fmt.Sprintf("Node: %s, Zone: %s, Ready: %v, Labels: %s\n",
			node.Name, node.Zone, node.IsReady, labels)
	}
	return str
}
