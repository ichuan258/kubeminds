# E2E 测试计划 (Mock LLM)

**目标**：在真实 K8s 环境中验证 MVP 完整流程，使用 Mock LLM 避免外部 API 依赖。

**范围**：CRD 创建 → Skill 匹配 → Agent 诊断 → 结果反馈

---

## 阶段 1: 环境准备

### 1.1 K8s 集群

**用户已确认**：
- 集群环境：GCloud（有现有配置文件，但未添加）
- 需要的信息：
  - [ ] GCloud 项目 ID
  - [ ] Cluster 名称与 region
  - [ ] kubeconfig 位置 / 获取方式（`gcloud container clusters get-credentials`）

**待办**：
```bash
# 步骤示例（需用户确认参数）
gcloud container clusters get-credentials CLUSTER_NAME --zone=ZONE --project=PROJECT_ID
kubectl config current-context  # 验证连接
```

### 1.2 Operator 部署

```bash
# 编译 Operator 二进制
go build -o ./bin/kubeminds-manager ./cmd/manager

# 使用 Mock 配置部署（见 1.3）
kubectl apply -f ./config/rbac/role.yaml
kubectl apply -f ./config/rbac/rolebinding.yaml
kubectl apply -f ./config/crd/bases/kubeminds.io_diagnosistasks.yaml
# 部署 Operator Pod（需 Dockerfile）
```

### 1.3 Mock LLM 配置

**在 `cmd/config/config.yaml` 中添加 Mock Provider**：

```yaml
# 临时改为 Mock Provider（或通过 env 覆盖）
apiKey: "mock"
model: "mock"
baseUrl: "http://localhost:9999/v1"  # 不会真正调用
```

**或在 `internal/llm/openai.go` 中新增 MockProvider**（推荐）：

```go
type MockProvider struct {
    nextResponse string
}

func NewMockProvider() *MockProvider {
    return &MockProvider{
        nextResponse: `{"type": "assistant", "content": "Root Cause: Pod OOM\nSuggestion: Increase memory limit"}`,
    }
}

func (m *MockProvider) Chat(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.Message, error) {
    // 返回预设响应，不调用真实 LLM
    return &agent.Message{
        Type:    agent.MessageTypeAssistant,
        Content: m.nextResponse,
    }, nil
}
```

---

## 阶段 2: 测试场景

### 场景 1: OOM Pod 诊断（推荐首先）

**前置**：
```bash
# 在 K8s 中创建一个内存溢出的 Pod
kubectl create namespace test-diagnosis
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: oom-pod
  namespace: test-diagnosis
spec:
  containers:
  - name: app
    image: busybox
    command: ["sh", "-c", "yes | head -c 1000000000 > /dev/null"]
    resources:
      limits:
        memory: "64Mi"
      requests:
        memory: "32Mi"
EOF

# 等待 Pod 被 OOMKilled
sleep 10
kubectl get pod oom-pod -n test-diagnosis
```

**触发诊断**：
```bash
kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-oom
  namespace: test-diagnosis
spec:
  podRef:
    name: oom-pod
    namespace: test-diagnosis
  approved: false  # 不需要写操作
EOF
```

**验证**：
```bash
# 检查 Agent 是否被调度
kubectl get diagnosistask diagnose-oom -n test-diagnosis -o yaml

# 查看最终报告
kubectl get diagnosistask diagnose-oom -n test-diagnosis -o jsonpath='{.status.report}'
```

**预期结果**：
- `status.phase = Completed`
- `status.report` 包含 "Root Cause: OOM"
- 工具调用：`get_pod_logs` + `get_pod_events` + `get_pod_spec`

---

### 场景 2: ImagePullBackOff（可选）

```bash
# 创建一个拉取失败的 Pod
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: image-pull-pod
  namespace: test-diagnosis
spec:
  containers:
  - name: app
    image: nonexistent-registry.example.com/app:v1
    imagePullPolicy: Always
EOF

# 创建诊断任务
kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-image-pull
  namespace: test-diagnosis
spec:
  podRef:
    name: image-pull-pod
    namespace: test-diagnosis
  approved: false
EOF
```

**预期工具调用**：`get_pod_events` + `get_pod_spec`

---

### 场景 3: 写操作验证（可选）

```bash
# 创建一个 CrashLoop 的 Pod
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: crashloop-pod
  namespace: test-diagnosis
spec:
  containers:
  - name: app
    image: busybox
    command: ["sh", "-c", "exit 1"]
    restartPolicy: Always
EOF

# 创建诊断任务，请求删除 Pod
kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-crashloop
  namespace: test-diagnosis
spec:
  podRef:
    name: crashloop-pod
    namespace: test-diagnosis
  approved: true  # 申请写操作
  requestedActions:
  - type: delete_pod
EOF
```

**预期行为**：
- `status.phase = WaitingApproval` → `Running` → `Completed`
- 如果 `approved: true`，`delete_pod` 工具被执行
- Pod 被删除

---

## 阶段 3: 日志验证

**Operator 日志**：
```bash
kubectl logs -n kube-system deployment/kubeminds-manager -f
# 应看到：
# INFO reconciling DiagnosisTask diagnosistask=test-diagnosis/diagnose-oom
# INFO agent starting run goal=Diagnose pod
# INFO executing tool tool=get_pod_logs
# INFO agent decided to finish
```

**诊断结果检查**：
```bash
kubectl describe diagnosistask diagnose-oom -n test-diagnosis
# 应显示 status.report 和 status.findings
```

---

## 阶段 4: 覆盖场景清单

| 场景 | Status | 工具调用 | 结果验证 |
|------|--------|---------|---------|
| OOM | ✅ | get_pod_logs, get_pod_events, get_pod_spec | report 包含 "OOM" |
| ImagePull | ⏳ | get_pod_events, get_pod_spec | report 包含 "Image" |
| CrashLoop | ⏳ | get_pod_logs, get_pod_events | report 包含 "CrashLoop" |
| 写操作 | ⏳ | delete_pod (approved=true) | Pod 被删除 |

---

## 阶段 5: Mock LLM 策略

**Option A: 在代码中添加 MockProvider**（推荐）
- 修改 `internal/llm/openai.go`，添加环境变量开关
- ```go
  if os.Getenv("MOCK_LLM") == "true" {
      llmProvider = llm.NewMockProvider()
  }
  ```
- 启动时：`MOCK_LLM=true go run ./cmd/manager/main.go`

**Option B: Mock HTTP Server**
- 启动一个本地 HTTP 服务器，暴露 OpenAI 兼容接口
- 配置 `baseUrl: http://localhost:9999/v1`
- 在测试脚本中自动启动 Mock Server

**Option C: 使用真实 LLM（后续）**
- 一旦测试流程验证成功，切换为真实 Gemini/DeepSeek/OpenAI

---

## 待办清单

- [ ] 确认 GCloud 集群信息（项目 ID、Cluster 名称、Region）
- [ ] 获取 kubeconfig 或配置 gcloud CLI
- [ ] 选择 Mock LLM 实现方式（Option A/B/C）
- [ ] 编写/更新 Dockerfile for Operator
- [ ] 测试场景 1: OOM（首先）
- [ ] 测试场景 2: ImagePull（可选）
- [ ] 测试场景 3: 写操作（可选）
- [ ] 验证所有工具调用日志
- [ ] 文档化测试结果

---

## 参考资源

- [K8s 集群访问](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/)
- [GCloud Container 文档](https://cloud.google.com/kubernetes-engine/docs/how-to/access-scopes)
- [Controller-runtime 本地测试](https://book.kubebuilder.io/cronjob-tutorial/running-webhook.html)
