#!/bin/bash
# E2E 测试脚本 - 在真实 K8s 集群中验证诊断流程
# 用法: ./hack/e2e-test.sh [scenario]
# 场景: oom, imagepull, crashloop, notready, all

set -e

NAMESPACE="kube-minds-test"
SCENARIO="${1:-all}"

# 颜色输出
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 初始化命名空间
init_namespace() {
    log_info "初始化命名空间 $NAMESPACE"
    kubectl create namespace $NAMESPACE 2>/dev/null || true
    kubectl label namespace $NAMESPACE diagnosis=enabled 2>/dev/null || true
}

# 清理测试资源
cleanup() {
    log_info "清理测试资源"
    kubectl delete pods -n $NAMESPACE --all 2>/dev/null || true
    kubectl delete diagnosistasks -n $NAMESPACE --all 2>/dev/null || true
}

# 场景 1: OOM Pod
test_oom() {
    log_info "测试场景 1: OOM Pod 诊断"

    # 创建会导致 OOM 的 Pod
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-oom-pod
  namespace: $NAMESPACE
  labels:
    test: oom
spec:
  restartPolicy: Never
  containers:
  - name: memory-hog
    image: busybox:latest
    command: ["sh", "-c"]
    args: ["yes | head -c 1000000000 > /dev/null || true"]
    resources:
      limits:
        memory: "64Mi"
      requests:
        memory: "32Mi"
EOF

    log_info "等待 Pod 进入 OOMKilled 状态..."
    sleep 15

    # 验证 Pod 状态
    POD_STATUS=$(kubectl get pod test-oom-pod -n $NAMESPACE -o jsonpath='{.status.containerStatuses[0].lastState.oom}')
    if [ "$POD_STATUS" == "true" ]; then
        log_success "Pod 已进入 OOM 状态"
    else
        log_info "Pod 状态: $(kubectl get pod test-oom-pod -n $NAMESPACE -o jsonpath='{.status.phase}')"
    fi

    # 创建诊断任务
    kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-oom-test
  namespace: $NAMESPACE
spec:
  podRef:
    name: test-oom-pod
    namespace: $NAMESPACE
  approved: false
  maxSteps: 5
EOF

    log_info "诊断任务已创建，等待完成..."
    wait_for_diagnosis "diagnose-oom-test"

    # 验证结果
    REPORT=$(kubectl get diagnosistask diagnose-oom-test -n $NAMESPACE -o jsonpath='{.status.report}')
    if echo "$REPORT" | grep -q "OOM\|memory"; then
        log_success "OOM 诊断成功: $REPORT"
    else
        log_error "OOM 诊断失败，报告: $REPORT"
    fi
}

# 场景 2: ImagePullBackOff
test_imagepull() {
    log_info "测试场景 2: ImagePullBackOff 诊断"

    kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-image-pull-pod
  namespace: $NAMESPACE
  labels:
    test: imagepull
spec:
  restartPolicy: Never
  containers:
  - name: nonexistent-image
    image: nonexistent.registry.invalid/app:v1-notexist
    imagePullPolicy: Always
EOF

    log_info "等待 Pod 进入 ImagePullBackOff 状态..."
    sleep 10

    # 创建诊断任务
    kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-imagepull-test
  namespace: $NAMESPACE
spec:
  podRef:
    name: test-image-pull-pod
    namespace: $NAMESPACE
  approved: false
  maxSteps: 5
EOF

    log_info "诊断任务已创建，等待完成..."
    wait_for_diagnosis "diagnose-imagepull-test"

    REPORT=$(kubectl get diagnosistask diagnose-imagepull-test -n $NAMESPACE -o jsonpath='{.status.report}')
    if echo "$REPORT" | grep -q "image\|pull\|registry"; then
        log_success "ImagePull 诊断成功: $REPORT"
    else
        log_error "ImagePull 诊断失败，报告: $REPORT"
    fi
}

# 场景 3: CrashLoop
test_crashloop() {
    log_info "测试场景 3: CrashLoopBackOff 诊断"

    kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-crashloop-pod
  namespace: $NAMESPACE
  labels:
    test: crashloop
spec:
  restartPolicy: Always
  containers:
  - name: crash-app
    image: busybox:latest
    command: ["sh", "-c"]
    args: ["echo 'Starting...' && exit 1"]
EOF

    log_info "等待 Pod 进入 CrashLoopBackOff 状态..."
    sleep 15

    # 创建诊断任务
    kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-crashloop-test
  namespace: $NAMESPACE
spec:
  podRef:
    name: test-crashloop-pod
    namespace: $NAMESPACE
  approved: false
  maxSteps: 5
EOF

    log_info "诊断任务已创建，等待完成..."
    wait_for_diagnosis "diagnose-crashloop-test"

    REPORT=$(kubectl get diagnosistask diagnose-crashloop-test -n $NAMESPACE -o jsonpath='{.status.report}')
    if echo "$REPORT" | grep -q "crash\|restart\|exit"; then
        log_success "CrashLoop 诊断成功: $REPORT"
    else
        log_error "CrashLoop 诊断失败，报告: $REPORT"
    fi
}

# 等待诊断完成
wait_for_diagnosis() {
    local task_name="$1"
    local timeout=120
    local elapsed=0

    while [ $elapsed -lt $timeout ]; do
        PHASE=$(kubectl get diagnosistask "$task_name" -n $NAMESPACE -o jsonpath='{.status.phase}' 2>/dev/null)

        if [ "$PHASE" == "Completed" ] || [ "$PHASE" == "Failed" ]; then
            log_info "诊断任务状态: $PHASE"
            return 0
        fi

        log_info "诊断进行中 (状态: $PHASE, 已等待: ${elapsed}s)"
        sleep 10
        elapsed=$((elapsed + 10))
    done

    log_error "诊断超时 (${timeout}s)"
    return 1
}

# 显示诊断结果
show_results() {
    log_info "=== 诊断结果汇总 ==="

    kubectl get diagnosistasks -n $NAMESPACE -o wide

    for task in $(kubectl get diagnosistasks -n $NAMESPACE -o jsonpath='{.items[*].metadata.name}'); do
        log_info "诊断任务: $task"
        kubectl describe diagnosistask "$task" -n $NAMESPACE | grep -E "Phase|Report|Status"
    done
}

# 主函数
main() {
    log_info "KubeMinds E2E 测试脚本"
    log_info "当前集群: $(kubectl config current-context)"
    log_info "命名空间: $NAMESPACE"
    log_info "场景: $SCENARIO"

    init_namespace

    case $SCENARIO in
        oom)
            test_oom
            ;;
        imagepull)
            test_imagepull
            ;;
        crashloop)
            test_crashloop
            ;;
        all)
            cleanup
            test_oom
            cleanup
            test_imagepull
            cleanup
            test_crashloop
            ;;
        cleanup)
            cleanup
            ;;
        *)
            log_error "未知场景: $SCENARIO"
            echo "用法: $0 {oom|imagepull|crashloop|all|cleanup}"
            exit 1
            ;;
    esac

    show_results
    log_success "E2E 测试完成"
}

main
