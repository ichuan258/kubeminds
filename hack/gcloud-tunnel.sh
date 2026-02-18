#!/bin/bash
# GCloud K8s API Server SSH 隧道管理脚本
# 用途: 建立/关闭 SSH 隧道以安全访问自建在 GCE 上的 K8s 集群

set -e

# GCloud 配置 (支持环境变量覆盖)
INSTANCE="${GCLOUD_INSTANCE:-}"
ZONE="${GCLOUD_ZONE:-}"
REMOTE_PORT="${GCLOUD_REMOTE_PORT:-6443}"
LOCAL_PORT="${GCLOUD_LOCAL_PORT:-6443}"
REMOTE_HOST="${GCLOUD_REMOTE_HOST:-}"

# 如果未设置，从 .env 文件加载
if [ -f "$(dirname "$0")/../.env.local" ]; then
    source "$(dirname "$0")/../.env.local"
fi

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 帮助信息
usage() {
    cat << EOF
用法: $0 [命令]

命令:
  up         启动 SSH 隧道
  down       关闭 SSH 隧道
  status     检查隧道状态
  restart    重启隧道
  verify     验证隧道和集群连接
  help       显示此帮助信息

示例:
  $0 up      # 启动隧道
  $0 status  # 检查隧道是否运行

必需的环境变量 (需要在 .env.local 中设置或命令行传递):
  GCLOUD_INSTANCE    GCE 实例名 (例: instance-20260215-051955)
  GCLOUD_ZONE        GCE 区域 (例: asia-east1-b)
  GCLOUD_REMOTE_HOST K8s API 内部 IP (例: 10.140.0.2)

可选的环境变量:
  GCLOUD_REMOTE_PORT K8s API 端口 (默认: 6443)
  GCLOUD_LOCAL_PORT  本地转发端口 (默认: 6443)

配置文件示例 (.env.local):
  GCLOUD_INSTANCE=instance-20260215-051955
  GCLOUD_ZONE=asia-east1-b
  GCLOUD_REMOTE_HOST=10.140.0.2

EOF
    exit 0
}

# 验证必需的环境变量
validate_config() {
    local missing=0

    if [ -z "$INSTANCE" ]; then
        echo -e "${RED}❌ 错误: GCLOUD_INSTANCE 未设置${NC}"
        missing=1
    fi

    if [ -z "$ZONE" ]; then
        echo -e "${RED}❌ 错误: GCLOUD_ZONE 未设置${NC}"
        missing=1
    fi

    if [ -z "$REMOTE_HOST" ]; then
        echo -e "${RED}❌ 错误: GCLOUD_REMOTE_HOST 未设置${NC}"
        missing=1
    fi

    if [ $missing -eq 1 ]; then
        echo ""
        echo -e "${BLUE}请设置环境变量或在 .env.local 中配置:${NC}"
        echo "  export GCLOUD_INSTANCE=instance-name"
        echo "  export GCLOUD_ZONE=zone-name"
        echo "  export GCLOUD_REMOTE_HOST=10.140.0.2"
        echo ""
        usage
    fi
}

# 获取隧道进程 PID
get_tunnel_pid() {
    # 查找 SSH 隧道进程 (更可靠的方式)
    lsof -i :6443 2>/dev/null | grep ssh | awk '{print $2}' | head -1 || true
}

# 启动隧道
tunnel_up() {
    if [ -n "$(get_tunnel_pid)" ]; then
        echo -e "${BLUE}ℹ️  隧道已在运行 (PID: $(get_tunnel_pid))${NC}"
        return 0
    fi

    echo -e "${BLUE}⏳ 正在启动 SSH 隧道...${NC}"
    gcloud compute ssh "$INSTANCE" --zone="$ZONE" -- \
        -L ${LOCAL_PORT}:${REMOTE_HOST}:${REMOTE_PORT} -N -f

    sleep 2

    if [ -n "$(get_tunnel_pid)" ]; then
        echo -e "${GREEN}✅ 隧道已启动${NC}"
        echo -e "${GREEN}   - 本地: 127.0.0.1:${LOCAL_PORT}${NC}"
        echo -e "${GREEN}   - 远程: ${REMOTE_HOST}:${REMOTE_PORT}${NC}"
        echo -e "${GREEN}   - PID: $(get_tunnel_pid)${NC}"
        return 0
    else
        echo -e "${RED}❌ 隧道启动失败${NC}"
        return 1
    fi
}

# 关闭隧道
tunnel_down() {
    local pid=$(get_tunnel_pid)
    if [ -z "$pid" ]; then
        echo -e "${BLUE}ℹ️  隧道未运行${NC}"
        return 0
    fi

    echo -e "${BLUE}⏳ 正在关闭隧道 (PID: $pid)...${NC}"
    kill "$pid" 2>/dev/null || true

    sleep 1

    if [ -z "$(get_tunnel_pid)" ]; then
        echo -e "${GREEN}✅ 隧道已关闭${NC}"
        return 0
    else
        echo -e "${RED}❌ 隧道关闭失败${NC}"
        return 1
    fi
}

# 检查隧道状态
tunnel_status() {
    local pid=$(get_tunnel_pid)
    if [ -n "$pid" ]; then
        echo -e "${GREEN}✅ 隧道正在运行${NC}"
        echo -e "   PID: $pid"
        # 验证连接
        if timeout 3 curl -k https://127.0.0.1:6443/api 2>/dev/null | grep -q '"apiVersion"'; then
            echo -e "${GREEN}   连接: 正常${NC}"
        else
            echo -e "${RED}   连接: 异常${NC}"
        fi
        return 0
    else
        echo -e "${RED}❌ 隧道未运行${NC}"
        return 1
    fi
}

# 验证 kubeconfig
verify_kubeconfig() {
    local kubeconfig="${HOME}/.kube/gcloud-k8s-config"
    if [ ! -f "$kubeconfig" ]; then
        echo -e "${RED}❌ kubeconfig 不存在: $kubeconfig${NC}"
        return 1
    fi

    echo -e "${GREEN}✅ kubeconfig 存在: $kubeconfig${NC}"

    # 验证是否能访问
    export KUBECONFIG="$kubeconfig"
    if kubectl cluster-info &>/dev/null; then
        echo -e "${GREEN}✅ 集群连接: 正常${NC}"
        kubectl cluster-info
        return 0
    else
        echo -e "${RED}❌ 集群连接: 异常${NC}"
        return 1
    fi
}

# 主函数
main() {
    local cmd="${1:-help}"

    case "$cmd" in
        up)
            validate_config
            tunnel_up
            ;;
        down)
            tunnel_down
            ;;
        status)
            tunnel_status
            ;;
        restart)
            validate_config
            tunnel_down
            sleep 1
            tunnel_up
            ;;
        verify)
            validate_config
            tunnel_status && verify_kubeconfig
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            echo -e "${RED}❌ 未知命令: $cmd${NC}"
            usage
            ;;
    esac
}

main "$@"
