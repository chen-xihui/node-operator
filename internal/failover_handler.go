package internal

import (
	"context"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FailoverHandler struct {
	nodeManager *NodeManager
	zoneManager *ZoneManager
	resourceMgr *ResourceManager
	masterCount int
	backupCount int
}

func NewFailoverHandler(nm *NodeManager, zm *ZoneManager, rm *ResourceManager, masterCount, backupCount int) *FailoverHandler {
	return &FailoverHandler{
		nodeManager: nm,
		zoneManager: zm,
		resourceMgr: rm,
		masterCount: masterCount,
		backupCount: backupCount,
	}
}

// HandlePrimaryFailure 处理主用节点故障，执行故障转移逻辑
// 当检测到主用节点不可用时，从备用节点中选择合适的节点升级为主用节点
// 参数:
//   - ctx: 上下文，用于取消操作和传递超时
//   - failedNodeName: 故障的主用节点名称
//   - allNodes: 所有节点的信息列表
//   - nodeGroup: NodeGroup 资源，用于更新状态信息
//
// 返回值:
//   - error: 操作过程中出现的错误，成功时返回 nil
func (fh *FailoverHandler) HandlePrimaryFailure(ctx context.Context, failedNodeName string, allNodes []NodeInfo, nodeGroup *nodeoperatorv1alpha1.NodeGroup) error {
	log.FromContext(ctx).Info("Handling primary node failure", "failedNode", failedNodeName)

	if err := fh.nodeManager.AddFailedTaint(ctx, failedNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to add failed taint", "node", failedNodeName)
		return err
	}
	if err := fh.nodeManager.AddFailedLabel(ctx, failedNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to add failed label", "node", failedNodeName)
		return err
	}

	failedNodeZone := ""
	for _, node := range allNodes {
		if node.Name == failedNodeName {
			failedNodeZone = node.Zone
			break
		}
	}

	// 如果节点没有可用区信息，直接返回
	if failedNodeZone == "" {
		log.FromContext(ctx).Info("Failed node has no zone information, skipping capacity check", "node", failedNodeName)
		return nil
	}

	// 获取所有升级节点信息，用于准确统计可用主用节点
	promotedNodesList := fh.nodeManager.GetPromotedNodes(allNodes)
	promotedNodes := make(map[string]string)
	for _, node := range promotedNodesList {
		promotedNodes[node.Name] = "promoted"
	}
	zoneStats := fh.zoneManager.CalculateZoneStats(allNodes, promotedNodes)

	availablePrimaryCount := 0
	if s, ok := zoneStats[failedNodeZone]; ok {
		// 可用主用节点数 = 总主用节点数 - 故障节点 - 过载节点数
		// 故障节点完全不可用，需要减去1；过载节点部分可用，但性能受限
		availablePrimaryCount = s.PrimaryCount - 1 - s.OverloadedCount
	}

	if availablePrimaryCount < fh.masterCount {
		backupNodes := fh.getBackupNodesInZone(allNodes, failedNodeZone)
		// 兜底策略：如果同一可用区没有可用备用节点，则从所有可用区选择
		if len(backupNodes) == 0 {
			// 获取所有可用区的备用节点
			backupNodes = fh.getAllBackupNodes(allNodes)
			// 清空可用区限制，允许跨可用区选择
			failedNodeZone = ""
		}

		bestNode := fh.nodeManager.SelectBestBackupNode(ctx, backupNodes, []string{})
		if bestNode != nil {
			log.FromContext(ctx).Info("Promoting backup node to primary", "node", bestNode.Name, "zone", bestNode.Zone)

			// 为选中的备用节点添加主用节点标签，使其能够接收 Pod 调度
			if err := fh.nodeManager.AddPaasLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add paas label", "node", bestNode.Name)
				return err
			}

			// 添加升级标签，标记该节点是从备用节点升级而来，便于后续恢复处理
			if err := fh.nodeManager.AddPromotedLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add promoted label", "node", bestNode.Name)
				return err
			}

			// 更新 NodeGroup Status 中的 PromotedNodes 字段
			if nodeGroup.Status.PromotedNodes == nil {
				nodeGroup.Status.PromotedNodes = make(map[string]string)
			}
			nodeGroup.Status.PromotedNodes[bestNode.Name] = failedNodeZone

			// 设置故障转移时间戳
			now := metav1.Now()
			nodeGroup.Status.LastFailoverTime = &now

			promotedNodes[bestNode.Name] = failedNodeZone
		}
	}

	return nil
}

// HandleOverloadedNode 处理主用节点过载情况，执行过载保护措施
// 当主用节点资源使用率超过阈值时，采取以下措施：
//  1. 为过载节点添加污点，防止新 Pod 调度到该节点
//  2. 检查同一可用区的主用节点数量，如果不足则升级备用节点补充容量
//  3. 确保服务的高可用性，防止过载影响业务稳定性
//
// 参数:
//   - ctx: 上下文，用于日志记录和取消操作
//   - overloadedNodeName: 过载的主用节点名称
//   - allNodes: 所有节点的信息列表
//   - nodeGroup: NodeGroup 资源，用于更新状态信息
//
// 返回值:
//   - error: 处理过程中出现的错误，成功时返回 nil
func (fh *FailoverHandler) HandleOverloadedNode(ctx context.Context, overloadedNodeName string, allNodes []NodeInfo, nodeGroup *nodeoperatorv1alpha1.NodeGroup) error {
	log.FromContext(ctx).Info("Handling overloaded node", "node", overloadedNodeName)

	// 更新 NodeGroup Status 中的 OverloadedNodes 字段
	if nodeGroup.Status.OverloadedNodes == nil {
		nodeGroup.Status.OverloadedNodes = []string{}
	}
	// 检查是否已经记录过该过载节点
	alreadyRecorded := false
	for _, nodeName := range nodeGroup.Status.OverloadedNodes {
		if nodeName == overloadedNodeName {
			alreadyRecorded = true
			break
		}
	}
	// 如果没有记录过，则添加到过载节点列表
	if !alreadyRecorded {
		nodeGroup.Status.OverloadedNodes = append(nodeGroup.Status.OverloadedNodes, overloadedNodeName)
	}

	if err := fh.nodeManager.AddOverloadedTaint(ctx, overloadedNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to add overloaded taint", "node", overloadedNodeName)
		return err
	}

	// 获取过载节点所在的可用区，用于后续的容量检查和节点选择
	overloadedNodeZone := ""
	for _, node := range allNodes {
		if node.Name == overloadedNodeName {
			overloadedNodeZone = node.Zone
			break
		}
	}

	// 如果节点没有可用区信息，直接返回
	if overloadedNodeZone == "" {
		log.FromContext(ctx).Info("Overloaded node has no zone information, skipping capacity check", "node", overloadedNodeName)
		return nil
	}

	// 获取所有升级节点信息，用于准确统计可用主用节点
	promotedNodesList := fh.nodeManager.GetPromotedNodes(allNodes)
	promotedNodes := make(map[string]string)
	for _, node := range promotedNodesList {
		promotedNodes[node.Name] = "promoted"
	}
	zoneStats := fh.zoneManager.CalculateZoneStats(allNodes, promotedNodes)

	availablePrimaryCount := 0
	if s, ok := zoneStats[overloadedNodeZone]; ok {
		// 可用主用节点数 = 总主用节点数 - 过载节点数（包括当前节点）
		availablePrimaryCount = s.PrimaryCount - s.OverloadedCount
	}

	if availablePrimaryCount < fh.masterCount {
		backupNodes := fh.getBackupNodesInZone(allNodes, overloadedNodeZone)
		// 兜底策略：如果同一可用区没有可用备用节点，则从所有可用区选择
		if len(backupNodes) == 0 {
			// 获取所有可用区的备用节点
			backupNodes = fh.getAllBackupNodes(allNodes)
			// 清空可用区限制，允许跨可用区选择
			overloadedNodeZone = ""
		}

		bestNode := fh.nodeManager.SelectBestBackupNode(ctx, backupNodes, []string{})
		if bestNode != nil {
			log.FromContext(ctx).Info("Promoting backup node due to overload", "node", bestNode.Name, "zone", bestNode.Zone)

			if err := fh.nodeManager.AddPaasLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add paas label", "node", bestNode.Name)
				return err
			}

			if err := fh.nodeManager.AddPromotedLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add promoted label", "node", bestNode.Name)
				return err
			}

			promotedNodes[bestNode.Name] = overloadedNodeZone
		}
	}

	return nil
}

// HandleNodeRecovery 处理节点恢复情况，执行恢复后的节点管理
// 当故障或过载节点恢复正常时，采取以下措施：
//  1. 移除故障或过载标签，使节点恢复正常状态
//  2. 检查是否需要降级刚恢复的节点，保持备用节点数量稳定
//  3. 确保节点角色分配的合理性，避免资源浪费
//
// 参数:
//   - ctx: 上下文，用于日志记录和取消操作
//   - recoveredNodeName: 恢复的节点名称
//   - allNodes: 所有节点的信息列表
//   - nodeGroup: NodeGroup 资源，用于更新状态信息
//
// 返回值:
//   - error: 处理过程中出现的错误，成功时返回 nil
func (fh *FailoverHandler) HandleNodeRecovery(ctx context.Context, recoveredNodeName string, allNodes []NodeInfo, nodeGroup *nodeoperatorv1alpha1.NodeGroup) error {
	log.FromContext(ctx).Info("Handling node recovery", "node", recoveredNodeName)

	// 保留标签移除操作（如果需要向后兼容）
	if err := fh.nodeManager.RemoveFailedLabel(ctx, recoveredNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to remove failed label", "node", recoveredNodeName)
		return err
	}

	recoveredNodeZone := ""
	var recoveredNode *NodeInfo
	for _, node := range allNodes {
		if node.Name == recoveredNodeName {
			recoveredNodeZone = node.Zone
			recoveredNode = &node
			break
		}
	}

	if recoveredNode == nil {
		return nil
	}

	promotedNodes := make(map[string]string)
	for _, node := range allNodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPromoted] == "true" {
			promotedNodes[node.Name] = node.Zone
		}
	}

	zoneStats := fh.zoneManager.CalculateZoneStats(allNodes, promotedNodes)

	initialBackupCount := make(map[string]int)
	// 计算每个可用区的初始备用节点数量，用于后续的降级决策
	for _, zone := range fh.zoneManager.GetZones() {
		// 如果可用区有备用节点或升级节点，计算合理的备用节点数量
		if zoneStats[zone].BackupCount+zoneStats[zone].PromotedCount > 0 {
			// 初始备用数量 = 当前备用节点数 + 升级节点数 - 1（为恢复节点留出空间）
			initialBackupCount[zone] = zoneStats[zone].BackupCount + zoneStats[zone].PromotedCount - 1
		} else {
			// 如果没有备用节点，使用默认的备用节点分配策略
			initialBackupCount[zone] = fh.backupCount / len(fh.zoneManager.GetZones())
		}
	}

	// 根据备用节点数量决定是否需要执行降级操作
	if s, ok := zoneStats[recoveredNodeZone]; ok {
		// 计算当前可用区的实际备用节点数量（包括升级节点）
		currentBackupCount := s.BackupCount + s.PromotedCount
		// 获取该可用区期望的备用节点数量
		expectedBackupCount := initialBackupCount[recoveredNodeZone]

		// 情况1：如果当前备用节点不足，将恢复的节点降级为备用节点
		if currentBackupCount < expectedBackupCount {
			log.FromContext(ctx).Info("Demoting recovered node to backup", "node", recoveredNodeName)
			// 移除主用节点标签，使节点恢复为备用节点
			if err := fh.nodeManager.RemovePaasLabel(ctx, recoveredNodeName); err != nil {
				log.FromContext(ctx).Error(err, "Failed to remove paas label", "node", recoveredNodeName)
				return err
			}
		} else {
			// 情况2：如果当前备用节点充足，说明故障影响已完全消除
			// 清理所有升级标记，让系统恢复到初始状态
			log.FromContext(ctx).Info("Backup nodes are sufficient, cleaning up all promoted labels", "zone", recoveredNodeZone, "currentBackupCount", currentBackupCount, "expectedBackupCount", expectedBackupCount)
			// TODO：是否需要更新status里的信息，比如PromotedCount等
			// 清理所有升级节点的标记
			for _, node := range allNodes {
				if node.Labels[nodeoperatorv1alpha1.LabelPromoted] == "true" {
					log.FromContext(ctx).Info("Cleaning up promoted label from node", "node", node.Name)
					if err := fh.nodeManager.RemovePromotedLabel(ctx, node.Name); err != nil {
						log.FromContext(ctx).Error(err, "Failed to remove promoted label", "node", node.Name)
						// 继续清理其他节点，不立即返回错误
					}

					// 从 NodeGroup Status 中移除升级节点记录
					if nodeGroup.Status.PromotedNodes != nil {
						delete(nodeGroup.Status.PromotedNodes, node.Name)
					}
				}
			}
		}

	}

	// 在所有恢复逻辑完成后，最后移除故障污点
	// 这样可以避免在恢复过程中有新的Pod调度到该节点
	if err := fh.nodeManager.RemoveFailedTaint(ctx, recoveredNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to remove failed taint", "node", recoveredNodeName)
		return err
	}

	return nil
}

func (fh *FailoverHandler) HandleOverloadRecovery(ctx context.Context, recoveredNodeName string, nodeGroup *nodeoperatorv1alpha1.NodeGroup) error {
	log.FromContext(ctx).Info("Handling overload recovery", "node", recoveredNodeName)

	if err := fh.nodeManager.RemoveOverloadedTaint(ctx, recoveredNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to remove overloaded taint", "node", recoveredNodeName)
		return err
	}

	// 从 NodeGroup Status 中移除过载节点记录
	if nodeGroup.Status.OverloadedNodes != nil {
		// 创建新的过载节点列表，排除已恢复的节点
		var updatedOverloadedNodes []string
		for _, nodeName := range nodeGroup.Status.OverloadedNodes {
			if nodeName != recoveredNodeName {
				updatedOverloadedNodes = append(updatedOverloadedNodes, nodeName)
			}
		}
		nodeGroup.Status.OverloadedNodes = updatedOverloadedNodes
	}

	return nil
}

func (fh *FailoverHandler) getBackupNodesInZone(nodes []NodeInfo, zone string) []NodeInfo {
	var backups []NodeInfo
	for _, node := range nodes {
		if node.Zone == zone && node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
			node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" && node.IsReady {
			backups = append(backups, node)
		}
	}
	return backups
}

func (fh *FailoverHandler) getAllBackupNodes(nodes []NodeInfo) []NodeInfo {
	var backups []NodeInfo
	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
			node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" && node.IsReady {
			backups = append(backups, node)
		}
	}
	return backups
}
