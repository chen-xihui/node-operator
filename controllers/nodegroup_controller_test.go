package controllers

import (
	"context"
	"testing"

	nodeoperatorv1alpha1 "node-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNodeGroupReconciler(t *testing.T) {
	// Setup test environment
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{"../config/crd/bases"},
	}

	// Start test environment
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("Failed to start test environment: %v", err)
	}
	defer testEnv.Stop()

	// Setup scheme
	if err := nodeoperatorv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	// Create client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create NodeGroup CR
	nodeGroup := &nodeoperatorv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "default",
		},
		Spec: nodeoperatorv1alpha1.NodeGroupSpec{
			MasterCount:       1,
			BackupCount:       1,
			ResourceThreshold: 80,
			NodeSelector:      map[string]string{"test": "true"},
			FailoverPolicy: nodeoperatorv1alpha1.FailoverPolicy{
				Enabled: true,
				Timeout: 30,
			},
		},
	}

	if err := k8sClient.Create(context.Background(), nodeGroup); err != nil {
		t.Fatalf("Failed to create NodeGroup: %v", err)
	}

	// Create test nodes
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node-1",
			Labels: map[string]string{"test": "true"},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node-2",
			Labels: map[string]string{"test": "true"},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	if err := k8sClient.Create(context.Background(), node1); err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}

	if err := k8sClient.Create(context.Background(), node2); err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	// Create reconciler
	reconciler := &NodeGroupReconciler{
		Client: k8sClient,
		Scheme: scheme.Scheme,
	}

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      "test-nodegroup",
			Namespace: "default",
		},
	}

	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}

	// Get updated NodeGroup
	updatedNodeGroup := &nodeoperatorv1alpha1.NodeGroup{}
	if err := k8sClient.Get(context.Background(), req.NamespacedName, updatedNodeGroup); err != nil {
		t.Fatalf("Failed to get updated NodeGroup: %v", err)
	}

	// Verify primary and backup nodes are set
	if len(updatedNodeGroup.Status.PrimaryNodes) != 1 {
		t.Errorf("Expected 1 primary node, got %d", len(updatedNodeGroup.Status.PrimaryNodes))
	}

	if len(updatedNodeGroup.Status.BackupNodes) != 1 {
		t.Errorf("Expected 1 backup node, got %d", len(updatedNodeGroup.Status.BackupNodes))
	}

	// Test failover
	// Make primary node unhealthy
	unhealthyNode := node1.DeepCopy()
	unhealthyNode.Status.Conditions[0].Status = corev1.ConditionFalse
	if err := k8sClient.Status().Update(context.Background(), unhealthyNode); err != nil {
		t.Fatalf("Failed to update node status: %v", err)
	}

	// Reconcile again
	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to reconcile after node failure: %v", err)
	}

	// Get updated NodeGroup
	if err := k8sClient.Get(context.Background(), req.NamespacedName, updatedNodeGroup); err != nil {
		t.Fatalf("Failed to get updated NodeGroup after failover: %v", err)
	}

	// Verify failover occurred
	if len(updatedNodeGroup.Status.PrimaryNodes) != 1 {
		t.Errorf("Expected 1 primary node after failover, got %d", len(updatedNodeGroup.Status.PrimaryNodes))
	}

	// Verify last failover time is set
	if updatedNodeGroup.Status.LastFailoverTime == nil {
		t.Error("Expected last failover time to be set")
	}
}
