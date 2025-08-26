# Docker 部署指南

本指南介绍如何使用 Docker 和 Docker Compose 部署行情数据服务。

## 快速开始

### 1. 启动所有服务

```bash
# 启动核心服务（行情数据服务 + ClickHouse）
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f quota-data-service
```

### 2. 验证服务

```bash
# 检查 HTTP 健康状态
curl http://localhost:8080/healthz

# 检查 gRPC 服务（需要 grpcurl）
grpcurl -plaintext localhost:9090 list

# 检查 ClickHouse 连接
curl http://localhost:8123/ping
```

## 服务配置

### 行情数据服务 (quota-data-service)

- **HTTP API**: http://localhost:8080
- **gRPC API**: localhost:9090
- **健康检查**: http://localhost:8080/healthz

### ClickHouse 数据库

- **Native TCP**: localhost:9000
- **HTTP**: http://localhost:8123
- **Interserver**: localhost:9009

## 环境变量配置

可以通过环境变量或 `.env` 文件自定义配置：

```bash
# 创建 .env 文件
cat > .env << EOF
# 服务配置
INGEST_ADDR=:8080
GRPC_ADDR=:9090

# ClickHouse 配置
CH_ADDR=tcp://clickhouse:9000
CH_USER=default
CH_PASSWORD=
CH_DATABASE=crypto_market
CH_TABLE=raw_ticks

# 批处理配置
FLUSH_MS=10
MAX_BATCH=800
MAX_QUEUE=20000
INSERT_TIMEOUT_MS=1500
EOF
```

## 高级用法

### 1. 仅启动核心服务

```bash
# 启动行情数据服务和 ClickHouse
docker-compose up -d quota-data-service clickhouse
```

### 2. 调试模式

```bash
# 启动调试服务（包含 ClickHouse 客户端）
docker-compose --profile debug up -d

# 连接到 ClickHouse 客户端
docker-compose exec clickhouse-client clickhouse-client --host clickhouse --port 9000
```

### 3. 测试模式

```bash
# 启动测试服务（包含 gRPC 测试客户端）
docker-compose --profile test up -d

# 查看测试结果
docker-compose logs grpc-test-client
```

### 4. 数据持久化

ClickHouse 数据会自动持久化到 Docker volumes：

```bash
# 查看 volumes
docker volume ls

# 备份数据
docker run --rm -v clickhouse-data:/data -v $(pwd):/backup \
    alpine tar czf /backup/clickhouse-backup.tar.gz -C /data .

# 恢复数据
docker run --rm -v clickhouse-data:/data -v $(pwd):/backup \
    alpine tar xzf /backup/clickhouse-backup.tar.gz -C /data
```

## 监控和日志

### 查看服务状态

```bash
# 查看所有服务状态
docker-compose ps

# 查看特定服务日志
docker-compose logs -f quota-data-service
docker-compose logs -f clickhouse

# 查看实时日志
docker-compose logs -f --tail=100
```

### 健康检查

服务包含内置健康检查：

```bash
# 检查服务健康状态
docker-compose ps

# 手动健康检查
docker exec quota-data-service curl -f http://localhost:8080/healthz
docker exec clickhouse wget --no-verbose --tries=1 --spider http://localhost:8123/ping
```

## 性能调优

### 资源限制

在 `docker-compose.yml` 中调整资源限制：

```yaml
deploy:
  resources:
    limits:
      memory: 2G      # 内存限制
      cpus: '2.0'     # CPU 限制
    reservations:
      memory: 1G      # 内存预留
      cpus: '1.0'     # CPU 预留
```

### ClickHouse 优化

调整 ClickHouse 配置以优化性能：

```xml
<!-- clickhouse-config/01-crypto-market.xml -->
<profiles>
    <default>
        <max_memory_usage>20000000000</max_memory_usage>
        <max_bytes_before_external_group_by>40000000000</max_bytes_before_external_group_by>
        <max_bytes_before_external_sort>40000000000</max_bytes_before_external_sort>
    </default>
</profiles>
```

## 故障排除

### 常见问题

1. **服务启动失败**
   ```bash
   # 查看详细日志
   docker-compose logs quota-data-service
   
   # 检查端口占用
   netstat -tulpn | grep :8080
   netstat -tulpn | grep :9090
   ```

2. **ClickHouse 连接失败**
   ```bash
   # 检查 ClickHouse 状态
   docker-compose exec clickhouse clickhouse-client --query "SELECT 1"
   
   # 检查网络连接
   docker-compose exec quota-data-service ping clickhouse
   ```

3. **内存不足**
   ```bash
   # 查看资源使用情况
   docker stats
   
   # 调整资源限制
   # 编辑 docker-compose.yml 中的 resources 配置
   ```

### 日志分析

```bash
# 查看错误日志
docker-compose logs quota-data-service | grep ERROR

# 查看性能日志
docker-compose logs quota-data-service | grep "flush\|batch"

# 实时监控
docker-compose logs -f --tail=50 quota-data-service
```

## 生产环境部署

### 1. 安全配置

```bash
# 设置强密码
export CH_PASSWORD=your_secure_password

# 限制网络访问
# 编辑 docker-compose.yml 中的 networks 配置
```

### 2. 备份策略

```bash
# 创建备份脚本
cat > backup.sh << 'EOF'
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
docker run --rm -v clickhouse-data:/data -v $(pwd)/backups:/backup \
    alpine tar czf /backup/clickhouse-backup-${DATE}.tar.gz -C /data .
echo "Backup completed: clickhouse-backup-${DATE}.tar.gz"
EOF

chmod +x backup.sh
```

### 3. 监控集成

集成 Prometheus 和 Grafana 进行监控：

```yaml
# 添加到 docker-compose.yml
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    networks:
      - quota-network

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    networks:
      - quota-network
```

## 清理和重置

### 停止服务

```bash
# 停止所有服务
docker-compose down

# 停止并删除 volumes（数据会丢失）
docker-compose down -v
```

### 重新构建

```bash
# 重新构建镜像
docker-compose build --no-cache

# 重新启动服务
docker-compose up -d
```

### 完全清理

```bash
# 停止所有服务
docker-compose down

# 删除所有相关容器和网络
docker-compose down --remove-orphans

# 删除 volumes（谨慎操作，数据会丢失）
docker volume rm quota_data_service_clickhouse-data
docker volume rm quota_data_service_clickhouse-logs

# 删除镜像
docker rmi quota_data_service-quota-data-service
docker rmi quota_data_service-grpc-test-client
```

## 总结

使用 Docker Compose 可以轻松部署和管理行情数据服务：

- ✅ 一键启动所有服务
- ✅ 自动健康检查
- ✅ 数据持久化
- ✅ 资源管理
- ✅ 易于扩展和维护

建议在生产环境中：
1. 配置适当的资源限制
2. 设置定期备份
3. 集成监控系统
4. 配置日志轮转
5. 使用私有镜像仓库 