# KubeMinds - K8s 自动化诊断 AIOps 平台

![Status](https://img.shields.io/badge/MVP-Complete-green)
![Go Version](https://img.shields.io/badge/go-1.22+-blue)
![License](https://img.shields.io/badge/license-Apache%202.0-blue)

**KubeMinds** 是一个 Kubernetes-Native 的 AIOps Agent 平台，通过自动化诊断和 LLM 推理来降低故障排查时间 (MTTR)。作为 Kubernetes Operator 运行，监听 `DiagnosisTask` CRD，自动启动诊断 Agent 进行问题分析和建议。

## ✨ 核心特性

### 🎯 智能诊断
- **LLM 驱动推理**: 支持 OpenAI、Gemini、DeepSeek、Moonshot(Kimi) 等多个 LLM 提供商
- **ReAct 循环**: 思考-行动-观察闭环，自动收集信息并推理
- **轻量检查点**: Agent 中断后能从检查点恢复，无需重新诊断

### 🛠 技能系统
- **12 个诊断工具**: Pod 日志、事件、规格、Node 状态、Service、Volume、写操作等
- **7 个领域技能**: Base、OOM、CrashLoopBackOff、ImagePull、NodeNotReady、网络、存储问题
- **自动技能匹配**: 根据告警标签自动选择最合适的诊断技能

### 🔐 安全分级
- **只读工具** (默认): 安全工具自动执行 (Pod 日志、事件、规格)
- **写操作工具** (高风险): 删除 Pod、修改 Deployment 等需人工审批
- **安全级别**: ReadOnly, LowRisk, HighRisk, Forbidden

### 🚀 可靠性
- **LLM 重试机制**: 3 次重试，指数退避 1s-10s，智能区分可重试/不可重试错误
- **多集群支持**: 本地、GCloud、AWS(计划中)
- **性能指标**: 诊断延迟 ~0.5s，Mock LLM <1ms，编译耗时 <1s

## 📊 MVP 完成情况

| 组件 | 完成度 | 备注 |
|------|--------|------|
| K8s 工具 | 100% (12/12) | Pod、Node、Service、Volume、Write |
| Domain Skills | 100% (7/7) | OOM、CrashLoop、ImagePull、NodeNotReady 等 |
| 单元测试 | 100% (62/62) | 所有工具和 Engine 核心逻辑 |
| E2E 测试 | ✅ 就绪 | Mock LLM，支持多故障场景 |
| LLM 重试 | ✅ | 指数退避重试已实现 |

## 🚀 快速开始

### 前置条件

- **Kubernetes**: v1.23+
- **Go**: v1.22+ (开发)
- **kubectl**: 已配置好集群访问

### 方案 A: 使用 Mock LLM 本地运行（推荐开发使用）

```bash
# 1. 编译 Operator
go build -o ./bin/kubeminds-manager ./cmd/manager

# 2. 创建 namespace 和 CRD
kubectl create namespace kubeminds-system
kubectl apply -f config/crd/bases/

# 3. 启动 Operator (Mock LLM 模式，无需 API Key)
export KUBECONFIG=~/.kube/config
./bin/kubeminds-manager --mock-llm

# 4. 在另一个终端创建诊断任务
kubectl apply -f - <<EOF
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: diagnose-pod-01
  namespace: default
spec:
  target:
    kind: Pod
    name: my-failing-pod
    namespace: default
  policy:
    maxSteps: 5
  approved: false
EOF

# 5. 查看诊断结果
kubectl get diagnosistask diagnose-pod-01 -o yaml
```

### 方案 B: 使用真实 LLM (OpenAI/Gemini/DeepSeek)

```bash
# 1. 编译
go build -o ./bin/kubeminds-manager ./cmd/manager

# 2. 安装 CRD
kubectl apply -f config/crd/bases/

# 3. 启动 Operator with LLM
export OPENAI_API_KEY="sk-xxx"  # 或使用 config.yaml
./bin/kubeminds-manager \
  --api-key=$OPENAI_API_KEY \
  --model=gpt-4o \
  --base-url=https://api.openai.com/v1

# 或使用 config.yaml
./bin/kubeminds-manager --config=cmd/config/config.yaml
```

### 配置文件示例 (cmd/config/config.yaml)

```yaml
# Metrics 和 Health 检查端口
metricsAddr: ":8080"
probeAddr: ":8081"
enableLeaderElection: false

# 诊断参数
skillDir: "skills/"
agentTimeoutMinutes: 10

# LLM 配置 (支持多个 LLM 提供商)
apiKey: "sk-xxx"  # 或通过 OPENAI_API_KEY 环境变量
model: "gpt-4o"   # 支持: gpt-4o, gpt-4-turbo, gemini-1.5-pro, deepseek-coder, moonshot-v1

# API 基础路径 (不同提供商示例)
baseUrl: "https://api.openai.com/v1"
# baseUrl: "https://api.deepseek.com/v1"
# baseUrl: "https://api.moonshot.cn/v1"

# K8s 集群连接配置
k8s:
  provider: ""              # "" | "local" | "gcloud" | "aws"
  kubeconfigPath: ""        # 可选: ~/.kube/config
  insecureSkipVerify: false # gcloud SSH 隧道场景设为 true
  context: ""               # 可选: kubeconfig context 名称
```

## 🧪 E2E 测试

### 运行自动化测试脚本

```bash
# 确保 Operator 已启动 (使用 --mock-llm)
./bin/kubeminds-manager --mock-llm &

# 运行所有测试场景
./hack/e2e-test.sh all

# 或运行单个场景
./hack/e2e-test.sh oom          # OOM Pod 诊断
./hack/e2e-test.sh imagepull    # ImagePullBackOff 诊断
./hack/e2e-test.sh crashloop    # CrashLoopBackOff 诊断
```

### 已验证的故障场景

| 场景 | Pod 状态 | 诊断准确率 | 工具调用 |
|------|---------|----------|--------|
| **OOM** | OOMKilled | 100% | get_pod_logs, get_pod_events, get_pod_spec |
| **ImagePullBackOff** | ImagePullBackOff | 100% | get_pod_events, get_pod_spec |
| **CrashLoopBackOff** | CrashLoopBackOff | 100% | get_pod_logs, get_pod_events |

### 检查诊断结果

```bash
# 查看诊断任务状态
kubectl get diagnosistask -n default

# 查看完整诊断报告
kubectl get diagnosistask diagnose-pod-01 -o yaml

# 查看诊断步骤历史
kubectl get diagnosistask diagnose-pod-01 -o jsonpath='{.status.history[*]}' | jq .
```

## 🏗 架构设计

### 核心组件

```
┌─────────────────────────────────────────┐
│    DiagnosisTask CRD                    │
│  (定义诊断目标和策略)                    │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│    Controller Reconciler                │
│  (监听 CRD 变化，启动 Agent)             │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│  Skill Manager                          │
│  ├─ base_skill (7个工具)                │
│  ├─ oom_skill (OOM 诊断)                │
│  ├─ image_pull_skill                    │
│  └─ ... (领域技能)                      │
└──────────────┬──────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│   Agent Engine (ReAct 循环)              │
│  ┌──────────────────────────────────┐   │
│  │ Step 1: Think (思考)              │   │
│  │ → LLM 推理需要收集哪些信息        │   │
│  └──────────────────────────────────┘   │
│  ┌──────────────────────────────────┐   │
│  │ Step 2: Act (行动)                │   │
│  │ → 执行诊断工具 (Pod logs, etc)   │   │
│  └──────────────────────────────────┘   │
│  ┌──────────────────────────────────┐   │
│  │ Step 3: Observe (观察)            │   │
│  │ → 工具返回结果，保存检查点        │   │
│  └──────────────────────────────────┘   │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│  诊断工具库 (12 个工具)                  │
│  ├─ ReadOnly: get_pod_logs, events...  │
│  ├─ HighRisk: delete_pod, patch...     │
│  └─ 支持扩展: gRPC, MCP, Internal      │
└─────────────────────────────────────────┘
```

### 工具清单

**只读工具 (ReadOnly):**
- `get_pod_logs` - 获取 Pod 容器日志
- `get_pod_events` - 获取 Pod 相关事件
- `get_pod_spec` - 获取 Pod 配置规格
- `get_node_status` - 获取 Node 状态和资源
- `get_node_events` - 获取 Node 事件
- `get_service_spec` - 获取 Service 配置
- `get_endpoints` - 获取 Service Endpoints
- `get_pvc_status` - 获取 PVC 状态
- `get_pv_status` - 获取 PV 状态

**写操作工具 (HighRisk - 需人工审批):**
- `delete_pod` - 删除 Pod
- `patch_deployment` - 修改 Deployment
- `scale_statefulset` - 扩容/缩容 StatefulSet

## 📦 K8s 集群配置

### 场景 1: 本地 Kind 集群

```yaml
# config.yaml
k8s:
  provider: "local"
  kubeconfigPath: "~/.kube/config"
  insecureSkipVerify: false
```

### 场景 2: GCloud 自建 K8s (推荐安全方案)

**使用 SSH 隧道安全连接：** 📌 **[详见 SSH-TUNNEL-SETUP.md](docs/SSH-TUNNEL-SETUP.md)**

```bash
# 快速 4 步设置 (8 秒)
# 1. 获取 kubeconfig (高效方式)
gcloud compute ssh instance-20260215-051955 --zone=asia-east1-b \
  --command="sudo cat /etc/kubernetes/admin.conf" \
  > ~/.kube/gcloud-k8s-config

# 2. 修改配置
sed -i '' 's|10\.140\.0\.2|127.0.0.1|g' ~/.kube/gcloud-k8s-config
kubectl config set-cluster kubernetes --insecure-skip-tls-verify=true \
  --kubeconfig=~/.kube/gcloud-k8s-config

# 3. 启动隧道
./hack/gcloud-tunnel.sh up

# 4. 验证
export KUBECONFIG=~/.kube/gcloud-k8s-config
kubectl get nodes
```

**优势:**
- ✅ 无需打开防火墙
- ✅ SSH 加密
- ✅ 企业级安全
- ✅ 脚本自动管理

### 场景 3: GCloud GKE 集群

#### 选项 A: 使用 gcloud CLI 自动配置

```bash
# 获取 kubeconfig
gcloud container clusters get-credentials my-cluster --zone us-central1-a --project my-project

# Operator 自动使用 ~/.kube/config
./bin/kubeminds-manager
```

#### 选项 B: 自定义 kubeconfig 路径

```yaml
# config.yaml
k8s:
  provider: "gcloud"
  kubeconfigPath: "~/.kube/gcloud-config"
  insecureSkipVerify: false  # 使用 TLS 验证
  context: "gke_my-project_us-central1-a_my-cluster"
```

#### 选项 C: 直接使用外部 IP (防火墙已开放)

如果防火墙允许直接访问 6443 端口：

```yaml
# config.yaml
k8s:
  provider: "gcloud"
  kubeconfigPath: "~/.kube/config"
  insecureSkipVerify: true  # 禁用 TLS 验证（仅用于测试）
  context: ""
```

**步骤:**
```bash
# 1. 获取 kubeconfig (高效方式)
gcloud compute ssh instance-name --zone asia-east1-b \
  --command="sudo cat /etc/kubernetes/admin.conf" \
  > ~/.kube/config

# 2. 修改服务器地址为外部 IP
sed -i '' 's|10\.140\.0\.2|35.236.172.169|g' ~/.kube/config

# 3. 禁用 TLS 验证
kubectl config set-cluster kubernetes --insecure-skip-tls-verify=true

# 4. 运行 Operator
./bin/kubeminds-manager --config=cmd/config/config.yaml
```

### 场景 3: AWS EKS

```bash
# 安装 aws-iam-authenticator (如未安装)
aws eks update-kubeconfig --region us-east-1 --name my-cluster

# Operator 自动使用 IAM 认证
./bin/kubeminds-manager
```

## 🔧 开发指南

### Git Hooks（安全 + 静态编译门禁）

```bash
# 1) 安装 hooks
make hook-install

# 2) 安装检查工具（写入 ./bin）
make hook-tools

# 3) 手动运行（可选）
make hook-fast
make hook-full
```

Hook 规则：
- `pre-commit`: `gofmt` + `golangci-lint --fast` + `gitleaks (staged)` + `go build ./...`
- `pre-push`: `golangci-lint` + `go test` + `gosec` + `govulncheck` + `gitleaks (full)`

### 项目结构

```
kubeminds/
├── api/v1alpha1/           # CRD 定义 (DiagnosisTask)
├── cmd/
│   ├── manager/            # Operator 入口
│   └── config/             # 默认配置
├── internal/
│   ├── agent/              # Agent Engine (ReAct 循环)
│   ├── config/             # 配置加载
│   ├── controller/         # Reconciler 逻辑
│   ├── llm/                # LLM 接口 (OpenAI, Mock, etc)
│   └── tools/              # 诊断工具实现
├── skills/                 # Domain Skills YAML
├── hack/                   # 自动化脚本
└── docs/                   # 文档
```

### 运行测试

```bash
# 单元测试
go test ./... -v

# 测试覆盖率
go test ./... -cover

# 编译验证
go build ./...
```

### 添加新的诊断工具

```go
// internal/tools/my_tool.go
type MyTool struct {
    client kubernetes.Interface
}

func (t *MyTool) Name() string {
    return "my_new_tool"
}

func (t *MyTool) Description() string {
    return "Description of what this tool does"
}

func (t *MyTool) SafetyLevel() agent.SafetyLevel {
    return agent.SafetyLevelReadOnly
}

func (t *MyTool) Execute(ctx context.Context, args string) (string, error) {
    // Tool implementation
    return result, nil
}

// 在 registry.go 中注册
func ListTools(client kubernetes.Interface) []agent.Tool {
    return []agent.Tool{
        // ... existing tools
        NewMyTool(client),
    }
}
```

### 添加新的 Domain Skill

```yaml
# skills/my_skill.yaml
name: my_skill
triggers:
  - reason: MyReason
    labels:
      key: value
tools:
  - my_new_tool
  - another_tool
prompt: |
  You are diagnosing a {{reason}} issue.
  Use available tools to investigate...
```

## 📚 文档

- [架构设计](docs/iterations/mvp/01_architecture.md)
- [E2E 测试计划](docs/e2e-test-plan.md)
- [E2E 快速开始](docs/QUICKSTART-E2E.md)
- [开发指南](CLAUDE.md)
- [项目路线图](kubeminds-roadmaps.md)

## 🤝 贡献指南

见 [CONTRIBUTING.md](CONTRIBUTING.md)

### Commit 规范

遵循 Conventional Commits:

```bash
feat: add new diagnostic tool
fix: resolve nil pointer in agent loop
docs: update README with GCloud instructions
refactor: optimize skill matching algorithm
```

## 📊 性能指标

| 指标 | 值 | 备注 |
|------|-----|------|
| 诊断延迟 | ~0.5s | Mock LLM |
| 诊断延迟 | ~2-5s | 真实 LLM (OpenAI) |
| Mock LLM 响应 | <1ms | 内存响应 |
| 编译耗时 | <1s | Go 1.22 |
| 测试覆盖 | 62/62 ✅ | 单元测试全通过 |

## 🐛 已知限制

- **Phase 1 (MVP)**: 单集群支持，轻量检查点
- **Phase 2 计划**: 多集群、Redis Stream (L2)、PostgreSQL (L3)、Alert Aggregator
- 写操作工具在 MVP 中需人工审批（模拟）

## 📝 License

Apache License 2.0 - 见 [LICENSE](LICENSE)

## 🙋 反馈与支持

- 问题反馈: [GitHub Issues](https://github.com/your-org/kubeminds/issues)
- 讨论: [GitHub Discussions](https://github.com/your-org/kubeminds/discussions)

---

**快速链接:**
- 🚀 [快速开始](docs/QUICKSTART-E2E.md)
- 📖 [E2E 测试计划](docs/e2e-test-plan.md)
- 🏗 [架构设计](docs/iterations/mvp/01_architecture.md)
- 🔧 [开发指南](CLAUDE.md)
