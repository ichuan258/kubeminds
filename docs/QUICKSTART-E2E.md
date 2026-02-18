# KubeMinds E2E 测试快速启动指南

## 📋 前置条件

- ✅ MVP 已完成（12 个工具，43 个测试全通过）
- K8s 集群（GCloud、Kind、本地都支持）
- `kubectl` 已配置并可访问集群

## 🚀 第一步：使用 Mock LLM 本地运行

### 选项 1: 在本地运行 Operator（推荐用于开发）

```bash
# 1. 编译 Operator
go build -o ./bin/kubeminds-manager ./cmd/manager

# 2. 启动 Operator with Mock LLM
export KUBECONFIG=~/.kube/config  # 或你的 K8s 配置
./bin/kubeminds-manager --mock-llm

# 输出应该显示：
# Mock LLM mode enabled - using test responses instead of real API
# INFO starting manager
```

### 选项 2: 在 Docker 中运行

```bash
# 1. 构建镜像（假设已有 Dockerfile）
docker build -t kubeminds:dev -f Dockerfile .

# 2. 推送到 K8s（如果需要）
# 配置 imagePullPolicy: Never 以使用本地镜像

# 3. 部署 Operator
kubectl apply -f config/manager/manager.yaml  # 需要配置 mock-llm=true
```

## 🧪 第二步：运行 E2E 测试场景

### 场景 1: OOM Pod 诊断（首选）

```bash
# 1. 准备测试命名空间
kubectl create namespace kube-minds-test

# 2. 创建会导致 OOM 的 Pod
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-oom
  namespace: kube-minds-test
spec:
  restartPolicy: Never
  containers:
  - name: mem-hog
    image: busybox:latest
    command: ["sh", "-c"]
    args: ["yes | head -c 1000000000 > /dev/null"]
    resources:
      limits:
        memory: "64Mi"
      requests:
        memory: "32Mi"
EOF

# 3. 等待 Pod 进入 OOMKilled 状态
sleep 10
kubectl get pod test-oom -n kube-minds-test

# 4. 创建诊断任务
cat <<EOF | kubectl apply -f -
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-oom-01
  namespace: kube-minds-test
spec:
  podRef:
    name: test-oom
    namespace: kube-minds-test
  approved: false
  maxSteps: 5
EOF

# 5. 等待诊断完成
kubectl get diagnosistask diagnose-oom-01 -n kube-minds-test -w

# 6. 查看诊断报告
kubectl get diagnosistask diagnose-oom-01 -n kube-minds-test -o jsonpath='{.status.report}'

# 预期输出应包含 "OOM" 或 "memory"
```

### 场景 2: ImagePullBackOff（快速）

```bash
# 1. 创建无法拉取镜像的 Pod
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-image-pull
  namespace: kube-minds-test
spec:
  restartPolicy: Never
  containers:
  - name: app
    image: nonexistent.registry.invalid/app:v1-notexist
    imagePullPolicy: Always
EOF

# 2. 等待 Pod 进入 ImagePullBackOff
sleep 5

# 3. 创建诊断任务
cat <<EOF | kubectl apply -f -
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-imagepull-01
  namespace: kube-minds-test
spec:
  podRef:
    name: test-image-pull
    namespace: kube-minds-test
  approved: false
EOF

# 4. 查看诊断报告
kubectl get diagnosistask diagnose-imagepull-01 -n kube-minds-test -o jsonpath='{.status.report}'
```

### 场景 3: 使用自动化脚本

```bash
# 运行所有场景
chmod +x ./hack/e2e-test.sh
./hack/e2e-test.sh all

# 或运行单个场景
./hack/e2e-test.sh oom
./hack/e2e-test.sh imagepull
./hack/e2e-test.sh crashloop
```

## 📊 验证测试结果

### 查看 Operator 日志

```bash
# 本地运行
# Operator 会在控制台输出日志

# 在集群中运行
kubectl logs -n kube-system deployment/kubeminds-manager -f
```

### 检查诊断任务状态

```bash
# 列出所有诊断任务
kubectl get diagnosistasks -n kube-minds-test

# 查看详细信息
kubectl describe diagnosistask diagnose-oom-01 -n kube-minds-test

# 导出为 YAML
kubectl get diagnosistask diagnose-oom-01 -n kube-minds-test -o yaml
```

### 验证工具调用

```bash
# 从诊断报告中查看调用的工具
kubectl get diagnosistask diagnose-oom-01 -n kube-minds-test -o jsonpath='{.status.findings[*].toolName}'

# 预期工具序列：
# - get_pod_logs
# - get_pod_events
# - get_pod_spec
# （可能顺序不同）
```

## 🔍 故障排查

### 问题 1: 诊断任务卡在 Running 状态

**症状**：`kubectl get diagnosistask xxx` 显示 `Running` 但超过 5 分钟无进展

**排查**：
```bash
# 检查 Operator 日志
kubectl logs -n kube-system deployment/kubeminds-manager --tail=100

# 检查 Pod 是否真实存在
kubectl get pod test-oom -n kube-minds-test

# 增加超时时间
kubectl patch diagnosistask diagnose-oom-01 -n kube-minds-test --type='json' \
  -p='[{"op": "replace", "path": "/spec/maxSteps", "value": 10}]'
```

### 问题 2: Mock LLM 响应不正确

**症状**：诊断报告没有包含预期的关键字（如 "OOM"）

**排查**：
```bash
# 确保使用了 --mock-llm 标志
ps aux | grep kubeminds-manager | grep mock-llm

# 查看 Operator 日志中的 "Using Mock LLM provider" 消息

# 检查 Pod 名称中是否包含关键字
# MockProvider 通过简单的字符串匹配来判断故障类型
```

### 问题 3: K8s 连接错误

**症状**：`unable to create kubernetes client` 或 `unable to list pods`

**排查**：
```bash
# 验证 kubeconfig
kubectl config view
kubectl auth can-i list pods --all-namespaces

# 手动测试 API 访问
kubectl get pods -n kube-minds-test
```

## 📝 下一步

- ✅ 验证 Mock LLM E2E 流程通过
- 🔄 切换到真实 LLM（Gemini / DeepSeek / OpenAI）
- 📊 采集性能指标
- 🚀 在 CI/CD 中集成 E2E 测试

## 🎯 成功标准

E2E 测试成功需要满足：

1. **诊断任务创建**：Pod 存在，CRD 可创建 ✅
2. **Skill 匹配**：Agent 选择正确的 Skill（OOM → oom_diagnosis） ✅
3. **工具调用**：至少 2 个工具被成功调用 ✅
4. **LLM 推理**：Mock LLM 返回包含关键字的诊断报告 ✅
5. **状态转移**：任务从 Pending → Running → Completed ✅
6. **报告生成**：status.report 包含 Root Cause 和 Suggestion ✅

---

**快速链接**：
- [E2E 测试计划详见](./e2e-test-plan.md)
- [Mock LLM 实现](../internal/llm/mock.go)
- [自动化脚本](../hack/e2e-test.sh)
