# 使用 Go 官方镜像作为构建阶段
FROM golang:1.23 AS builder

# 设置Go环境变量，使用国内代理
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
ENV CGO_ENABLED=0
ENV GOOS=linux

# 安装 protobuf 编译器
RUN apt-get update && apt-get install -y \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 安装 protobuf Go 插件
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 复制源代码
COPY . .

# 生成 protobuf 代码
RUN protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/quota_service.proto

# 编译二进制
RUN go build -a -installsuffix cgo -o quota_data_service .

# 运行阶段 - 使用轻量级镜像
FROM debian:bullseye-slim

# 安装运行时依赖
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 创建非root用户
RUN groupadd -r appuser && useradd -r -g appuser appuser

WORKDIR /app

# 复制编译好的二进制文件
COPY --from=builder /app/quota_data_service .

# 设置权限
RUN chown -R appuser:appuser /app
USER appuser

# 暴露端口
EXPOSE 8080 9090

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

# 启动命令
CMD ["./quota_data_service"]
