# node-operator

## 项目简介

node-operator 是一个 Kubernetes Operator，用于自动化管理集群中的主备节点，实现智能的故障转移和资源管理。

### 核心功能

- **主备节点管理**：自动分配和管理主用节点和备用节点
- **故障转移**：当主用节点故障时，自动从备用节点中选择合适的节点进行升级
- **资源管理**：监控节点资源使用情况，对过载节点添加污点防止新Pod调度
- **多可用区支持**：实现可用区内的节点平衡，提高集群的容错能力
- **节点恢复处理**：当故障节点恢复后，智能调整节点角色

## 架构设计

### 核心组件

- **NodeGroup 控制器**：协调所有管理器，实现主要的 reconciliation 逻辑
- **节点管理器**：管理节点标签和污点，处理节点状态变化
- **可用区管理器**：管理可用区分类，处理跨可用区兜底逻辑
- **资源管理器**：监控节点资源使用情况，处理过载节点
- **故障转移处理器**：处理主用节点故障和过载的情况
- **节点控制器**：监控节点状态变化并更新 NodeGroup 状态

### API 定义

node-operator 定义了一个 `NodeGroup` 自定义资源，用于配置和管理节点组：

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
  initialBackupCount:
    region-name.az01arm: 1
    region-name.az02arm: 1
    region-name.az03arm: 1
```

## 安装指南

### 前提条件

- Kubernetes 集群 (v1.20+)
- kubectl 命令行工具
- metrics-server（用于资源使用率监控）

### 安装步骤

1. **安装 CRD**
   ```bash
   kubectl apply -f config/crd/bases/nodeoperator.k8s.io_nodegroups.yaml
   ```

2. **部署 RBAC 权限**
   ```bash
   kubectl apply -f config/rbac/role.yaml
   kubectl apply -f config/rbac/role_binding.yaml
   ```

3. **部署 Operator**
   ```bash
   kubectl apply -f config/manager/manager.yaml
   ```

4. **创建 NodeGroup 资源**
   ```bash
   kubectl apply -f config/samples/nodeoperator_v1alpha1_nodegroup.yaml
   ```

## 使用方法

### 配置说明

| 配置项 | 描述 | 默认值 |
|-------|------|-------|
| `masterCount` | 主用节点数量 | - |
| `backupCount` | 备用节点数量 | - |
| `resourceThreshold` | 资源使用率阈值（%） | 85 |
| `nodeSelector` | 节点选择器 | {} |
| `failoverPolicy.enabled` | 是否启用故障转移 | true |
| `failoverPolicy.timeout` | 故障转移超时时间（秒） | 30 |
| `zones` | 可用区列表 | [] |
| `initialBackupCount` | 各可用区初始备用节点数量 | {} |

### 监控与日志

#### 监控指标

- 主用节点数量
- 备用节点数量
- 故障节点数量
- 过载节点数量
- 故障转移次数
- 节点升级次数
- 节点降级次数

#### 日志输出

Operator 会输出详细的日志信息，包括：
- 节点状态变化
- 故障转移操作
- 资源使用率警告
- 节点升级和降级操作
- 错误信息

### 最佳实践

1. **Pod 配置**：为所有 Pod 添加 nodeAffinity 规则，确保 Pod 只调度到主用节点
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

2. **资源配置**：根据集群规模和工作负载特点，合理设置 `masterCount` 和 `backupCount`

3. **可用区配置**：在多可用区环境中，确保每个可用区都有足够的备用节点

4. **资源阈值**：根据节点性能和工作负载特点，调整 `resourceThreshold` 值

## 故障排查

### 常见问题

1. **节点未被识别**：检查节点是否满足 NodeGroup 的 nodeSelector 条件

2. **故障转移未触发**：检查节点状态是否为 NotReady，以及 failoverPolicy 是否启用

3. **资源使用率检测失败**：确保 metrics-server 已正确安装并运行

4. **权限问题**：检查 Operator 是否有足够的权限管理节点标签和污点

### 日志查看

```bash
kubectl logs deployment/node-operator-controller-manager -n node-operator-system
```

## 开发指南

### 本地开发

1. **克隆代码**
   ```bash
   git clone https://github.com/your-org/node-operator.git
   cd node-operator
   ```

2. **安装依赖**
   ```bash
   go mod download
   ```

3. **运行测试**
   ```bash
   go test ./...
   ```

4. **本地运行 Operator**
   ```bash
   make run
   ```

### 构建与部署

1. **构建镜像**
   ```bash
   make docker-build IMG=your-registry/node-operator:v1.0.0
   ```

2. **推送镜像**
   ```bash
   make docker-push IMG=your-registry/node-operator:v1.0.0
   ```

3. **部署到集群**
   ```bash
   make deploy IMG=your-registry/node-operator:v1.0.0
   ```

## 版本历史

- v1.0.0：初始版本，支持基本的主备节点管理和故障转移

## 贡献指南

欢迎贡献代码、报告问题或提出建议！请查看 [CONTRIBUTING.md](CONTRIBUTING.md) 文件了解详细信息。

## 许可证

本项目采用 Apache 2.0 许可证。请查看 [LICENSE](LICENSE) 文件了解详细信息。