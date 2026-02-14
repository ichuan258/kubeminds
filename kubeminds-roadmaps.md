# KubeMinds — K8s 智能运维 Agent 平台（完整终版）

---

## 一、项目全景

### 一句话定位

KubeMinds 是一个 K8s-Native 的 AIOps Agent，以 CRD + Operator 模式部署在集群内，自动感知异常 → 匹配专家技能 → 调用工具链采集上下文 → LLM 推理根因 → 生成并执行修复方案，将 MTTR 从 30 分钟降低到 3 分钟以内。

### 核心痛点

传统 K8s 运维中，告警触发后 SRE 需手动执行：查日志 → 查事件 → 查指标 → 查变更记录 → 判断根因 → 执行修复。平均耗时 15-40 分钟，高度依赖个人经验，且专家知识无法沉淀复用。

### 技术栈选型

| 层级 | 技术选型 | 选型理由 |
|---|---|---|
| Agent 核心 | Go + 自研 Agent Loop | K8s 生态原生契合，避免 Python GIL 瓶颈 |
| LLM 接入 | OpenAI API / 私有化 Ollama | Function Calling 驱动工具调用，大小模型分流 |
| 控制面 | K8s Operator (controller-runtime) | CRD 定义诊断任务，声明式管理 |
| 事件采集 | K8s Informer + Prometheus API | 实时监听集群事件与指标 |
| 记忆层 | Redis Stream + PostgreSQL (pgvector) | 短期上下文 + 长期故障知识库 + 向量语义检索 |
| 工具层 | gRPC + MCP + Internal 三层适配 | 高性能内部调用 + 外部生态复用 + 轻量计算 |
| 技能层 | YAML DSL + Skill Engine | SRE 专家经验编码为可复用诊断能力包 |
| 可观测性 | OpenTelemetry + Grafana | Agent 推理链路全链路追踪 |

---

## 二、技术选型决策：为什么用 Go 而不是 Python？

KubeMinds 不是 RAG 系统，不需要文档分块、Embedding Pipeline、重排序模型等 Python 生态的重型组件。它的核心动作是：调 K8s API → 拿结果 → 喂给 LLM → 解析工具调用 → 再调 K8s API。

### KubeMinds 实际用到的 AI 能力

- LLM API 调用（HTTP Client, Go 原生强项）
- Function Calling 解析（JSON, Go 原生强项）
- 向量检索（pgvector SQL 查询，不需要 LangChain）
- Prompt 管理（text/template, 足够）

### KubeMinds 不需要的 AI 能力

- 文档分块（Chunking）
- Embedding 模型本地运行
- RAG Pipeline
- Fine-tuning
- 多模态处理

### Go 在本项目中不可替代的优势

| 优势 | Python 能否替代？ |
|---|---|
| client-go 原生集成 | ❌ Python K8s 客户端是二等公民 |
| controller-runtime (Operator) | ❌ Python 没有成熟 Operator 框架 |
| CRD + Informer + Watch | ❌ Go 专属领地 |
| 单二进制部署 ~20MB | ❌ Python 镜像 ~500MB+ |
| Goroutine 并发 Agent (~20KB/个) | ⚠️ asyncio 受 GIL 限制 |
| Context 超时传播 | ⚠️ Python timeout 管理更脆弱 |

### 量化对比

| 维度 | Go (KubeMinds) | Python (假设方案) |
|---|---|---|
| Operator 开发 | controller-runtime 原生 | kopf 库，功能弱很多 |
| K8s API 调用 | client-go 一等公民 | kubernetes-python 二等公民 |
| 部署产物 | 单二进制 ~20MB | Docker 镜像 ~500MB+ |
| 并发 10 Agent | Goroutine, ~20KB/个 | asyncio, GIL 瓶颈 |
| LLM 调用开发量 | ~200 行 HTTP 封装 | openai 库 0 行 |
| Agent Loop 开发量 | ~500 行 | LangGraph ~50 行 |
| 总额外开发量 | ~1500 行 | ~0 行 |
| 生产稳定性 | 编译期类型检查 | 运行时报错 |

1500 行代码换来 K8s 原生集成 + 编译期安全 + 20MB 部署产物 + 真并发，在运维场景下这笔账是划算的。

> **面试话术：** "如果做企业知识库/RAG 系统，我会毫不犹豫选 Python。但 KubeMinds 是 K8s 运维场景——核心是 Operator + Agent Loop + K8s API 调用，这是 Go 的主场。我们用到的 AI 能力就是 LLM API 调用和 pgvector 检索，Go 的 HTTP Client 和 SQL Driver 够用。1500 行代码换来 client-go 原生集成、编译期安全、20MB 单二进制部署、真并发，这笔账划算。"

---

## 三、系统架构

```
┌──────────────────────────────────────────────────────────────┐
│                         K8s Cluster                          │
│                                                              │
│  ┌───────────────┐     ┌───────────────────────────────────┐ │
│  │ AlertManager   │────▶│       KubeMinds Operator          │ │
│  │ / K8s Event    │     │       (CRD: DiagnosisTask)        │ │
│  └───────────────┘     │                                   │ │
│                        │  ┌─────────────────────────────┐  │ │
│                        │  │     Alert Aggregator        │  │ │
│                        │  │  (去重 + 合并 + 优先级排序)   │  │ │
│                        │  └──────────────┬──────────────┘  │ │
│                        │                 │                 │ │
│                        │  ┌──────────────▼──────────────┐  │ │
│                        │  │       Skill Engine          │  │ │
│                        │  │  (Base Skill + Domain Skill) │  │ │
│                        │  └──────────────┬──────────────┘  │ │
│                        │                 │                 │ │
│                        │  ┌──────────────▼──────────────┐  │ │
│                        │  │     Agent Loop Engine        │  │ │
│                        │  │  Think → Validate → Act      │  │ │
│                        │  │  → Observe → Checkpoint      │  │ │
│                        │  └──────────────┬──────────────┘  │ │
│                        │                 │                 │ │
│                        │  ┌──────────────▼──────────────┐  │ │
│                        │  │    Unified Tool Router      │  │ │
│                        │  └──┬──────────┬───────────┬───┘  │ │
│                        └─────┼──────────┼───────────┼──────┘ │
│                              │          │           │        │
│                  ┌───────────▼──┐ ┌─────▼─────┐ ┌──▼─────┐  │
│                  │ gRPC Adapter │ │MCP Adapter│ │Internal│  │
│                  │ (K8s内部工具) │ │(外部生态)  │ │Adapter │  │
│                  │ · PodLog     │ │ · Slack   │ │ · 时间 │  │
│                  │ · Events     │ │ · GitHub  │ │   计算 │  │
│                  │ · Metrics    │ │ · Jira    │ │ · JSON │  │
│                  │ · PodSpec    │ │ · Pager   │ │   格式 │  │
│                  └──────────────┘ │   Duty    │ └────────┘  │
│                                   └───────────┘             │
│  ┌──────────────┐  ┌──────────────────┐                     │
│  │ Redis Stream  │  │ PostgreSQL       │                     │
│  │ (L2 短期记忆) │  │ + pgvector      │                     │
│  │              │  │ (L3 长期记忆)     │                     │
│  └──────────────┘  └──────────────────┘                     │
└──────────────────────────────────────────────────────────────┘
```

### 核心流程

AlertManager 触发告警 → Alert Aggregator 去重合并 → Skill Engine 匹配专家技能（Base + Domain 继承合并）→ Operator 创建 DiagnosisTask CR → Agent Loop 启动 → LLM 规划诊断步骤 → Unified Tool Router 分发到 gRPC/MCP/Internal → LLM 推理根因 → 安全校验 → 修复 Plan → 人工确认或自动执行 → 结果写入知识库。

---

## 四、模块详解

---

### 模块一：Memory — 三层记忆架构

#### 4.1.1 设计总览

```
┌──────────────────────────────────────────────────────┐
│  L1: Working Memory (工作记忆)                        │
│  载体: Go struct (in-process)                         │
│  生命周期: 单次诊断任务                                 │
│  内容: 当前告警上下文、已执行工具调用及结果、LLM对话历史    │
├──────────────────────────────────────────────────────┤
│  L2: Short-Term Memory (短期记忆)                     │
│  载体: Redis Stream                                   │
│  生命周期: 24小时滑动窗口 (TTL)                         │
│  内容: 近期故障事件流、集群状态快照                       │
│  用途: 关联分析 ("10分钟内同一Node上已经第3次OOM了")      │
├──────────────────────────────────────────────────────┤
│  L3: Long-Term Memory (长期记忆)                      │
│  载体: PostgreSQL + pgvector                          │
│  生命周期: 永久                                        │
│  内容: 历史故障诊断报告(结构化) + 故障Pattern Embedding  │
│  用途: 相似故障语义检索, 经验复用                        │
└──────────────────────────────────────────────────────┘
```

#### 4.1.2 为什么选 pgvector 而非 Milvus/Qdrant？

KubeMinds 部署在客户 K8s 集群内，需尽量减少外部依赖。多数企业已有 PostgreSQL，pgvector 是扩展而非独立服务，运维成本几乎为零。向量数据量级为万级（历史故障报告），IVFFlat 索引完全够用。Memory 层是接口抽象，可平滑替换为 Qdrant。

#### 4.1.3 统一接口定义

```go
type MemoryManager interface {
    // L1: 工作记忆
    GetWorkingContext(taskID string) (*DiagnosisContext, error)
    AppendToolResult(taskID string, toolName string, result *ToolResult) error

    // L2: 短期记忆
    GetRecentEvents(namespace string, window time.Duration) ([]*ClusterEvent, error)

    // L3: 长期记忆
    SearchSimilarFaults(embedding []float32, topK int) ([]*FaultReport, error)
    SaveFaultReport(report *FaultReport) error
}

type DiagnosisContext struct {
    TaskID      string
    Alert       *Alert
    Messages    []LLMMessage
    ToolCalls   []ToolCallRecord
    CurrentPlan *DiagnosisPlan
    StepIndex   int
    CreatedAt   time.Time
}
```

#### 4.1.4 记忆注入 Prompt 的策略

Agent 每轮调用 LLM 时分层组装 Prompt：L3 语义检索取 top3 相似案例 → L2 按时间窗口过滤关联事件 → L1 完整对话历史。

```go
func (a *Agent) buildPrompt(ctx *DiagnosisContext) []LLMMessage {
    messages := []LLMMessage{
        {Role: "system", Content: systemPrompt},
    }

    // L3: 相似历史故障（语义检索 top3）
    if similar := a.memory.SearchSimilarFaults(ctx.Alert.Embedding, 3); len(similar) > 0 {
        messages = append(messages, LLMMessage{
            Role:    "system",
            Content: fmt.Sprintf("历史相似故障诊断报告：\n%s", formatFaultReports(similar)),
        })
    }

    // L2: 近期关联事件（时间窗口过滤）
    if events := a.memory.GetRecentEvents(ctx.Alert.Namespace, 30*time.Minute); len(events) > 0 {
        messages = append(messages, LLMMessage{
            Role:    "system",
            Content: fmt.Sprintf("近30分钟同命名空间相关事件：\n%s", formatEvents(events)),
        })
    }

    // L1: 当前对话历史（完整保留）
    messages = append(messages, ctx.Messages...)
    return messages
}
```

> **面试话术：** "我们不是把所有记忆一股脑塞给 LLM，而是分层注入。L3 长期记忆做语义检索只取 top3 相似案例，L2 短期记忆按时间窗口过滤，L1 工作记忆是完整对话。既控制 token 消耗又保证上下文相关性。"

---

### 模块二：Tool Calling — 三层混合工具体系

#### 4.2.1 设计理念

单一工具调用模式无法同时满足性能、生态、灵活性三个需求，因此采用三层 Adapter 架构，对 Agent 层完全透明。

| 工具类型 | 选用模式 | 原因 |
|---|---|---|
| K8s 集群内诊断工具（日志、事件、指标） | **gRPC** | 高频调用，二进制序列化，进程隔离 |
| 外部协作工具（Slack、Jira、GitHub） | **MCP** | 复用社区已有 MCP Server，零代码接入 |
| 轻量计算工具（时间转换、JSON格式化） | **Internal** | 简单 Go 函数，无需跨进程 |

#### 4.2.2 gRPC 工具 — Proto 定义

```protobuf
service DiagnosisTool {
    rpc Describe(Empty) returns (ToolDescription);
    rpc Execute(ToolRequest) returns (ToolResponse);
}

message ToolDescription {
    string name = 1;
    string description = 2;
    string parameters_json_schema = 3;  // 直接喂给 LLM Function Calling
}

message ToolRequest {
    string task_id = 1;
    string parameters_json = 2;
    int32 timeout_seconds = 3;
}

message ToolResponse {
    bool success = 1;
    string result = 2;
    string error_message = 3;
}
```

每个工具自带 Describe 方法返回 JSON Schema，直接喂给 LLM 做 Function Calling，保证工具描述与实现的一致性。新增工具只需实现 DiagnosisTool 接口，部署为 sidecar 或独立 Pod，Operator 自动发现注册。

#### 4.2.3 gRPC 工具示例：Pod 日志采集

```go
type PodLogTool struct{}

func (t *PodLogTool) Describe(ctx context.Context, _ *Empty) (*ToolDescription, error) {
    return &ToolDescription{
        Name:        "get_pod_logs",
        Description: "获取指定Pod最近N行日志，用于分析崩溃原因和错误信息",
        ParametersJsonSchema: `{
            "type": "object",
            "properties": {
                "namespace":  {"type": "string", "description": "Pod所在namespace"},
                "pod_name":   {"type": "string", "description": "Pod名称"},
                "tail_lines": {"type": "integer", "description": "获取最近多少行", "default": 100},
                "container":  {"type": "string", "description": "容器名(可选)"}
            },
            "required": ["namespace", "pod_name"]
        }`,
    }, nil
}

func (t *PodLogTool) Execute(ctx context.Context, req *ToolRequest) (*ToolResponse, error) {
    var params struct {
        Namespace string `json:"namespace"`
        PodName   string `json:"pod_name"`
        TailLines int    `json:"tail_lines"`
        Container string `json:"container"`
    }
    json.Unmarshal([]byte(req.ParametersJson), &params)

    logs, err := t.k8sClient.CoreV1().Pods(params.Namespace).
        GetLogs(params.PodName, &corev1.PodLogOptions{
            TailLines: ptr.To(int64(params.TailLines)),
            Container: params.Container,
        }).Do(ctx).Raw()

    if err != nil {
        return &ToolResponse{Success: false, ErrorMessage: err.Error()}, nil
    }

    // 日志可能很长，截断后返回给 LLM
    truncated := truncateToTokenLimit(string(logs), 2000)
    return &ToolResponse{Success: true, Result: truncated}, nil
}
```

#### 4.2.4 MCP Adapter — 外部工具接入

```go
type MCPAdapter struct {
    clients map[string]*MCPClient
}

func (a *MCPAdapter) ConnectMCPServer(config MCPServerConfig) error {
    var transport MCPTransport
    switch config.Transport {
    case "stdio":
        transport = NewStdioTransport(config.Command, config.Args)
    case "sse":
        transport = NewSSETransport(config.URL)
    }

    client := NewMCPClient(transport)
    client.Initialize()
    tools, _ := client.ListTools()
    for _, tool := range tools {
        a.clients[tool.Name] = client
    }
    return nil
}

func (a *MCPAdapter) Execute(ctx context.Context, toolName string, params json.RawMessage) (*ToolResponse, error) {
    client := a.clients[toolName]
    result, err := client.CallTool(ctx, toolName, params)
    if err != nil {
        return &ToolResponse{Success: false, ErrorMessage: err.Error()}, nil
    }
    return &ToolResponse{Success: true, Result: result.Content[0].Text}, nil
}
```

MCP 配置示例：

```yaml
mcp_servers:
  - name: slack-notify
    transport: stdio
    command: "npx"
    args: ["-y", "@anthropic/slack-mcp-server"]
  - name: github-changes
    transport: stdio
    command: "npx"
    args: ["-y", "@anthropic/github-mcp-server"]
  - name: pagerduty
    transport: sse
    url: "http://pagerduty-mcp:8080/sse"
```

#### 4.2.5 Unified Tool Router — 对 Agent 透明的统一调度

```go
type ToolRouter struct {
    grpcTools     map[string]*GRPCAdapter
    mcpTools      map[string]*MCPAdapter
    internalTools map[string]InternalToolFunc
}

// 对 Agent 透明：它不关心底层是 gRPC、MCP 还是函数调用
func (r *ToolRouter) Dispatch(ctx context.Context, call *LLMFunctionCall) (*ToolResponse, error) {
    if tool, ok := r.grpcTools[call.Name]; ok {
        return tool.Execute(ctx, call)
    }
    if tool, ok := r.mcpTools[call.Name]; ok {
        return tool.Execute(ctx, call.Name, call.Arguments)
    }
    if fn, ok := r.internalTools[call.Name]; ok {
        return fn(ctx, call.Arguments)
    }
    return nil, fmt.Errorf("unknown tool: %s", call.Name)
}

// 生成 LLM Function Calling 所需的 tools 定义（三种来源合并）
func (r *ToolRouter) GetAllToolDefinitions() []LLMToolDef {
    var defs []LLMToolDef
    // 合并 gRPC 工具描述
    for _, tool := range r.grpcTools {
        desc, _ := tool.Describe(context.Background(), &Empty{})
        defs = append(defs, toLLMToolDef(desc))
    }
    // 合并 MCP 工具描述
    for _, tool := range r.mcpTools {
        defs = append(defs, tool.GetToolDefs()...)
    }
    // 合并 Internal 工具描述
    for name, fn := range r.internalTools {
        defs = append(defs, fn.Definition())
    }
    return defs
}
```

> **面试话术：** "工具体系分三层 Adapter。K8s 内部诊断工具用 gRPC 做进程隔离和高性能调用；外部协作工具通过 MCP 协议复用社区生态，零代码接入；轻量计算用内部 Go 函数。Unified Tool Router 对 Agent 完全透明，新增工具只需实现接口并注册。选 gRPC 而不是全用 MCP 是因为 K8s 内部工具调用频率高，MCP 的 JSON-RPC over stdio 在高频场景下性能不如 gRPC 的二进制序列化，且 gRPC 提供进程隔离——工具 panic 不影响 Agent 主进程。"

---

### 模块三：Skill Engine — 两层技能架构

#### 4.3.1 核心价值

裸 LLM + 工具只是通用推理器，Skill 将 SRE 专家经验编码为可复用的诊断能力包。相当于给一个聪明但没经验的实习生配了一本专家手册。

#### 4.3.2 为什么不做"统一 Skill"？为什么也不做"每告警一个 Skill"？

统一 Skill（一个大而全的运维技能）的问题：Prompt 过长、工具集过大导致 LLM 选择空间爆炸、无法注入针对性的诊断决策树。

每告警一个 Skill 的问题：Skill 数量爆炸、维护成本极高、大量重复内容。

解法是两层继承架构：

```
┌───────────────────────────────────────────────────┐
│  Layer 2: Domain Skills (领域技能, ~7个)            │
│  只定义差异化部分，约 20-30 行 YAML/个              │
│                                                   │
│  ┌──────────┐ ┌──────────┐ ┌───────────────────┐  │
│  │ OOM 诊断  │ │ 网络排障  │ │ CrashLoopBack     │  │
│  │ 专属Prompt│ │ 专属Prompt│ │ 诊断              │  │
│  │ 决策树    │ │ 决策树    │ │ 专属Prompt+决策树  │  │
│  └─────┬────┘ └─────┬────┘ └────────┬──────────┘  │
│        └────────────┼───────────────┘              │
│                     │ 继承 + 覆盖                    │
├─────────────────────┼─────────────────────────────┤
│  Layer 1: Base Skill (基座技能, 唯一)               │
│                                                   │
│  · 通用 K8s 运维 SystemPrompt                       │
│  · 全量工具集注册                                    │
│  · 通用诊断流程 (采集→分析→定位→建议)                 │
│  · 默认记忆策略                                      │
│  · 通用安全规则                                      │
└─────────────────────────────────────────────────────┘
```

运行逻辑：告警进来 → 先匹配 Layer 2 Domain Skill → 匹配到则 Domain 继承 Base 并覆盖特定字段 → 未匹配则直接使用 Base Skill，LLM 自由推理。

#### 4.3.3 Skill 数据结构

```go
type Skill struct {
    Name         string          // "oom_diagnosis"
    Description  string          // "诊断 OOMKilled 相关问题"
    Triggers     []TriggerRule   // 什么告警激活这个技能
    SystemPrompt string          // 领域专家经验 Prompt
    RequiredTools []string       // 绑定工具集（限定 LLM 选择空间）
    DecisionTree *DecisionNode   // 诊断决策树（引导 LLM 排查路径）
    MemoryPolicy *MemoryPolicy   // 该类故障的记忆查询策略
}

type TriggerRule struct {
    AlertName string
    Labels    map[string]string
    Priority  int
}

type DecisionNode struct {
    Step       string
    ToolToCall string
    ToolParams map[string]any
    OnSuccess  *DecisionNode
    OnFailure  *DecisionNode
    Conclusion string
}

type MemoryPolicy struct {
    ShortTermWindow time.Duration  // 这类故障需要看多久的近期事件
    LongTermTopK    int            // 检索多少条历史相似案例
    RelevantMetrics []string       // 需要关注的 Prometheus 指标
}
```

#### 4.3.4 继承合并逻辑

```go
func (e *SkillEngine) ResolveSkill(alert *Alert) *Skill {
    domain := e.matchDomainSkill(alert)
    if domain != nil {
        return e.baseSkill.MergeWith(domain)
    }
    return e.baseSkill  // 兜底：通用运维技能
}

func (base *Skill) MergeWith(domain *Skill) *Skill {
    merged := *base
    if domain.SystemPrompt != "" {
        // 拼接：通用 Prompt + 领域专属 Prompt
        merged.SystemPrompt = base.SystemPrompt + "\n\n" + domain.SystemPrompt
    }
    if len(domain.RequiredTools) > 0 {
        // 领域技能限定工具集（缩小 LLM 选择空间）
        merged.RequiredTools = domain.RequiredTools
    }
    if domain.DecisionTree != nil {
        merged.DecisionTree = domain.DecisionTree
    }
    if domain.MemoryPolicy != nil {
        merged.MemoryPolicy = domain.MemoryPolicy
    }
    return &merged
}
```

#### 4.3.5 Skill 引擎如何增强 Agent

```go
func (e *SkillEngine) EnhanceAgent(agent *Agent, skill *Skill) {
    // 1. 注入专属 System Prompt（Base + Domain 已合并）
    agent.SetSystemPrompt(skill.SystemPrompt)

    // 2. 限定可用工具集（减少 LLM 选择空间，降低幻觉）
    agent.SetAvailableTools(skill.RequiredTools)

    // 3. 注入决策树引导（作为 Prompt 的一部分）
    if skill.DecisionTree != nil {
        agent.InjectGuidance(skill.DecisionTree.ToPrompt())
    }

    // 4. 调整记忆策略
    agent.SetMemoryPolicy(skill.MemoryPolicy)
}
```

#### 4.3.6 示例：OOM 诊断 Domain Skill

```yaml
name: oom_diagnosis
description: "诊断 Pod OOMKilled 问题"

triggers:
  - alert_name: "KubePodCrashLooping"
    labels: { reason: "OOMKilled" }
  - alert_name: "KubeContainerOOMKilled"

system_prompt: |
  你是一名 K8s 内存问题专家。排查优先级：
  1. 检查 memory limit 是否设置过低
  2. 查看 Prometheus 近 1 小时内存使用趋势
  3. 若内存持续增长 → 内存泄漏，查看应用日志
  4. 若为突发尖峰 → 查看异常请求量
  注意：不要建议直接调大 limit，先找根因；Java 应用区分 heap 和 non-heap。

required_tools: [get_pod_logs, query_prometheus, get_pod_spec, get_events]

decision_tree:
  step: "获取 Pod 的 memory limit 配置"
  tool: get_pod_spec
  on_success:
    step: "查询近1小时内存使用趋势"
    tool: query_prometheus
    params:
      query: "container_memory_usage_bytes{pod='${pod_name}'}"
      range: "1h"
    on_success:
      conclusion: "内存平稳但接近 limit，建议调整 requests/limits"
    on_failure:
      step: "获取应用日志查找泄漏线索"
      tool: get_pod_logs
      params: { tail_lines: 500 }

memory_policy:
  short_term_window: 1h
  long_term_top_k: 5
  relevant_metrics:
    - container_memory_usage_bytes
    - container_memory_working_set_bytes
    - kube_pod_container_resource_limits
```

实际生产中维护约 7 个 Domain Skill：OOM、CrashLoop、网络不通、Node NotReady、PV 挂载失败、证书过期、镜像拉取失败。

#### 4.3.7 飞轮效应

```
告警进入 → SkillEngine.MatchSkill(alert)
              │
              ├─ 匹配到 Skill → EnhanceAgent(agent, skill)
              │
              └─ 未匹配 → 使用 Base Skill 通用诊断

诊断完成 → 经验沉淀为新 Domain Skill / 更新已有 Skill → 飞轮效应
```

> **面试话术：** "我们不是为每个告警写一个 Skill。Base Skill 覆盖 80% 场景，Domain Skill 只定义差异化的专家经验，继承 Base 并覆盖特定字段。实际生产中只维护了 7 个 Domain Skill，新增一个约 20 行 YAML。没有匹配 Skill 的新故障走通用诊断，完成后经验沉淀为新 Skill，形成飞轮效应。"

---

### 模块四：有状态 Agent 与无状态 Operator 的统一

#### 4.4.1 核心矛盾

| | Operator (Reconcile Loop) | Agent (Agent Loop) |
|---|---|---|
| 设计哲学 | 无状态、幂等、声明式 | 有状态、顺序执行、命令式 |
| 生命周期 | 永久运行，反复 Reconcile | 创建 → 执行 → 完成 → 销毁 |
| 状态管理 | 状态在 CR 的 Status 中 | 状态在进程内存中 |
| 失败恢复 | 重新 Reconcile（幂等） | 需要恢复策略 |

#### 4.4.2 ROI 分析：断点恢复是否值得？

一次典型 OOM 诊断的成本：

```
Step 1: get_pod_spec          ~0.5s   (K8s API)
Step 2: query_prometheus      ~0.8s   (Prometheus API)
Step 3: get_pod_logs          ~1.2s   (K8s API)
Step 4: LLM 综合分析          ~3.0s   (GPT-4o)
Step 5: 生成修复建议          ~2.0s   (GPT-4o)
────────────────────────────────────
工具调用总耗时:              ~2.5s
LLM 推理总耗时:              ~12s    (含每步 Think ~2s × 5)
总 Token 消耗:               ~8000 tokens (~$0.04)
总耗时:                      ~45s
```

假设在 Step 3 中断：

| 维度 | 重新开始 | 完整断点恢复 |
|---|---|---|
| 时间浪费 | 重跑 ~13s | ~1s 重建 |
| Token 浪费 | ~$0.025 | ~$0.0025 |
| 额外代码量 | 0 | ~800 行 |
| 开发周期 | 0 | 3-5 人天 |
| 维护/Bug风险 | 无 | 中高（状态一致性） |
| Operator 重启频率 | ~月级 | 同左 |
| 年化节省 | 基准 | 省 144s/年，省 $0.3/年 |

**关键转折点：审批状态丢失不可接受。** SRE 半夜被叫醒审批了一次，系统重启后再叫一次，体验极差。

#### 4.4.3 解法：分阶段恢复策略

```
Phase 1 (当前版本, 0 额外代码):
  诊断阶段 → 直接重跑（工具调用天然幂等，代价小）
  审批阶段 → CRD Status 天然持久化，零额外代码

Phase 2 (已实现, ~200 行代码):
  诊断阶段 → 轻量 Checkpoint，只保存阶段性发现（不是完整对话）
  恢复时作为 Prompt 注入，LLM 知道之前发现了什么，不会重复推理

Phase 3 (Roadmap):
  完整对话级 Checkpoint/Restore
  仅当超长诊断任务(>10步) + 高频重启 + Token 成本敏感时投入
```

#### 4.4.4 Phase 1 实现：重新开始 + 审批状态持久化

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var task v1alpha1.DiagnosisTask
    r.Get(ctx, req.NamespacedName, &task)

    switch task.Status.Phase {
    case "":
        task.Status.Phase = "Pending"
        return r.updateStatus(ctx, &task)

    case "Pending":
        skill := r.skillEngine.ResolveSkill(&task.Spec.Alert)
        agent := r.createAgent(&task, skill)
        task.Status.Phase = "Diagnosing"
        task.Status.MatchedSkill = skill.Name
        task.Status.StartedAt = metav1.Now()
        r.updateStatus(ctx, &task)
        go agent.Run(ctx)
        return ctrl.Result{}, nil

    case "Diagnosing":
        if r.agentManager.IsRunning(task.Name) {
            return ctrl.Result{}, nil // Agent 还在跑，不干预
        }
        // Operator 重启 → Agent 不在了
        if time.Since(task.Status.StartedAt.Time) > timeout {
            task.Status.Phase = "Failed"
            task.Status.Report = &DiagnosisReport{
                Summary: "诊断因系统重启超时，请手动排查",
            }
            return r.updateStatus(ctx, &task)
        }
        // 代价小，直接重跑（工具调用天然幂等）
        agent := r.createFreshAgent(&task)
        go agent.Run(ctx)
        return ctrl.Result{}, nil

    case "WaitingApproval":
        // 审批状态天然在 CRD 中，Operator 重启无影响
        if task.Spec.Approved {
            task.Status.Phase = "Executing"
            return r.updateStatus(ctx, &task)
        }
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

    case "Completed", "Failed":
        return ctrl.Result{}, nil
    }
    return ctrl.Result{}, nil
}
```

#### 4.4.5 Phase 2 实现：轻量 Checkpoint

不保存完整对话历史（太重，且有 etcd 1.5MB 限制风险），只保存每步得出的阶段性结论。

```go
type LightCheckpoint struct {
    Findings  []Finding `json:"findings"`
    StepIndex int       `json:"stepIndex"`
}

type Finding struct {
    Step    int    `json:"step"`
    Tool    string `json:"tool"`
    Summary string `json:"summary"`  // "Pod memory limit 256Mi, 实际使用峰值 251Mi"
}
```

Agent 每步结束时提取发现：

```go
func (a *Agent) extractFinding(step int, toolName string, result *ToolResponse) Finding {
    return Finding{
        Step:    step,
        Tool:    toolName,
        Summary: extractKeySummary(result.Result),
    }
}
```

恢复时注入 Prompt（LLM 知道之前发现了什么，不重复排查）：

```go
func (a *Agent) restoreFromFindings(findings []Finding) {
    summary := "以下是之前诊断已得出的发现，请基于此继续，不要重复已完成的排查：\n"
    for _, f := range findings {
        summary += fmt.Sprintf("- [Step %d, %s] %s\n", f.Step, f.Tool, f.Summary)
    }
    a.context.Messages = append(a.context.Messages, LLMMessage{
        Role:    "system",
        Content: summary,
    })
}
```

#### 4.4.6 CRD 完整定义

```yaml
apiVersion: kubeminds.io/v1alpha1
kind: DiagnosisTask
metadata:
  name: oom-nginx-2024-01-15
spec:
  alert:
    name: "KubePodCrashLooping"
    namespace: "production"
    pod: "nginx-7b4d9c-x2k8p"
    labels:
      reason: "OOMKilled"
  policy:
    maxSteps: 15
    timeoutMinutes: 5
    autoExecute: false
  approved: false            # 人工审批字段

status:
  phase: "Diagnosing"        # Pending → Diagnosing → WaitingApproval
                             # → Executing → Completed / Failed
  currentStep: 3
  matchedSkill: "oom_diagnosis"
  startedAt: "2024-01-15T03:22:00Z"

  # Phase 2: 轻量 Checkpoint
  checkpoint:
    findings:
      - step: 1
        tool: "get_pod_spec"
        summary: "memory limit 256Mi, requests 128Mi"
      - step: 2
        tool: "query_prometheus"
        summary: "近1小时内存持续增长，峰值251Mi，接近limit"
      - step: 3
        tool: "get_pod_logs"
        summary: "发现 java.lang.OutOfMemoryError: Java heap space"
    stepIndex: 3

  report:                    # 完成后填充
    rootCause: ""
    suggestion: ""
    confidence: 0
  conditions: []
```

#### 4.4.7 三种策略 ROI 对比

```
                  Phase 1            Phase 2              Phase 3
                  重新开始            轻量Checkpoint        完整Checkpoint
─────────────────────────────────────────────────────────────────────────
额外代码量        0                   ~200行                ~800行
开发周期          0                   1人天                 3-5人天
恢复精度          从头重跑             跳过已知结论           精确断点
Token浪费         全量重跑 ~$0.04      部分节省 ~$0.02       接近0
审批状态          ✅ CRD天然保留       ✅                    ✅
维护成本          无                  低                    中高
Bug风险           低(无状态,幂等)     低(只存摘要)          中高(状态一致性)
适合阶段          MVP/第一版          生产稳定后             规模化运营
```

> **面试话术：** "我们基于 ROI 做了分阶段决策。诊断阶段重跑代价小（13 秒，$0.04），工具调用天然幂等，直接重跑。审批状态 CRD 天然持久化。第二版引入轻量 Checkpoint——只保存阶段性发现而非完整对话，恢复时 Prompt 注入让 LLM 不重复已有结论，200 行代码。完整 Checkpoint 评估后 ROI 不足，留在 Roadmap。这不是技术取舍，是工程判断。"

---

### 模块五：性能优化 — 四层策略

#### 4.5.1 总览

```
50 alerts ──▶ [Alert Aggregator]  ← 去重合并(60s窗口) → ~12个
                    │
                    ▼
             [Priority Queue]     ← 按严重度排序(Go heap)
                    │
                    ▼
             [Worker Pool]        ← Goroutine + Semaphore, max=10
              │    │    │
              ▼    ▼    ▼
             [LLM Router]        ← 大小模型分流
              │         │
         Ollama 7B   GPT-4o
         (简单,200ms) (复杂,2s)
```

#### 4.5.2 策略一：告警去重与合并

同一 Deployment 下多个 Pod 同时 OOM，合并为一个诊断任务。60 秒聚合窗口，基于 `namespace/resource/alertType` 做 key。

```go
type AlertAggregator struct {
    window  time.Duration  // 60s
    pending map[string]*AggregatedAlert
    mu      sync.Mutex
}

func (a *AlertAggregator) Ingest(alert *Alert) {
    key := fmt.Sprintf("%s/%s/%s", alert.Namespace, alert.Resource, alert.AlertType)
    a.mu.Lock()
    defer a.mu.Unlock()

    if existing, ok := a.pending[key]; ok {
        existing.Count++
        existing.AffectedPods = append(existing.AffectedPods, alert.PodName)
    } else {
        a.pending[key] = &AggregatedAlert{
            Key: key, FirstAlert: alert, Count: 1,
            AffectedPods: []string{alert.PodName},
        }
        time.AfterFunc(a.window, func() { a.flush(key) })
    }
}
```

#### 4.5.3 策略二：LLM 大小模型分流

70% 常见故障走本地 Ollama 7B（约 200ms），复杂未知故障走 GPT-4o（约 2s）。

```go
type LLMRouter struct {
    localModel  LLMProvider  // Ollama Qwen2.5-7B
    remoteModel LLMProvider  // OpenAI GPT-4o
}

func (r *LLMRouter) Route(alert *AggregatedAlert) LLMProvider {
    if alert.MatchesKnownPattern() || alert.Severity == "warning" {
        return r.localModel
    }
    return r.remoteModel
}
```

误判兜底：如果本地模型诊断后置信度低于阈值（通过 Prompt 让 LLM 自评），自动升级到 GPT-4o 重跑。实测误判率约 5%，额外延迟约 3 秒。

#### 4.5.4 策略三：Goroutine Worker Pool + 背压

```go
type AgentPool struct {
    sem   chan struct{}     // 信号量控制并发数
    queue *PriorityQueue   // 按告警严重度排序
}

func NewAgentPool(maxConcurrency int) *AgentPool {
    return &AgentPool{sem: make(chan struct{}, maxConcurrency)}
}

func (p *AgentPool) Submit(task *DiagnosisTask) {
    p.queue.Push(task)
    go func() {
        p.sem <- struct{}{}        // 获取信号量，满了就阻塞
        defer func() { <-p.sem }() // 释放
        task := p.queue.Pop()
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        NewAgent(task).Run(ctx)
    }()
}
```

#### 4.5.5 策略四：LLM Streaming 响应

不等完整输出，流式解析 Function Call，降低端到端延迟。

```go
func (a *Agent) callLLMStreaming(ctx context.Context, messages []LLMMessage) (<-chan string, error) {
    ch := make(chan string, 64)
    go func() {
        defer close(ch)
        stream, _ := a.llm.CreateChatCompletionStream(ctx, messages)
        for {
            chunk, err := stream.Recv()
            if err == io.EOF { break }
            ch <- chunk.Choices[0].Delta.Content
        }
    }()
    return ch, nil
}
```

#### 4.5.6 性能指标

P95 诊断时间从 40 分钟降到 2.5 分钟。告警风暴（50 告警/分钟）下系统稳定无降级。

> **面试话术：** "四层优化。第一层告警去重，50 个合并到 12 个。第二层大小模型路由，70% 常见故障走本地 7B 模型 200ms 返回，复杂故障走 GPT-4o，误判时自动升级。第三层 Worker Pool 并发控制和优先级调度。第四层 LLM Streaming 流式解析 Function Call。P95 从 40 分钟降到 2.5 分钟。"

---

### 模块六：工程化挑战

#### 4.6.1 防自循环 — 三道防线

```go
func (a *Agent) Run(ctx context.Context) (*DiagnosisReport, error) {
    for step := 0; ; step++ {
        // 防线1: 硬性步数上限 (MaxSteps=15)
        if step >= a.config.MaxSteps {
            return a.forceConclusion("达到最大步数")
        }
        // 防线2: 重复动作检测 (连续3次相同工具+参数)
        if a.detectLoop() {
            return a.forceConclusion("检测到重复动作")
        }
        // 防线3: Context 超时 (5 分钟 deadline)
        select {
        case <-ctx.Done():
            return a.forceConclusion("诊断超时")
        default:
        }

        // Agent Loop: Think → Validate → Act → Observe
        action, _ := a.think(ctx)
        if action.Type == "conclude" {
            return a.conclude(action)
        }
        if err := a.validateToolCall(action.FunctionCall); err != nil {
            a.handleValidationError(err, action.FunctionCall)
            continue
        }
        result, _ := a.act(ctx, action)
        a.observe(result)
    }
}

// 重复检测：连续3次调用相同工具+相同参数，判定为循环
func (a *Agent) detectLoop() bool {
    calls := a.context.ToolCalls
    if len(calls) < 3 { return false }
    last3 := calls[len(calls)-3:]
    return last3[0].Signature() == last3[1].Signature() &&
           last3[1].Signature() == last3[2].Signature()
}
```

#### 4.6.2 防幻觉 — 操作安全四级分级

```go
// 安全分级定义
var toolSafetyMap = map[string]SafetyLevel{
    // ReadOnly → 自动执行
    "get_pod_logs":     SafetyReadOnly,
    "get_events":       SafetyReadOnly,
    "query_prometheus": SafetyReadOnly,
    // LowRisk → 二次确认
    "restart_pod":      SafetyLowRisk,
    "scale_deployment": SafetyLowRisk,
    // HighRisk → 人工审批（Slack/钉钉通知SRE）
    "apply_resource":   SafetyHighRisk,
    // Forbidden → 禁止执行
    "delete_resource":  SafetyForbidden,
    "exec_in_pod":      SafetyForbidden,
}

func (a *Agent) act(ctx context.Context, action *Action) (*ToolResponse, error) {
    level := toolSafetyMap[action.ToolName]
    switch level {
    case SafetyReadOnly:
        return a.router.Dispatch(ctx, action.FunctionCall)
    case SafetyLowRisk:
        a.requestApproval(action)
        return a.waitForApproval(ctx, action)
    case SafetyHighRisk:
        a.notifyAndWait(action) // Slack/钉钉通知 SRE
        return a.waitForApproval(ctx, action)
    case SafetyForbidden:
        return &ToolResponse{
            Success: false,
            Result:  "该操作被安全策略禁止，请换一种修复方案",
        }, nil
    }
    return nil, fmt.Errorf("unknown safety level")
}
```

#### 4.6.3 幻觉校验 — LLM 输出三重验证

```go
func (a *Agent) validateToolCall(call *LLMFunctionCall) error {
    // 检查1: 工具是否存在
    if !a.router.HasTool(call.Name) {
        return fmt.Errorf("不存在的工具: %s", call.Name)
    }
    // 检查2: 参数是否符合 JSON Schema
    schema := a.router.GetSchema(call.Name)
    if err := schema.Validate(call.Arguments); err != nil {
        return fmt.Errorf("参数校验失败: %w", err)
    }
    // 检查3: 参数中的 K8s 资源是否真实存在
    if ns, ok := extractNamespace(call.Arguments); ok {
        if !a.k8sClient.NamespaceExists(ns) {
            return fmt.Errorf("幻觉: namespace %s 不存在", ns)
        }
    }
    return nil
}

// 校验失败：把错误反馈给 LLM 让它自我修正
func (a *Agent) handleValidationError(err error, call *LLMFunctionCall) {
    a.context.Messages = append(a.context.Messages, LLMMessage{
        Role:    "tool",
        Content: fmt.Sprintf("工具调用失败: %s。请检查参数后重试。", err.Error()),
    })
    // 回到 think 步骤让 LLM 重新决策
}
```

#### 4.6.4 Agent Loop 完整状态机

```
START → THINKING (LLM推理决策)
           │
           ├──▶ CONCLUDE (LLM认为诊断完成)
           │
           ▼
        VALIDATE (安全校验 + 幻觉检测)
           │
           ├──▶ 校验失败 → 回到 THINKING (最多重试3次)
           │
           ▼
        EXECUTING (真正执行工具)
           │
           ▼
        OBSERVING (结果写入记忆) ──▶ 回到 THINKING

超时/超步数 → 任意阶段均可强制 CONCLUDE
每一步 OpenTelemetry metrics 上报
Grafana 可视化完整推理链路
```

> **面试话术：** "三层防护。第一层循环检测和硬边界——最多 15 步、5 分钟超时、重复动作检测。第二层操作安全四级分级——只读自动执行、写操作需审批、危险操作禁止，LLM 幻觉出 delete 操作会被拦截并返回错误提示让它换方案。第三层幻觉校验——工具调用必须通过 Schema 验证和 K8s 资源真实性验证，失败回传 LLM 自我修正。整个 Agent Loop 是显式状态机，全链路 OpenTelemetry 追踪。"

---

## 五、面试话术库

### 话术一：项目总览 (30秒)

> "我主导设计了 KubeMinds，一个 K8s-Native 的智能运维 Agent 平台。它以 Operator 模式部署在集群内，当告警触发时自动匹配专家诊断技能，通过 LLM 驱动的 Agent Loop 调用工具链采集日志、事件、指标，推理根因并生成修复方案。上线后将 MTTR 从平均 30 分钟降低到 3 分钟以内。"

### 话术二：Memory 设计

> "记忆分三层。L1 进程内 struct 保存单次诊断上下文。L2 Redis Stream 做 24 小时滑动窗口关联近期故障。L3 PostgreSQL + pgvector 存储历史诊断报告做语义检索。调用 LLM 时分层注入，既控制 token 消耗又保证相关性。选 pgvector 而不是 Milvus 是因为部署在客户集群内要减少依赖，万级数据 pgvector 够用，且 Memory 层是接口抽象可替换。"

### 话术三：工具体系

> "工具体系分三层 Adapter。K8s 内部诊断工具用 gRPC 做进程隔离和高性能调用；外部协作工具通过 MCP 协议复用社区生态，零代码接入；轻量计算用内部 Go 函数。Unified Tool Router 对 Agent 完全透明。上层是 Skill 引擎——Base Skill 覆盖 80% 场景，7 个 Domain Skill 定义差异化专家经验，新增约 20 行 YAML。"

### 话术四：Agent 与 Operator 统一

> "Agent 有状态，Operator 无状态，我们基于 ROI 做了分阶段决策。诊断阶段重跑代价小（13 秒，$0.04），工具调用天然幂等，直接重跑。审批状态 CRD 天然持久化。第二版引入轻量 Checkpoint——只保存阶段性发现而非完整对话，恢复时 Prompt 注入让 LLM 不重复已有结论，200 行代码。完整 Checkpoint 评估后 ROI 不足，留在 Roadmap。这不是技术取舍，是工程判断。"

### 话术五：为什么用 Go

> "如果做 RAG 系统我会选 Python，但 KubeMinds 是 K8s 运维场景——核心是 Operator + Agent Loop + K8s API 调用，这是 Go 的主场。我们用到的 AI 能力就是 LLM API 调用和 pgvector 检索，Go 的 HTTP Client 和 SQL Driver 够用。1500 行代码换来 client-go 原生集成、编译期安全、20MB 单二进制部署、真并发，这笔账划算。"

### 话术六：性能优化

> "四层优化。告警去重 50 合并到 12。大小模型路由，70% 走本地 7B 模型 200ms 返回，误判自动升级。Worker Pool 并发控制和优先级调度。LLM Streaming 流式解析。P95 从 40 分钟降到 2.5 分钟。"

### 话术七：工程化安全

> "三层防护。循环检测和硬边界（15 步、5 分钟、重复检测）。操作安全四级分级（只读自动、写操作审批、危险禁止）。幻觉校验（Schema 验证 + K8s 资源真实性验证，失败回传 LLM 修正）。全链路 OpenTelemetry 追踪。"

---

## 六、面试高频追问及应对

### Q1: "你的 Agent 和直接让 ChatGPT 看日志有什么区别？"

> "三个本质区别。第一，Skill 引擎注入了领域专家经验——决策树引导 LLM 按最优路径排查，不是漫无目的地试。第二，三层记忆让 Agent 能关联历史故障和近期事件，不是每次从零推理。第三，操作安全分级确保 Agent 可以在生产环境自动执行只读诊断，而不只是'建议你去看看日志'。"

### Q2: "pgvector 性能够吗？为什么不用专业向量数据库？"

> "我们的向量数据量级是万级历史故障报告，不是百万级文档检索。pgvector 的 IVFFlat 索引在万级数据上检索延迟 < 10ms，完全满足需求。选它的核心原因是减少客户集群的部署依赖——多数企业已有 PostgreSQL，pgvector 是扩展不是独立服务。Memory 层是接口抽象，未来数据增长可平滑替换。"

### Q3: "MCP 和 gRPC 混用会不会增加复杂度？"

> "不会，因为有 Unified Tool Router 层。Agent 只面对统一的 Dispatch 接口，不感知底层是 gRPC、MCP 还是内部函数。三种 Adapter 各解决不同场景：gRPC 解决 K8s 内部高频调用的性能和隔离需求，MCP 解决外部工具的生态复用需求，Internal 解决轻量计算的简洁性需求。这是按场景选型，不是技术堆砌。"

### Q4: "Operator 重启后 Agent 状态怎么恢复？"

> "基于 ROI 分阶段。诊断阶段重跑代价小（13 秒、$0.04），工具幂等，直接重跑。审批阶段 CRD Status 天然持久化，零额外代码。第二版引入轻量 Checkpoint，只保存发现摘要（不是完整对话），恢复时 Prompt 注入。完整 Checkpoint 评估后 ROI 不足，留 Roadmap。这个决策本身就是工程判断——不为技术而技术。"

### Q5: "大小模型分流的判断标准是什么？误判怎么办？"

> "判断依据有两个：一是告警是否匹配已知 Pattern（有 Domain Skill 的都算已知），二是告警严重度。已知 Pattern + Warning 走本地 7B，其余走 GPT-4o。误判的兜底机制是：如果本地模型诊断后置信度低于阈值（通过 Prompt 让 LLM 自评），自动升级到 GPT-4o 重跑。实测误判率约 5%，误判后的额外延迟约 3 秒，可接受。"

### Q6: "Skill 的决策树和直接写 if-else 规则引擎有什么区别？"

> "决策树不是硬编码的 if-else，它是 LLM 的'排查路线图'。传统规则引擎是确定性的——条件A成立走分支B，否则走C。我们的决策树是引导性的——它告诉 LLM '建议先查内存 limit，再查使用趋势'，但 LLM 可以根据中间结果灵活偏离。比如查日志时发现了决策树没预料到的线索（第三方库 bug），LLM 会自主调整方向追查下去。这是'专家经验引导 + LLM 灵活推理'的结合，比纯规则引擎灵活，比纯 LLM 高效。"

### Q7: "为什么不直接用 LangChain/LangGraph 做 Agent？"

> "如果做 RAG 系统我会毫不犹豫选 Python。但 KubeMinds 是 K8s 运维场景，核心是 Operator + Agent Loop + K8s API 调用——client-go、controller-runtime、Informer 这些是 Go 专属领地，Python 的 K8s 客户端是二等公民。我们实际用到的 AI 能力只有 LLM API 调用和 pgvector 向量检索，200 行 HTTP 封装就搞定了。额外写 1500 行 Go 代码，换来原生集成、编译期安全、20MB 部署产物和真并发，在运维场景这笔账划算。"

---

## 七、简历描述 (STAR 原则)

**项目：KubeMinds — K8s 智能运维 Agent 平台**

公司 K8s 集群 200+ 节点，日均告警 500+，故障定位平均 30 分钟。主导设计基于 LLM 的 K8s-Native 智能诊断 Agent。以 CRD + Operator 构建控制面，设计三层记忆架构（Redis Stream 短期关联 + pgvector 语义检索）沉淀故障经验；构建 gRPC + MCP 混合工具体系统一封装 15+ 诊断工具；引入两层 Skill Engine 将 SRE 经验编码为 YAML 声明式技能包；通过告警聚合、大小模型分流（本地 7B + GPT-4o）、Worker Pool 应对告警风暴；实现操作安全四级分级与幻觉三重校验。上线后 MTTR 降至 3 分钟（P95），夜间告警自动处理率 78%，SRE 人效提升 60%。
