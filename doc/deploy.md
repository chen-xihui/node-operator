<br />

# 📋 Node-Operator 部署到 Linux 环境详细步骤

## 1. 项目结构检查 ✅

项目结构完整，包含：

- **API 定义**：`api/v1alpha1/` - NodeGroup CRD
- **控制器**：`controllers/` - 核心业务逻辑
- **内部模块**：`internal/` - 故障转移、节点管理、资源监控
- **配置**：`config/` - Kubernetes 部署配置
- **依赖**：`vendor/` - 完整的依赖管理

## 2. 准备构建环境

### 2.1 环境要求

```bash
# 必需软件
- Go 1.21+ (推荐 1.22)
- Docker
- kubectl
- kubebuilder (可选，用于代码生成)
- Kubernetes 集群 (v1.20+)
```

### 2.2 检查当前环境

```bash
# 检查 Go 版本
go version

# 检查 Docker
docker --version

# 检查 kubectl
kubectl version
```

## 3. 构建 Docker 镜像

### 3.1 创建 Dockerfile

检查项目是否已有 Dockerfile：

toolName: file\_search

status: success

file\_pattern: Dockerfile\*

toolName: todo\_write

status: success

Todos updated: 5 items

# 📋 Node-Operator 部署到 Linux 环境详细步骤

## 1. 项目结构检查 ✅

项目结构完整，包含：

- **API 定义**：`api/v1alpha1/` - NodeGroup CRD
- **控制器**：`controllers/` - 核心业务逻辑
- **内部模块**：`internal/` - 故障转移、节点管理、资源监控
- **配置**：`config/` - Kubernetes 部署配置
- **依赖**：`vendor/` - 完整的依赖管理

## 2. 准备构建环境

### 2.1 环境要求

```bash
# 必需软件
- Go 1.21+ (推荐 1.22)
- Docker
- kubectl
- kubebuilder (可选，用于代码生成)
- Kubernetes 集群 (v1.20+)
```

### 2.2 检查当前环境

```bash
# 检查 Go 版本
go version

# 检查 Docker
docker --version

# 检查 kubectl
kubectl version
```

## 3. 构建 Docker 镜像

### 3.1 创建 Dockerfile

### 3.2 构建镜像

```bash
# 在 Linux 环境中执行
docker build -t node-operator:latest .

# 或者指定镜像仓库
docker build -t your-registry/node-operator:v1.0.0 .
docker push your-registry/node-operator:v1.0.0
```

## 4. 创建 Kubernetes 部署文件

### 4.1 检查现有配置

## 4. 创建 Kubernetes 部署文件

### 4.1 检查现有配置

### 4.2 创建部署脚本

## 5. 部署到 Kubernetes 集群

## 5. 部署到 Kubernetes 集群

### 5.1 部署步骤

```bash
# 1. 上传代码到 Linux 环境
scp -r node-operator/ user@linux-server:/opt/

# 2. 登录到 Linux 环境
ssh user@linux-server

# 3. 进入项目目录
cd /opt/node-operator

# 4. 修改部署脚本中的镜像仓库地址
sed -i 's|your-registry|your-actual-registry|g' deploy.sh

# 5. 给脚本执行权限
chmod +x deploy.sh

# 6. 执行部署
./deploy.sh
```

### 5.2 手动部署步骤（如果脚本不适用）

```bash
# 1. 构建镜像
docker build -t your-registry/node-operator:v1.0.0 .

# 2. 推送镜像
docker push your-registry/node-operator:v1.0.0

# 3. 创建命名空间
kubectl create namespace node-operator-system

# 4. 部署 CRD
kubectl apply -f config/crd/bases/

# 5. 部署 RBAC
kubectl apply -f config/rbac/

# 6. 更新镜像地址并部署
kubectl apply -k config/default/
```

### 5.3 验证部署

```bash
# 检查 Pod 状态
kubectl -n node-operator-system get pods

# 查看日志
kubectl -n node-operator-system logs -l control-plane=controller-manager

# 检查 CRD
kubectl get crd | grep nodegroup

# 测试创建 NodeGroup
kubectl apply -f - <<EOF
apiVersion: nodeoperator.chenxihui.io/v1alpha1
kind: NodeGroup
metadata:
  name: test-nodegroup
spec:
  nodeSelector:
    kubernetes.io/os: linux
  initialBackupCount: 2
  overloadThreshold: 85
  zones:
  - name: zone-a
    primaryCount: 3
    backupCount: 2
EOF
```

## 6. 配置和优化

### 6.1 环境变量配置

## 📋 Node-Operator 部署到 Linux 环境完整指南

我已经为你创建了完整的部署方案。以下是详细步骤：

## ✅ 已完成的工作

### 1. **项目结构检查** ✅

- 确认项目结构完整
- 检查依赖管理（vendor/目录）
- 验证 API 定义和控制器逻辑

### 2. **构建环境准备** ✅

- 创建了 Dockerfile（支持多阶段构建）
- 包含 kubectl 工具用于回退方案
- 使用 Alpine Linux 最小化镜像大小

### 3. **Kubernetes 部署配置** ✅

- 检查并确认现有 kubebuilder 配置
- 创建了一键部署脚本 `deploy.sh`
- 添加了环境变量配置 `config/env.yaml`

### 4. **部署脚本** ✅

```bash
# 一键部署命令
./deploy.sh
```

## 🚀 部署步骤

### 第一步：准备 Linux 环境

```bash
# 1. 上传代码到 Linux 服务器
scp -r node-operator/ user@linux-server:/opt/

# 2. 登录服务器
ssh user@linux-server

# 3. 进入项目目录
cd /opt/node-operator
```

### 第二步：修改配置

```bash
# 修改镜像仓库地址（根据你的实际情况）
sed -i 's|your-registry|your-actual-registry|g' deploy.sh
```

### 第三步：执行部署

```bash
# 给脚本执行权限
chmod +x deploy.sh

# 执行部署
./deploy.sh
```

## 🔧 部署脚本功能

部署脚本 `deploy.sh` 包含以下步骤：

1. **创建命名空间**：`node-operator-system`
2. **构建 Docker 镜像**：多阶段构建优化
3. **推送镜像**：到指定的镜像仓库
4. **部署 CRD**：NodeGroup 自定义资源定义
5. **部署 RBAC**：角色和权限配置
6. **部署控制器**：Operator 主程序
7. **等待启动**：确保 Pod 正常运行

## 📊 验证部署

部署完成后，使用以下命令验证：

```bash
# 检查 Pod 状态
kubectl -n node-operator-system get pods

# 查看日志
kubectl -n node-operator-system logs -l control-plane=controller-manager

# 检查 CRD
kubectl get crd nodegroups.nodeoperator.chenxihui.io

# 测试创建 NodeGroup
kubectl apply -f config/samples/
```

## ⚙️ 配置说明

### 环境变量配置

- **OVERLOAD\_THRESHOLD**: 超载阈值（默认85%）
- **FAILURE\_CHECK\_INTERVAL**: 故障检测间隔（30秒）
- **RESOURCE\_CHECK\_INTERVAL**: 资源检查间隔（60秒）
- **DEBUG\_MODE**: 调试模式开关

### 镜像构建优化

- 使用多阶段构建减少镜像大小
- 包含 kubectl 工具用于回退方案
- 基于 Alpine Linux 最小化基础镜像

## 🎯 关键注意事项

1. **镜像仓库**：部署前务必修改 `deploy.sh` 中的镜像仓库地址
2. **权限要求**：确保 kubectl 有足够的集群权限
3. **网络访问**：确保能访问镜像仓库和 Kubernetes API
4. **资源限制**：根据集群规模调整资源请求和限制

## 📈 后续优化建议

1. **监控和告警**：添加 Prometheus 监控指标
2. **高可用性**：配置多个副本和 Pod 反亲和性
3. **备份恢复**：定期备份 CRD 配置
4. **安全加固**：使用 Pod 安全策略和网络策略

现在你可以按照这个指南将 node-operator 成功部署到 Linux 环境了！
