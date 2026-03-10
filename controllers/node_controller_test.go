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

func TestNodeReconciler(t *testing.T) {
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

	// Create test node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
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

	if err := k8sClient.Create(context.Background(), node); err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Create reconciler
	reconciler := &NodeReconciler{
		Client: k8sClient,
		Scheme: scheme.Scheme,
	}

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name: "test-node",
		},
	}

	_, err = reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to reconcile: %v", err)
	}

	// Get updated NodeGroup
	updatedNodeGroup := &nodeoperatorv1alpha1.NodeGroup{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-nodegroup", Namespace: "default"}, updatedNodeGroup); err != nil {
		t.Fatalf("Failed to get updated NodeGroup: %v", err)
	}

	// Verify node status is updated
	if _, exists := updatedNodeGroup.Status.NodeStatuses["test-node"]; !exists {
		t.Error("Expected node status to be updated in NodeGroup")
	}
}
