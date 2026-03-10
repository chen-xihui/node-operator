package controllers

import (
	"context"
	"time"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"
	"node-operator/internal"

	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("nodegroup", req.NamespacedName)

	nodeGroup := &nodeoperatorv1alpha1.NodeGroup{}
	if err := r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nm := internal.NewNodeManager(r.Client)
	zoneManager := internal.NewZoneManager(nodeGroup.Spec.Zones)
	resourceManager := internal.NewResourceManager(r.Client, nodeGroup.Spec.ResourceThreshold)
	failoverHandler := internal.NewFailoverHandler(nm, zoneManager, resourceManager, nodeGroup.Spec.MasterCount, nodeGroup.Spec.BackupCount)

	nodes, err := nm.GetAllNodes(ctx, nodeGroup.Spec.NodeSelector)
	if err != nil {
		log.Error(err, "Failed to get nodes")
		return ctrl.Result{}, err
	}

	log.Info("Node info", "nodes", internal.PrintNodeInfo(nodes))

	if err := r.initializeNodeGroup(ctx, nodeGroup, nodes, zoneManager, nm); err != nil {
		log.Error(err, "Failed to initialize node group")
	}

	if err := r.syncNodeStatuses(ctx, nodeGroup, nodes, resourceManager); err != nil {
		log.Error(err, "Failed to sync node statuses")
	}

	if err := r.handleFailures(ctx, nodeGroup, nodes, failoverHandler); err != nil {
		log.Error(err, "Failed to handle failures")
	}

	if err := r.handleOverloads(ctx, nodeGroup, nodes, resourceManager, failoverHandler); err != nil {
		log.Error(err, "Failed to handle overloads")
	}

	if err := r.handleRecoveries(ctx, nodeGroup, nodes, failoverHandler); err != nil {
		log.Error(err, "Failed to handle recoveries")
	}

	if err := r.handleOverloadRecoveries(ctx, nodeGroup, nodes, resourceManager, failoverHandler); err != nil {
		log.Error(err, "Failed to handle overload recoveries")
	}

	r.updateZoneStatus(ctx, nodeGroup, nodes)

	if err := r.Status().Update(ctx, nodeGroup); err != nil {
		log.Error(err, "Failed to update node group status")
		return ctrl.Result{}, err
	}

	reconcileInterval := 30 * time.Second
	if nodeGroup.Spec.ReconciliationInterval > 0 {
		reconcileInterval = time.Duration(nodeGroup.Spec.ReconciliationInterval) * time.Second
	}

	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

func (r *NodeGroupReconciler) initializeNodeGroup(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, zoneManager *internal.ZoneManager, nm *internal.NodeManager) error {
	if nodeGroup.Status.NodeStatuses == nil {
		nodeGroup.Status.NodeStatuses = make(map[string]nodeoperatorv1alpha1.NodeStatus)
	}
	if nodeGroup.Status.PrimaryNodes == nil {
		nodeGroup.Status.PrimaryNodes = []string{}
	}
	if nodeGroup.Status.BackupNodes == nil {
		nodeGroup.Status.BackupNodes = []string{}
	}
	if nodeGroup.Status.DedicatedNodes == nil {
		nodeGroup.Status.DedicatedNodes = []string{}
	}
	if nodeGroup.Status.PromotedNodes == nil {
		nodeGroup.Status.PromotedNodes = make(map[string]string)
	}
	if nodeGroup.Status.OverloadedNodes == nil {
		nodeGroup.Status.OverloadedNodes = []string{}
	}
	if nodeGroup.Status.ZoneStatus == nil {
		nodeGroup.Status.ZoneStatus = make(map[string]nodeoperatorv1alpha1.ZoneStatus)
	}

	if len(nodeGroup.Spec.InitialBackupCount) == 0 {
		initialBackupCount := make(map[string]int)
		zones := zoneManager.GetZones()
		backupPerZone := nodeGroup.Spec.BackupCount / len(zones)
		for _, zone := range zones {
			initialBackupCount[zone] = backupPerZone
		}
		nodeGroup.Spec.InitialBackupCount = initialBackupCount
	}

	primaryNodes := nm.GetPrimaryNodes(nodes)
	backupNodes := nm.GetBackupNodes(nodes)

	if len(primaryNodes) == 0 && len(backupNodes) > 0 {
		zoneStats := zoneManager.CalculateZoneStats(nodes, nodeGroup.Status.PromotedNodes)
		zones := zoneManager.GetZones()

		for _, zone := range zones {
			requiredPrimaries := nodeGroup.Spec.MasterCount / len(zones)
			if zoneStats[zone].PrimaryCount < requiredPrimaries {
				backupInZone := r.getBackupNodesInZone(backupNodes, zone)
				if len(backupInZone) > 0 {
					bestNode := nm.SelectBestBackupNode(ctx, backupInZone, []string{})
					if bestNode != nil {
						log.FromContext(ctx).Info("Initial: Promoting backup to primary", "node", bestNode.Name, "zone", bestNode.Zone)
						if err := nm.AddPaasLabel(ctx, bestNode.Name); err != nil {
							log.FromContext(ctx).Error(err, "Failed to add paas label", "node", bestNode.Name)
						}
					}
				}
			}
		}
	}

	return nil
}

func (r *NodeGroupReconciler) syncNodeStatuses(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, resourceManager *internal.ResourceManager) error {
	usages, err := resourceManager.GetAllNodesResourceUsage(ctx, nil)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to get resource usage", "error", err)
	}

	primaryMap := make(map[string]bool)
	backupMap := make(map[string]bool)
	dedicatedMap := make(map[string]bool)

	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
			primaryMap[node.Name] = true
		}
		if node.Labels[nodeoperatorv1alpha1.LabelDedicated] == "true" {
			dedicatedMap[node.Name] = true
		}
		if !primaryMap[node.Name] && !dedicatedMap[node.Name] {
			backupMap[node.Name] = true
		}
	}

	for _, node := range nodes {
		health := nodeoperatorv1alpha1.HealthHealthy
		if !node.IsReady {
			health = nodeoperatorv1alpha1.HealthUnhealthy
		}

		role := nodeoperatorv1alpha1.RoleNone
		if primaryMap[node.Name] {
			role = nodeoperatorv1alpha1.RolePrimary
		} else if dedicatedMap[node.Name] {
			role = nodeoperatorv1alpha1.RoleDedicated
		} else {
			role = nodeoperatorv1alpha1.RoleBackup
		}

		cpuPercent := 0
		memPercent := 0
		if usage, ok := usages[node.Name]; ok {
			cpuPercent = usage.CPUPercent
			memPercent = usage.MemoryPercent
		}

		isPromoted := node.Labels[nodeoperatorv1alpha1.LabelPromoted] == "true"
		promotedFrom := nodeGroup.Status.PromotedNodes[node.Name]

		nodeGroup.Status.NodeStatuses[node.Name] = nodeoperatorv1alpha1.NodeStatus{
			Role:   role,
			Health: health,
			Zone:   node.Zone,
			ResourceUsage: nodeoperatorv1alpha1.ResourceUsage{
				CPU:    cpuPercent,
				Memory: memPercent,
			},
			IsPromoted:   isPromoted,
			PromotedFrom: promotedFrom,
		}
	}

	primaryList := []string{}
	backupList := []string{}
	dedicatedList := []string{}

	for name := range primaryMap {
		primaryList = append(primaryList, name)
	}
	for name := range backupMap {
		backupList = append(backupList, name)
	}
	for name := range dedicatedMap {
		dedicatedList = append(dedicatedList, name)
	}

	nodeGroup.Status.PrimaryNodes = primaryList
	nodeGroup.Status.BackupNodes = backupList
	nodeGroup.Status.DedicatedNodes = dedicatedList

	return nil
}

func (r *NodeGroupReconciler) handleFailures(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, failoverHandler *internal.FailoverHandler) error {
	if !nodeGroup.Spec.FailoverPolicy.Enabled {
		return nil
	}

	previousFailedNodes := make(map[string]bool)
	for _, nodeName := range nodeGroup.Status.NodeStatuses {
		if nodeName.Health == nodeoperatorv1alpha1.HealthUnhealthy && nodeName.Role == nodeoperatorv1alpha1.RolePrimary {
			previousFailedNodes[nodeName.Zone] = true
		}
	}

	for _, node := range nodes {
		if !node.IsReady && node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
			if node.Labels[nodeoperatorv1alpha1.LabelFailed] != "true" {
				log.FromContext(ctx).Info("Primary node became unhealthy", "node", node.Name)
				if err := failoverHandler.HandlePrimaryFailure(ctx, node.Name, nodes); err != nil {
					log.FromContext(ctx).Error(err, "Failed to handle primary failure", "node", node.Name)
				}
			}
		}
	}

	return nil
}

func (r *NodeGroupReconciler) handleOverloads(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, resourceManager *internal.ResourceManager, failoverHandler *internal.FailoverHandler) error {
	if !nodeGroup.Spec.FailoverPolicy.Enabled {
		return nil
	}

	overloadedNodes, err := resourceManager.GetOverloadedNodes(ctx, nil)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to get overloaded nodes", "error", err)
		return nil
	}

	currentOverloaded := make(map[string]bool)
	for _, nodeName := range overloadedNodes {
		currentOverloaded[nodeName] = true
	}

	previousOverloaded := make(map[string]bool)
	for _, nodeName := range nodeGroup.Status.OverloadedNodes {
		previousOverloaded[nodeName] = true
	}

	for _, node := range nodes {
		if currentOverloaded[node.Name] && !previousOverloaded[node.Name] {
			if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
				log.FromContext(ctx).Info("Primary node became overloaded", "node", node.Name)
				if err := failoverHandler.HandleOverloadedNode(ctx, node.Name, nodes); err != nil {
					log.FromContext(ctx).Error(err, "Failed to handle overloaded node", "node", node.Name)
				}
			}
		}
	}

	nodeGroup.Status.OverloadedNodes = overloadedNodes
	return nil
}

func (r *NodeGroupReconciler) handleRecoveries(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, failoverHandler *internal.FailoverHandler) error {
	for _, node := range nodes {
		if node.IsReady && node.Labels[nodeoperatorv1alpha1.LabelFailed] == "true" {
			log.FromContext(ctx).Info("Failed node recovered", "node", node.Name)
			if err := failoverHandler.HandleNodeRecovery(ctx, node.Name, nodes); err != nil {
				log.FromContext(ctx).Error(err, "Failed to handle node recovery", "node", node.Name)
			}
		}
	}
	return nil
}

func (r *NodeGroupReconciler) handleOverloadRecoveries(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo, resourceManager *internal.ResourceManager, failoverHandler *internal.FailoverHandler) error {
	overloadedNodes, err := resourceManager.GetOverloadedNodes(ctx, nil)
	if err != nil {
		return nil
	}

	currentOverloaded := make(map[string]bool)
	for _, nodeName := range overloadedNodes {
		currentOverloaded[nodeName] = true
	}

	for _, node := range nodes {
		if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" && !currentOverloaded[node.Name] {
			wasOverloaded := false
			for _, prevNode := range nodeGroup.Status.OverloadedNodes {
				if prevNode == node.Name {
					wasOverloaded = true
					break
				}
			}
			if wasOverloaded {
				log.FromContext(ctx).Info("Overloaded node recovered", "node", node.Name)
				if err := failoverHandler.HandleOverloadRecovery(ctx, node.Name); err != nil {
					log.FromContext(ctx).Error(err, "Failed to handle overload recovery", "node", node.Name)
				}
			}
		}
	}

	return nil
}

func (r *NodeGroupReconciler) updateZoneStatus(ctx context.Context, nodeGroup *nodeoperatorv1alpha1.NodeGroup, nodes []internal.NodeInfo) {
	zoneStats := make(map[string]nodeoperatorv1alpha1.ZoneStatus)

	zones := []string{
		"region-name.az01arm",
		"region-name.az02arm",
		"region-name.az03arm",
	}
	if len(nodeGroup.Spec.Zones) > 0 {
		zones = nodeGroup.Spec.Zones
	}

	for _, zone := range zones {
		zoneStats[zone] = nodeoperatorv1alpha1.ZoneStatus{}
	}

	for _, node := range nodes {
		zone := node.Zone
		if zone == "" {
			continue
		}

		if status, ok := zoneStats[zone]; ok {
			if node.Labels[nodeoperatorv1alpha1.LabelPaas] == "true" {
				status.PrimaryCount++
			}
			if node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
				node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" {
				status.BackupCount++
			}
			if node.Labels[nodeoperatorv1alpha1.LabelFailed] == "true" {
				status.FailedCount++
			}
			zoneStats[zone] = status
		}
	}

	nodeGroup.Status.ZoneStatus = zoneStats
	log.FromContext(ctx).Info("Zone status updated", "status", zoneStats)
}

func (r *NodeGroupReconciler) getBackupNodesInZone(nodes []internal.NodeInfo, zone string) []internal.NodeInfo {
	var backups []internal.NodeInfo
	for _, node := range nodes {
		if node.Zone == zone && node.Labels[nodeoperatorv1alpha1.LabelPaas] != "true" &&
			node.Labels[nodeoperatorv1alpha1.LabelDedicated] != "true" && node.IsReady {
			backups = append(backups, node)
		}
	}
	return backups
}

func SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeoperatorv1alpha1.NodeGroup{}).
		Owns(&corev1.Node{}).
		Complete(&NodeGroupReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
}
