package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelPaas      = "node-role.kubernetes.io/paas"
	LabelFailed    = "node-role.kubernetes.io/failed"
	LabelPromoted  = "node-role.kubernetes.io/promoted"
	LabelDedicated = "node-role.kubernetes.io/dedicated"
	LabelZone      = "topology.kubernetes.io/zone"

	TaintOverloaded = "node-role.kubernetes.io/overloaded"
	TaintDedicated  = "node-role.kubernetes.io/dedicated"

	RolePrimary   = "primary"
	RoleBackup    = "backup"
	RoleDedicated = "dedicated"
	RoleNone      = "none"

	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthOverload  = "overload"
)

type NodeGroupSpec struct {
	MasterCount            int               `json:"masterCount" validate:"required,min=1"`
	BackupCount            int               `json:"backupCount" validate:"required,min=1"`
	ResourceThreshold      int               `json:"resourceThreshold" validate:"required,min=50,max=100"`
	NodeSelector           map[string]string `json:"nodeSelector" validate:"required"`
	FailoverPolicy         FailoverPolicy    `json:"failoverPolicy" validate:"required"`
	ReconciliationInterval int               `json:"reconciliationInterval" validate:"min=10,max=300"`
	ResourceCheckInterval  int               `json:"resourceCheckInterval" validate:"min=5,max=60"`
	Zones                  []string          `json:"zones"`
	InitialBackupCount     map[string]int    `json:"initialBackupCount"`
}

type FailoverPolicy struct {
	Enabled bool `json:"enabled"`
	Timeout int  `json:"timeout" validate:"min=5,max=300"`
}

type NodeGroupStatus struct {
	PrimaryNodes     []string              `json:"primaryNodes"`
	BackupNodes      []string              `json:"backupNodes"`
	DedicatedNodes   []string              `json:"dedicatedNodes"`
	NodeStatuses     map[string]NodeStatus `json:"nodeStatuses"`
	ZoneStatus       map[string]ZoneStatus `json:"zoneStatus"`
	LastFailoverTime *metav1.Time          `json:"lastFailoverTime,omitempty"`
	PromotedNodes    map[string]string     `json:"promotedNodes"`
	OverloadedNodes  []string              `json:"overloadedNodes"`
}

type ZoneStatus struct {
	PrimaryCount int `json:"primaryCount"`
	BackupCount  int `json:"backupCount"`
	FailedCount  int `json:"failedCount"`
}

type NodeStatus struct {
	Role          string        `json:"role"`
	Health        string        `json:"health"`
	Zone          string        `json:"zone"`
	ResourceUsage ResourceUsage `json:"resourceUsage"`
	IsPromoted    bool          `json:"isPromoted"`
	PromotedFrom  string        `json:"promotedFrom,omitempty"`
}

type ResourceUsage struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
	Disk   int `json:"disk"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NodeGroup 是 nodegroups API 的架构
// +kubebuilder:printcolumn:name="MasterCount",type="integer",JSONPath=".spec.masterCount"
// +kubebuilder:printcolumn:name="BackupCount",type="integer",JSONPath=".spec.backupCount"
// +kubebuilder:printcolumn:name="ResourceThreshold",type="integer",JSONPath=".spec.resourceThreshold"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type NodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeGroupSpec   `json:"spec,omitempty"`
	Status NodeGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeGroupList 包含 NodeGroup 的列表
type NodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeGroup{}, &NodeGroupList{})
}
