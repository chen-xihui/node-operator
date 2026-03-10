package controllers

import (
	"context"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("node", req.Name)

	// Fetch the Node instance
	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// List all NodeGroups
	nodeGroupList := &nodeoperatorv1alpha1.NodeGroupList{}
	if err := r.List(ctx, nodeGroupList); err != nil {
		log.Error(err, "unable to list NodeGroups")
		return ctrl.Result{}, err
	}

	// Check if this node belongs to any NodeGroup
	for _, nodeGroup := range nodeGroupList.Items {
		// Check if node matches the NodeGroup's node selector
		matches := true
		for key, value := range nodeGroup.Spec.NodeSelector {
			if node.Labels[key] != value {
				matches = false
				break
			}
		}

		if matches {
			// Update node status in the NodeGroup
			r.updateNodeStatusInNodeGroup(ctx, nodeGroup, *node)
		}
	}

	return ctrl.Result{}, nil
}

// updateNodeStatusInNodeGroup updates the status of a node in a NodeGroup
func (r *NodeReconciler) updateNodeStatusInNodeGroup(ctx context.Context, nodeGroup nodeoperatorv1alpha1.NodeGroup, node corev1.Node) {
	// Check if the node is already in the NodeGroup's status
	if _, exists := nodeGroup.Status.NodeStatuses[node.Name]; exists {
		// Update node status
		health := "healthy"
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
				health = "unhealthy"
				break
			}
		}

		// Calculate resource usage
		cpuUsage := calculateCPUUsage(node)
		memoryUsage := calculateMemoryUsage(node)
		diskUsage := calculateDiskUsage(node)

		// Update node status
		nodeStatus := nodeoperatorv1alpha1.NodeStatus{
			Role: "none",
			Health: health,
			ResourceUsage: nodeoperatorv1alpha1.ResourceUsage{
				CPU:    cpuUsage,
				Memory: memoryUsage,
				Disk:   diskUsage,
			},
		}

		// Check if node is a master or backup
		for _, masterNode := range nodeGroup.Status.MasterNodes {
			if masterNode == node.Name {
				nodeStatus.Role = "master"
				break
			}
		}
		for _, backupNode := range nodeGroup.Status.BackupNodes {
			if backupNode == node.Name {
				nodeStatus.Role = "backup"
				break
			}
		}

		nodeGroup.Status.NodeStatuses[node.Name] = nodeStatus

		// Update the NodeGroup status
		if err := r.Status().Update(ctx, &nodeGroup); err != nil {
			log.FromContext(ctx).Error(err, "unable to update NodeGroup status", "nodeGroup", nodeGroup.Name)
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
