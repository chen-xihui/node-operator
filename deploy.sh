#!/bin/bash

# Node-Operator 部署脚本
set -e

# 配置变量
NAMESPACE="node-operator-system"
IMAGE_NAME="node-operator"
IMAGE_TAG="v1.0.0"
REGISTRY="your-registry"  # 修改为你的镜像仓库

# 创建命名空间
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# 构建镜像
echo "构建 Docker 镜像..."
docker build -t $REGISTRY/$IMAGE_NAME:$IMAGE_TAG .

# 推送镜像
echo "推送镜像到仓库..."
docker push $REGISTRY/$IMAGE_NAME:$IMAGE_TAG

# 更新部署配置中的镜像地址
sed -i.bak "s|controller:latest|$REGISTRY/$IMAGE_NAME:$IMAGE_TAG|g" config/default/manager_image_patch.yaml

# 部署 CRD
echo "部署 CRD..."
kubectl apply -f config/crd/bases/

# 部署 RBAC
echo "部署 RBAC..."
kubectl apply -f config/rbac/

# 部署控制器
echo "部署控制器..."
kubectl apply -k config/default/

# 等待部署完成
echo "等待控制器启动..."
kubectl -n $NAMESPACE wait --for=condition=ready pod -l control-plane=controller-manager --timeout=300s

echo "✅ Node-Operator 部署完成！"
echo "检查状态: kubectl -n $NAMESPACE get pods"
echo "查看日志: kubectl -n $NAMESPACE logs -l control-plane=controller-manager"