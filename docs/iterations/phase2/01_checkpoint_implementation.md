# Phase 2: Checkpoint & Skill Engine 实现记录

## 1. 目标
解决 MVP 阶段的两个核心问题：
1.  **Context 溢出与状态丢失**: Agent 运行过程中无中间状态保存，Operator 重启或 Context 满时丢失进度。
2.  **诊断准确率**: 依赖通用 Prompt，缺乏领域专家知识引导。

## 2. 变更内容 (已实现 Checkpoint)

### 2.1 CRD 变更 (`api/v1alpha1/diagnosistask_types.go`)
在 `DiagnosisTaskStatus` 中增加了 `Checkpoint` 字段：
```go
type Finding struct {
    Step      int    `json:"step"`
    ToolName  string `json:"toolName"`
    ToolArgs  string `json:"toolArgs"`
    Summary   string `json:"summary"` // 截断或 LLM 总结的输出
    Timestamp string `json:"timestamp"`
}

type DiagnosisTaskStatus struct {
    // ...
    Checkpoint []Finding `json:"checkpoint,omitempty"`
}
```

### 2.2 Agent 增强 (`internal/agent/engine.go`)
1.  **OnStepComplete 回调**: Agent 在每次 Tool 执行后，调用此回调。
2.  **Restore 机制**: `Restore(findings)` 方法将历史 Findings 转换为 Prompt 注入 Memory，实现断点续传。
3.  **Summary**: 暂时使用简单的字符串截断 (200 chars) 作为 Summary，后续可升级为 LLM 总结。

### 2.3 Controller 增强 (`internal/controller/diagnosistask_controller.go`)
1.  **ActiveAgents 管理**: 引入 `sync.Map` 记录本地运行的 Agent (含 CancelFunc)，防止重复运行。
2.  **Resume 逻辑**:
    *   如果 Task 处于 `Running` 状态但本地无 Active Agent（暗示 Controller 重启过），则触发 Resume。
    *   启动 Agent 前读取 `Status.Checkpoint` 并调用 `Restore`。
3.  **实时更新**: 通过 `onStepComplete` 回调，实时将 Finding 更新到 Etcd。

## 3. 待办事项 (To-Do)
*   [ ] **Skill Engine**: 实现 Skill 定义、加载与匹配。
*   [ ] **LLM Summary**: 在 Checkpoint 时使用轻量级 LLM 生成高质量摘要。
*   [ ] **Safety**: 实现操作安全分级。

---
*Created: 2026-02-14*
