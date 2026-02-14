# Phase 2: Skill Engine 实现记录

## 1. 目标
*   提升诊断的专业性和准确性。
*   通过 Tool Restriction 减少 LLM 幻觉。

## 2. 变更内容 (已实现 Skill Engine)

### 2.1 Skill 定义 (`internal/agent/skill.go`)
定义了 `Skill` 结构体和两个内置 Skill：
*   **BaseSkill**: 通用 K8s 排查，无工具限制。
*   **OOMSkill**: 专注于 OOM 问题，注入了 "Check Limit -> Check Metrics -> Check Leak" 的 System Prompt，并限定只能使用 3 个相关工具。

### 2.2 Skill Manager (`internal/agent/skill_manager.go`)
实现了简单的 `SkillManager`，目前采用静态匹配逻辑（默认 BaseSkill）。
*   `Register()`: 注册新 Skill。
*   `Match()`: 根据 `DiagnosisTask` 匹配最佳 Skill（MVP 阶段暂未实现基于 labels 的智能匹配）。

### 2.3 Agent 集成 (`internal/agent/engine.go`)
*   `NewAgent` 增加了 `Skill` 参数。
*   **Prompt Injection**: 如果 Skill 定义了 System Prompt，会自动注入到对话开头。
*   **Tool Filter**: 如果 Skill 定义了 `AllowedTools` 白名单，Agent 只会向 LLM 暴露这些工具。

### 2.4 Controller 集成 (`internal/controller/diagnosistask_controller.go`)
*   在 Reconcile Loop 中实例化 `SkillManager`。
*   在启动 Agent 前调用 `skillManager.Match(task)` 获取 Skill。
*   将匹配到的 Skill 传递给 Agent。

## 3. 待办事项 (To-Do)
*   [ ] **Smart Matching**: 在 `Match` 方法中实现基于 `Alert Labels` (e.g., `reason=OOMKilled`) 的匹配逻辑。
*   [ ] **Dynamic Skills**: 支持从 ConfigMap 或外部文件加载 Skill 定义，而非硬编码。

---
*Created: 2026-02-14*
