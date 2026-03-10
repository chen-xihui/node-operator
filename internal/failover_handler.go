package internal

import (
	"context"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type FailoverHandler struct {
	nodeManager    *NodeManager
	zoneManager    *ZoneManager
	resourceMgr    *ResourceManager
	masterCount    int
	backupCount    int
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

func (fh *FailoverHandler) HandlePrimaryFailure(ctx context.Context, failedNodeName string, allNodes []NodeInfo) error {
	log.FromContext(ctx).Info("Handling primary node failure", "failedNode", failedNodeName)

	if err := fh.nodeManager.AddFailedLabel(ctx, failedNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to add failed label", "node", failedNodeName)
		return err
	}

	promotedNodes := make(map[string]string)
	zoneStats := fh.zoneManager.CalculateZoneStats(allNodes, promotedNodes)

	failedNodeZone := ""
	for _, node := range allNodes {
		if node.Name == failedNodeName {
			failedNodeZone = node.Zone
			break
		}
	}

	availablePrimaryCount := 0
	if s, ok := zoneStats[failedNodeZone]; ok {
		availablePrimaryCount = s.PrimaryCount - 1 - s.OverloadedCount
	}

	if availablePrimaryCount < fh.masterCount {
		backupNodes := fh.getBackupNodesInZone(allNodes, failedNodeZone)
		if len(backupNodes) == 0 {
			backupNodes = fh.getAllBackupNodes(allNodes)
			failedNodeZone = ""
		}

		bestNode := fh.nodeManager.SelectBestBackupNode(ctx, backupNodes, []string{})
		if bestNode != nil {
			log.FromContext(ctx).Info("Promoting backup node to primary", "node", bestNode.Name, "zone", bestNode.Zone)

			if err := fh.nodeManager.AddPaasLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add paas label", "node", bestNode.Name)
				return err
			}

			if err := fh.nodeManager.AddPromotedLabel(ctx, bestNode.Name); err != nil {
				log.FromContext(ctx).Error(err, "Failed to add promoted label", "node", bestNode.Name)
				return err
			}

			promotedNodes[bestNode.Name] = failedNodeZone
		}
	}

	return nil
}

func (fh *FailoverHandler) HandleOverloadedNode(ctx context.Context, overloadedNodeName string, allNodes []NodeInfo) error {
	log.FromContext(ctx).Info("Handling overloaded node", "node", overloadedNodeName)

	if err := fh.nodeManager.AddOverloadedTaint(ctx, overloadedNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to add overloaded taint", "node", overloadedNodeName)
		return err
	}

	overloadedNodeZone := ""
	for _, node := range allNodes {
		if node.Name == overloadedNodeName {
			overloadedNodeZone = node.Zone
			break
		}
	}

	promotedNodes := make(map[string]string)
	zoneStats := fh.zoneManager.CalculateZoneStats(allNodes, promotedNodes)

	availablePrimaryCount := 0
	if s, ok := zoneStats[overloadedNodeZone]; ok {
		availablePrimaryCount = s.PrimaryCount - s.OverloadedCount
	}

	if availablePrimaryCount < fh.masterCount {
		backupNodes := fh.getBackupNodesInZone(allNodes, overloadedNodeZone)
		if len(backupNodes) == 0 {
			backupNodes = fh.getAllBackupNodes(allNodes)
		}

		bestNode := fh.nodeManager.SelectBestBackupNode(ctx, backupNodes, []string{overloadedNodeZone})
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

func (fh *FailoverHandler) HandleNodeRecovery(ctx context.Context, recoveredNodeName string, allNodes []NodeInfo) error {
	log.FromContext(ctx).Info("Handling node recovery", "node", recoveredNodeName)

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
	for _, zone := range fh.zoneManager.GetZones() {
		if zoneStats[zone].BackupCount+zoneStats[zone].PromotedCount > 0 {
			initialBackupCount[zone] = zoneStats[zone].BackupCount + zoneStats[zone].PromotedCount - 1
		} else {
			initialBackupCount[zone] = fh.backupCount / len(fh.zoneManager.GetZones())
		}
	}

	if s, ok := zoneStats[recoveredNodeZone]; ok {
		currentBackupCount := s.BackupCount + s.PromotedCount
		expectedBackupCount := initialBackupCount[recoveredNodeZone]

		if currentBackupCount < expectedBackupCount {
			log.FromContext(ctx).Info("Demoting recovered node to backup", "node", recoveredNodeName)
			if err := fh.nodeManager.RemovePaasLabel(ctx, recoveredNodeName); err != nil {
				log.FromContext(ctx).Error(err, "Failed to remove paas label", "node", recoveredNodeName)
				return err
			}
		} else if s.PromotedCount > 0 {
			for _, node := range allNodes {
				if node.Labels[nodeoperatorv1alpha1.LabelPromoted] == "true" && node.Zone == recoveredNodeZone {
					log.FromContext(ctx).Info("Demoting promoted node to backup", "node", node.Name)
					if err := fh.nodeManager.RemovePaasLabel(ctx, node.Name); err != nil {
						log.FromContext(ctx).Error(err, "Failed to remove paas label", "node", node.Name)
						return err
					}
					if err := fh.nodeManager.RemovePromotedLabel(ctx, node.Name); err != nil {
						log.FromContext(ctx).Error(err, "Failed to remove promoted label", "node", node.Name)
						return err
					}
					break
				}
			}
		}
	}

	return nil
}

func (fh *FailoverHandler) HandleOverloadRecovery(ctx context.Context, recoveredNodeName string) error {
	log.FromContext(ctx).Info("Handling overload recovery", "node", recoveredNodeName)

	if err := fh.nodeManager.RemoveOverloadedTaint(ctx, recoveredNodeName); err != nil {
		log.FromContext(ctx).Error(err, "Failed to remove overloaded taint", "node", recoveredNodeName)
		return err
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
