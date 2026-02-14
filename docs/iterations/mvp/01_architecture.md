# KubeMinds MVP 版本架构图

MVP 版本聚焦于验证核心链路：**CRD 触发 -> Agent 诊断 -> LLM 推理 -> 工具执行 -> 结果反馈**。暂不包含复杂的分布式记忆、多模态工具路由和高级技能引擎。

```mermaid
graph TD
    User[运维人员] -->|创建| CR[DiagnosisTask CR]
    CR -->|Watch| Operator[KubeMinds Operator]

    subgraph "KubeMinds MVP Core"
        Operator -->|启动| AgentLoop[Agent Loop Engine]

        AgentLoop -->|1. Think| LLM[LLM Client (OpenAI/Ollama)]
        LLM -->|2. Function Call| AgentLoop

        AgentLoop -->|3. Act| ToolRouter[Simple Tool Router]

        ToolRouter -->|调用| K8sClient[K8s Client (client-go)]
        K8sClient -->|查日志/事件/Spec| K8sCluster[K8s Cluster]

        K8sCluster -->|返回数据| ToolRouter
        ToolRouter -->|4. Observe| AgentLoop

        AgentLoop -->|重复 Think-Act-Observe| AgentLoop

        AgentLoop -->|5. Conclude| Report[生成诊断报告]
    end

    Report -->|更新 Status| CR
```

## 核心流程 (MVP)

1.  **触发**: 用户创建一个 `DiagnosisTask` CR，指定要诊断的 Pod 和 Namespace。
2.  **调度**: Operator 监听到 CR 创建，启动一个 Goroutine 运行 Agent。
3.  **循环**:
    *   **思考 (Think)**: Agent 将当前上下文（CR 信息 + 已有对话）发送给 LLM。
    *   **决策**: LLM 决定调用工具（如 `get_pod_logs`）。
    *   **执行 (Act)**: Agent 调用本地 Go 函数执行 K8s 查询。
    *   **观察 (Observe)**: 获取工具返回结果（如日志内容），追加到上下文。
4.  **产出**: 达到目标或步数限制后，LLM 生成最终诊断结论，Operator 将其更新到 CR 的 `Status.Report` 字段。
