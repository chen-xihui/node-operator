# 主备节点规划方案设计文档

## 1. 项目背景与目标

### 1.1 背景
在大规模Kubernetes集群中，节点的合理规划和管理对集群的稳定性和资源利用率至关重要。传统的节点管理方式难以实现自动化的主备节点切换和资源优化，需要一种更智能的方案来确保服务的高可用性和资源的合理分配。

### 1.2 目标
- 设计一种基于Kubernetes标签和污点的主备节点规划方案
- 实现自动检测节点状态、处理故障转移和资源管理
- 确保主用节点故障时能够快速切换到备用节点
- 当主用节点资源使用率超过阈值时，防止新Pod调度到该节点
- 处理故障节点恢复后的角色调整问题
- 保持备用节点数量稳定，避免因升级导致备用节点减少

## 2. 设计方案概述

### 2.1 核心思路
本方案通过动态管理节点标签来实现主备节点的角色切换，替代传统的污点和容忍度机制，从而简化配置并提高灵活性。

### 2.2 节点角色定义
- **主用节点**：带有 `node-role.kubernetes.io/paas=true` 标签的节点，占比为75%
- **备用节点**：不带 `node-role.kubernetes.io/paas` 标签的节点，占比为20%
- **独占节点**：带有 `node-role.kubernetes.io/dedicated=true` 标签的节点，占比为5%


### 2.3 Pod调度策略
每个Pod在创建时应配置如下nodeAffinity规则：
```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: node-role.kubernetes.io/paas
          operator: In
          values:
          - true
```

这样，Pod只会被调度到带有 `node-role.kubernetes.io/paas=true` 标签的节点上。

## 3. 标签和污点的作用及设置

### 3.1 标签设置

| 标签名称 | 标签值 | 作用 | 适用节点 |
|---------|-------|------|----------|
| `node-role.kubernetes.io/paas` | `true` | 标识主用节点，Pod会优先调度到此类节点 | 主用节点、升级后的备用节点 |
| `node-role.kubernetes.io/failed` | `true` | 标识故障节点 | 故障的主用节点 |
| `node-role.kubernetes.io/promoted` | `true` | 标识从备用节点升级而来的主用节点 | 升级后的备用节点 |
| `topology.kubernetes.io/zone` | `region-name.az01arm`, `region-name.az02arm`, `region-name.az03arm` | 标识节点所属的可用区 | 所有节点 |

### 3.2 污点设置

| 污点名称 | 污点值 | 效果 | 适用节点 |
|---------|-------|------|----------|
| `node-role.kubernetes.io/overloaded` | `NoSchedule` | 防止新Pod调度到资源过载的节点 | 资源使用率超过85%的主用节点 |
| `node-role.kubernetes.io/dedicated` | `NoSchedule` | Pod独占节点 | pod独占节点 |
| `node-role.kubernetes.io/failed` | `NoSchedule` | 给故障节点加上，等恢复后通过operator进行删除污点，防止恢复后有pod落上去 | 故障的主用节点 |

### 3.3 标签和污点的管理策略

1. **初始状态**：
   - 主用节点：添加 `node-role.kubernetes.io/paas=true` 标签
   - 备用节点：无特殊标签
   - 所有节点：添加对应的 `topology.kubernetes.io/zone` 标签

2. **故障转移**：
   - 当主用节点故障时，添加 `node-role.kubernetes.io/failed=true` 标签
   - 从备用节点中选择资源最充足且主用节点数量最少的可用区中的节点，添加 `node-role.kubernetes.io/paas=true` 和 `node-role.kubernetes.io/promoted=true` 标签

3. **资源过载**：
   - 当主用节点资源使用率超过85%时，添加 `node-role.kubernetes.io/overloaded:NoSchedule` 污点
   - 当资源使用率恢复正常时，移除该污点

4. **节点恢复**：
   - 故障节点恢复后，移除 `node-role.kubernetes.io/failed=true` 标签
   - 根据当前备用节点数量和可用区均衡情况，决定是否将恢复的节点降级为备用节点

## 4. 主要处理逻辑

### 4.1 节点状态检测

#### 4.1.1 定期检测
- 每30秒检查一次所有节点的状态
- 检测节点是否就绪（NodeReady状态）
- 检测节点的资源使用率
- 检测节点所属的可用区

#### 4.1.2 故障检测
- 当节点状态变为NotReady时，标记为故障节点
- 当节点资源使用率超过85%时，标记为过载节点

### 4.2 故障转移处理

#### 4.2.1 主用节点故障
1. 检测到主用节点故障
2. 检查当前可用主用节点数量
3. 检查故障节点所属的可用区，统计该可用区的可用主用节点数量
4. 如果该可用区的可用主用节点数量不足，从同一可用区的备用节点中选择节点进行升级：
   - 在同一可用区内，选择资源最充足的节点（CPU优先，内存次之）
5. 为选中的备用节点添加 `node-role.kubernetes.io/paas=true` 和 `node-role.kubernetes.io/promoted=true` 标签

#### 4.2.2 资源过载处理
1. 检测到主用节点资源使用率超过85%
2. 为该节点添加 `node-role.kubernetes.io/overloaded:NoSchedule` 污点
3. 检查过载节点所属的可用区，统计该可用区的可用主用节点数量
4. 如果该可用区的可用主用节点数量不足，从同一可用区的备用节点中选择资源最充足的节点进行升级

### 4.3 节点恢复处理

#### 4.3.1 故障节点恢复
1. 检测到故障节点恢复为Ready状态
2. 移除 `node-role.kubernetes.io/failed=true` 标签
3. 检查恢复节点所属的可用区，统计该可用区的备用节点数量
4. 如果该可用区的备用节点数量少于初始数量，将恢复的节点降级为备用节点
5. 如果该可用区的备用节点数量充足，检查该可用区是否有升级的备用节点可以降级，优先降级该可用区中的升级节点

#### 4.3.2 过载节点恢复
1. 检测到过载节点资源使用率恢复正常
2. 移除 `node-role.kubernetes.io/overloaded:NoSchedule` 污点

### 4.4 备用节点管理

#### 4.4.1 数量控制
- 系统启动时记录初始备用节点数量
- 当备用节点因升级为主用节点而减少时，通过降级操作保持备用节点数量稳定

#### 4.4.2 智能选择
- 当需要升级备用节点时，只考虑同一可用区的备用节点，选择资源最充足的节点
- 当需要降级节点时，只考虑同一可用区的升级节点
- 这样可以确保每个可用区的节点数量保持平衡，避免跨可用区的节点迁移

### 4.5 多可用区支持

#### 4.5.1 可用区划分
- 将集群节点均分为3个可用区：
  - `region-name.az01arm`
  - `region-name.az02arm`
  - `region-name.az03arm`

#### 4.5.2 可用区均衡策略
- **升级策略**：优先考虑同一可用区的备用节点进行升级，不跨可用区操作
- **降级策略**：只考虑同一可用区的升级节点进行降级，不跨可用区操作
- **资源平衡**：在同一可用区内，选择资源最充足的节点进行升级
- **数量平衡**：每个可用区的主用节点和备用节点数量保持独立平衡
- **兜底策略**：当同一可用区没有可用备用节点时，从备用节点数量最多的其他可用区选择节点进行升级

#### 4.5.3 高可用性保障
- 确保每个可用区都有足够的主用节点和备用节点
- 当某个可用区的主用节点故障时，优先从同一可用区的备用节点中选择节点进行升级
- 当同一可用区没有可用备用节点时，从备用节点数量最多的其他可用区选择节点进行升级，确保高可用性
- 通过可用区内的节点平衡，提高每个可用区的容错能力
- 避免跨可用区的节点迁移，减少网络延迟和复杂性（仅在必要时进行跨可用区升级）

## 5. 实现细节

### 5.1 API 定义

#### 5.1.1 NodeGroup CRD
```go
type NodeGroupSpec struct {
	MasterCount       int                    `json:"masterCount"`
	BackupCount       int                    `json:"backupCount"`
	ResourceThreshold int                    `json:"resourceThreshold"`
	NodeSelector      map[string]string      `json:"nodeSelector"`
	FailoverPolicy    FailoverPolicy         `json:"failoverPolicy"`
	Zones             []string               `json:"zones"`
	InitialBackupCount map[string]int        `json:"initialBackupCount"`
}

type NodeGroupStatus struct {
	PrimaryNodes      []string              `json:"primaryNodes"`
	BackupNodes       []string              `json:"backupNodes"`
	FailedNodes       []string              `json:"failedNodes"`
	OverloadedNodes   []string              `json:"overloadedNodes"`
	LastFailoverTime  *metav1.Time          `json:"lastFailoverTime,omitempty"`
	ZoneStatus        map[string]ZoneStatus `json:"zoneStatus"`
	NodeStatuses      map[string]NodeStatus `json:"nodeStatuses"`
}

type ZoneStatus struct {
	PrimaryCount int `json:"primaryCount"`
	BackupCount  int `json:"backupCount"`
	FailedCount  int `json:"failedCount"`
}

type NodeStatus struct {
	Role          string       `json:"role"`
	Health        string       `json:"health"`
	ResourceUsage ResourceUsage `json:"resourceUsage"`
}

type ResourceUsage struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
	Disk   int `json:"disk"`
}

type FailoverPolicy struct {
	Enabled bool `json:"enabled"`
	Timeout int  `json:"timeout"`
}
```

### 5.2 核心组件
- **节点管理器**：定期检测节点状态和资源使用率，处理故障转移
- **可用区管理器**：管理可用区分类，处理跨可用区兜底逻辑
- **资源管理器**：监控节点资源使用情况，处理过载节点
- **故障转移处理器**：处理主用节点故障和过载的情况
- **节点控制器**：监控节点状态变化并更新 NodeGroup 状态
- **NodeGroup 控制器**：协调所有管理器，实现主要的 reconciliation 逻辑

### 5.3 资源管理策略

#### 5.3.1 资源使用率计算

**方式一：实时资源使用率监控（用于过载检测）**
- 依赖 Kubernetes metrics-server 组件
- 通过执行 `kubectl top node <节点名>` 命令获取节点的实时资源使用情况
- 解析命令输出结果，提取CPU和内存使用率百分比
- CPU使用率格式：`XX%`（如 "50%"）
- 内存使用率格式：`XX%`（如 "45%"）
- 阈值检查：将使用率百分比与配置的阈值（默认85%）比较，超过阈值则标记为过载节点

**方式一（推荐）：使用 Kubernetes Metrics API 获取资源使用情况**
- 依赖 Kubernetes metrics-server 组件
- 通过直接调用 Kubernetes Metrics API 获取节点的实时资源使用情况
- 使用 `k8s.io/metrics` 包中的客户端库
- 步骤：
  1. 添加 metrics API 依赖：`go get k8s.io/metrics@v0.27.4`
  2. 导入必要的包：`k8s.io/metrics/pkg/apis/metrics/v1beta1` 和 `k8s.io/metrics/pkg/client/clientset/versioned`
  3. 创建 metrics 客户端
  4. 调用 `NodeMetricses().Get()` 方法获取节点指标
  5. 从返回的 `NodeMetrics` 对象中提取 CPU 和内存使用情况
  6. 计算资源使用率：使用率 = (已使用资源 / 可分配资源) * 100%
  7. 阈值检查：将使用率与配置的阈值比较，超过阈值则标记为过载节点
- 优点：
  - 结构化数据，易于处理
  - 直接集成到代码中，无需执行外部命令
  - 与 Kubernetes 原生集成
  - 更可靠的错误处理

**方式二：可用资源计算（用于备用节点选择）**
- 通过 Kubernetes API 获取节点资源信息
- 优先使用 `node.Status.Allocatable`（可分配资源）
- 如果 `Allocatable` 为空，则使用 `node.Status.Capacity`（总容量）作为备用
- CPU资源计算：使用 `MilliValue()` 方法，单位为毫核（1核 = 1000m）
- 内存资源计算：使用 `Value()` 方法，单位为字节
- 节点选择策略：
  1. 第一优先级：选择CPU可用资源最多的节点
  2. 第二优先级：当CPU相同时，选择内存可用资源最多的节点


#### 5.3.2 过载处理
- 为过载节点添加 `node-role.kubernetes.io/overloaded:NoSchedule` 污点
- 这样可以防止新Pod调度到该节点，同时不影响已有的Pod

### 5.4 故障转移策略

#### 5.4.1 触发条件
- 主用节点状态变为NotReady
- 主用节点资源使用率超过85%且无其他可用主用节点

#### 5.4.2 执行流程
1. 检测到主用节点故障或过载
2. 检查当前可用主用节点数量
3. 如果可用主用节点数量不足，从备用节点中选择资源最充足的节点
4. 为选中的备用节点添加主用节点标签
5. 记录升级操作以便后续恢复时处理

## 6. 部署与使用

### 6.1 安装步骤
1. **安装 CRD**
   ```bash
   kubectl apply -f config/crd/bases/nodeoperator.k8s.io_nodegroups.yaml
   ```

2. **部署 Operator**
   ```bash
   kubectl apply -f config/rbac/role.yaml
   kubectl apply -f config/rbac/role_binding.yaml
   kubectl apply -f config/manager/manager.yaml
   ```

3. **创建 NodeGroup CR**
   ```yaml
   apiVersion: nodeoperator.k8s.io/v1alpha1
   kind: NodeGroup
   metadata:
     name: example-nodegroup
     namespace: default
   spec:
     masterCount: 3
     backupCount: 1
     resourceThreshold: 85
     nodeSelector:
       kubernetes.io/os: linux
     failoverPolicy:
       enabled: true
       timeout: 30
     zones:
     - region-name.az01arm
     - region-name.az02arm
     - region-name.az03arm
   ```

### 6.2 配置选项
- `masterCount`：主用节点数量
- `backupCount`：备用节点数量
- `resourceThreshold`：资源使用率阈值（%）
- `nodeSelector`：节点选择器
- `failoverPolicy.enabled`：是否启用故障转移
- `failoverPolicy.timeout`：故障转移超时时间（秒）
- `zones`：可用区列表
- `initialBackupCount`：各可用区初始备用节点数量

## 7. 监控与日志

### 7.1 监控指标
- 主用节点数量
- 备用节点数量
- 独占节点数量
- 故障节点数量
- 过载节点数量
- 故障转移次数
- 节点升级次数
- 节点降级次数

### 7.2 日志输出
工具会输出详细的日志信息，包括：
- 节点状态变化
- 故障转移操作
- 资源使用率警告
- 节点升级和降级操作
- 错误信息

## 8. 优势与限制

### 8.1 优势
- **简化配置**：通过标签管理替代污点和容忍度，减少配置复杂度
- **自动化**：自动检测节点状态并处理故障转移
- **智能资源管理**：基于资源使用率动态调整节点状态
- **高可用性**：确保主用节点故障时能够快速切换到备用节点
- **灵活性**：可以根据实际需求调整备用节点数量和资源阈值
- **多可用区支持**：实现可用区内的节点平衡，提高集群的容错能力

### 8.2 限制
- **依赖Kubernetes API**：需要足够的权限来管理节点标签和污点
- **Pod配置要求**：需要为所有Pod添加nodeAffinity规则
- **资源检测延迟**：资源使用率检测可能存在一定延迟
- **集群规模限制**：在超大规模集群中，可能需要调整检测间隔以避免API服务器过载

## 9. 总结

本设计方案通过动态管理节点标签和污点，实现了一种智能的主备节点规划方案，能够自动处理节点故障、资源过载和节点恢复等场景。该方案简化了配置复杂度，提高了集群的可用性和资源利用率，为大规模Kubernetes集群的节点管理提供了一种有效的解决方案。

通过定期检测节点状态、智能选择备用节点进行升级、合理管理资源使用率和处理节点恢复等操作，该方案能够确保服务的持续可用性和资源的合理分配，为集群的稳定运行提供有力保障。