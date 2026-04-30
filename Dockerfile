# 构建阶段
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

# 复制 go.mod 和 go.sum
COPY go.mod go.mod
COPY go.sum go.sum

# 下载依赖
RUN go mod download

# 复制源代码
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY internal/ internal/

# 构建
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/manager/main.go

# 运行阶段
FROM alpine:3.19

WORKDIR /

# 复制二进制文件
COPY --from=builder /workspace/manager .

# 设置用户
USER 65532:65532

ENTRYPOINT ["/manager"]