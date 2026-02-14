# KubeMinds 模块规划 (MVP vs Future)

## 核心模块 (MVP 必需)

### 1. DiagnosisTask CRD & Controller
*   **功能**: 定义和管理诊断任务生命周期。
*   **输入**: CRD Spec (`namespace`, `pod_name`, `target_resource`)。
*   **输出**: CRD Status (`phase`, `report`, `error`)。
*   **逻辑**: 仅支持 "One-Shot" 或简单轮询，不包含复杂的队列去重。

### 2. Agent Loop Engine (Basic)
*   **功能**: 简单的 ReAct 循环 (Think -> Act -> Observe)。
*   **限制**: 最大步数限制 (MaxSteps=10)，超时控制 (Timeout=5m)。
*   **内存**: 仅 L1 (In-Memory)，仅保存本次会话上下文，进程重启即丢失。

### 3. LLM Client Wrapper
*   **功能**: 统一 LLM API 调用接口。
*   **支持**:
    *   **OpenAI**: GPT-4o, GPT-3.5
    *   **Gemini**: Gemini 1.5 Pro/Flash
    *   **Moonshot (Kimi)**: moonshot-v1-8k/32k
    *   **Ollama**: Qwen2.5, Llama3 (本地部署)
*   **特性**: 支持 Function Calling 格式解析，提供统一的 `LLMProvider` 接口。

### 4. Basic Tool Set (K8s Internal)
*   **`get_pod_logs`**: 获取指定 Pod/Container 的日志 (支持 tail lines)。
*   **`get_pod_events`**: 获取与 Pod 相关的 K8s Events。
*   **`get_pod_spec`**: 获取 Pod 的完整 YAML 定义 (用于检查资源限制、镜像版本等)。
*   **`list_pods`**: 列出 Namespace 下的 Pods。

### 5. Simple Prompt Manager
*   **功能**: 提供硬编码的 System Prompt。
*   **内容**: "你是一个 Kubernetes 专家，请根据提供的工具诊断问题..."

---

## 优化模块 (未来迭代建议)

### 1. Memory System (L2/L3)
*   **L2 (Short-Term)**: Redis Stream 集成，用于关联同一 Namespace 下近期的类似告警。
*   **L3 (Long-Term)**: Postgres + pgvector 集成，用于存储和检索历史故障报告（RAG）。

### 2. Advanced Skill Engine
*   **功能**: 基于 YAML 定义的决策树 (Decision Tree) 引擎。
*   **价值**: 引导 LLM 进行更规范的排查，而不是完全依赖其自由发挥，减少 Token 消耗和幻觉。

### 3. Unified Tool Router (gRPC + MCP)
*   **gRPC**: 将重型 K8s 工具拆分为独立 Sidecar，提高安全性和隔离性。
*   **MCP**: 支持 Model Context Protocol，接入外部工具 (Slack, GitHub, Prometheus, Grafana)。

### 4. Alert Aggregator
*   **功能**: 告警风暴抑制。
*   **逻辑**: 60秒窗口内相同 Label 的告警合并为一个 DiagnosisTask。

### 5. Safety & Approval Workflow
*   **功能**: 人机协同 (Human-in-the-loop)。
*   **逻辑**: 敏感操作 (如 `delete_pod`) 需人工在 CRD 中 approve 才能执行。
