#!/bin/bash

# 行情数据服务 Docker 启动脚本
# 使用方法: ./start.sh [start|stop|restart|status|logs|build|clean]

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 服务名称
SERVICE_NAME="quota-data-service"

# 打印带颜色的消息
print_message() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

# 检查 Docker 是否运行
check_docker() {
    if ! docker info > /dev/null 2>&1; then
        print_message $RED "错误: Docker 未运行或无法访问"
        exit 1
    fi
}

# 检查 Docker Compose 是否可用
check_docker_compose() {
    if ! command -v docker-compose > /dev/null 2>&1; then
        print_message $RED "错误: docker-compose 未安装"
        exit 1
    fi
}

# 启动服务
start_services() {
    print_message $BLUE "启动行情数据服务..."
    docker-compose up -d
    print_message $GREEN "服务启动完成！"
    
    # 等待服务就绪
    print_message $YELLOW "等待服务就绪..."
    sleep 10
    
    # 检查服务状态
    check_services
}

# 停止服务
stop_services() {
    print_message $YELLOW "停止行情数据服务..."
    docker-compose down
    print_message $GREEN "服务已停止"
}

# 重启服务
restart_services() {
    print_message $YELLOW "重启行情数据服务..."
    docker-compose restart
    print_message $GREEN "服务重启完成"
}

# 查看服务状态
show_status() {
    print_message $BLUE "服务状态:"
    docker-compose ps
    
    echo ""
    print_message $BLUE "资源使用情况:"
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}"
}

# 查看日志
show_logs() {
    local service=${1:-$SERVICE_NAME}
    print_message $BLUE "查看 $service 的日志 (按 Ctrl+C 退出):"
    docker-compose logs -f --tail=100 $service
}

# 构建镜像
build_images() {
    print_message $BLUE "构建 Docker 镜像..."
    docker-compose build --no-cache
    print_message $GREEN "镜像构建完成"
}

# 清理资源
clean_resources() {
    print_message $YELLOW "清理 Docker 资源..."
    
    read -p "确定要删除所有容器、网络和镜像吗？(y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker-compose down --remove-orphans -v
        docker system prune -f
        print_message $GREEN "清理完成"
    else
        print_message $YELLOW "取消清理"
    fi
}

# 检查服务健康状态
check_services() {
    print_message $BLUE "检查服务健康状态..."
    
    # 检查 HTTP 服务
    if curl -s http://localhost:8080/healthz > /dev/null; then
        print_message $GREEN "✓ HTTP 服务正常 (http://localhost:8080)"
    else
        print_message $RED "✗ HTTP 服务异常"
    fi
    
    # 检查 ClickHouse
    if curl -s http://localhost:8123/ping > /dev/null; then
        print_message $GREEN "✓ ClickHouse 正常 (http://localhost:8123)"
    else
        print_message $RED "✗ ClickHouse 异常"
    fi
    
    # 检查端口占用
    echo ""
    print_message $BLUE "端口占用情况:"
    netstat -tulpn | grep -E ':(8080|9090|9000|8123)' || echo "未检测到相关端口"
}

# 显示帮助信息
show_help() {
    cat << EOF
行情数据服务 Docker 管理脚本

使用方法: $0 [命令]

命令:
  start     启动所有服务
  stop      停止所有服务
  restart   重启所有服务
  status    显示服务状态
  logs      查看服务日志
  build     重新构建镜像
  clean     清理所有资源
  help      显示此帮助信息

示例:
  $0 start          # 启动服务
  $0 logs           # 查看主服务日志
  $0 logs clickhouse # 查看 ClickHouse 日志
  $0 status         # 查看服务状态

服务访问地址:
  HTTP API:     http://localhost:8080
  gRPC API:     localhost:9090
  ClickHouse:   localhost:9000 (TCP), localhost:8123 (HTTP)
  ClickHouse UI: http://localhost:8123

EOF
}

# 主函数
main() {
    local command=${1:-help}
    
    # 检查依赖
    check_docker
    check_docker_compose
    
    case $command in
        start)
            start_services
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_services
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs $2
            ;;
        build)
            build_images
            ;;
        clean)
            clean_resources
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_message $RED "未知命令: $command"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# 运行主函数
main "$@" 