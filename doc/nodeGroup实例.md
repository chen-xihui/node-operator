## NodeGroupStatus 实际示例

### 1. 完整的 NodeGroup YAML 示例

```yaml
apiVersion: nodeoperator.k8s.io/v1alpha1
kind: NodeGroup
metadata:
  name: production-nodes
  namespace: default
  creationTimestamp: "2026-03-10T10:00:00Z"
spec:
  masterCount: 3
  backupCount: 2
  resourceThreshold: 85
  nodeSelector:
    kubernetes.io/os: linux
    environment: production
  failoverPolicy:
    enabled: true
    timeout: 30
  zones:
  - region-name.az01arm
  - region-name.az02arm
  - region-name.az03arm
  initialBackupCount:
    region-name.az01arm: 1
    region-name.az02arm: 1
    region-name.az03arm: 0
status:
  primaryNodes:
  - node-1
  - node-2
  - node-3
  backupNodes:
  - node-4
  - node-5
  dedicatedNodes:
  - gpu-node-1
  nodeStatuses:
    node-1:
      role: "primary"
      health: "healthy"
      zone: "region-name.az01arm"
      resourceUsage:
        cpu: 65
        memory: 70
        disk: 45
      isPromoted: false
    node-2:
      role: "primary"
      health: "healthy"
      zone: "region-name.az02arm"
      resourceUsage:
        cpu: 45
        memory: 60
        disk: 30
      isPromoted: false
    node-3:
      role: "primary"
      health: "unhealthy"
      zone: "region-name.az03arm"
      resourceUsage:
        cpu: 90
        memory: 85
        disk: 70
      isPromoted: true
      promotedFrom: "node-6"
    node-4:
      role: "backup"
      health: "healthy"
      zone: "region-name.az01arm"
      resourceUsage:
        cpu: 20
        memory: 25
        disk: 15
      isPromoted: false
    node-5:
      role: "backup"
      health: "healthy"
      zone: "region-name.az02arm"
      resourceUsage:
        cpu: 15
        memory: 20
        disk: 10
      isPromoted: false
    gpu-node-1:
      role: "dedicated"
      health: "healthy"
      zone: "region-name.az03arm"
      resourceUsage:
        cpu: 40
        memory: 35
        disk: 25
      isPromoted: false
  zoneStatus:
    region-name.az01arm:
      primaryCount: 1
      backupCount: 1
      failedCount: 0
    region-name.az02arm:
      primaryCount: 1
      backupCount: 1
      failedCount: 0
    region-name.az03arm:
      primaryCount: 1
      backupCount: 0
      failedCount: 0
  lastFailoverTime: "2026-03-10T09:30:00Z"
  promotedNodes:
    node-3: "node-6"
  overloadedNodes:
  - node-3
```

### 2. 状态说明

**节点角色分布**：
- **主用节点**：node-1, node-2, node-3
- **备用节点**：node-4, node-5
- **专用节点**：gpu-node-1

**可用区状态**：
- az01arm：1主1备
- az02arm：1主1备  
- az03arm：1主0备（node-3 过载）

**故障转移历史**：
- node-3 是从 node-6 升级而来
- 最近一次故障转移发生在 09:30

**资源使用情况**：
- node-3 资源使用率超过阈值（90%），被标记为过载
- 其他节点资源使用正常

### 3. 通过 kubectl 查看

```bash
# 查看 NodeGroup 状态
kubectl get nodegroup production-nodes -o yaml

# 只看状态部分
kubectl get nodegroup production-nodes -o jsonpath='{.status}'
```

这个示例展示了 NodeGroupStatus 在实际运行时的完整状态信息，包括节点分配、健康状态、资源使用和故障转移历史等。