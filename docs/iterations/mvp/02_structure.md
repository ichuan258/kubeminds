# KubeMinds MVP 目录结构建议

基于 Go Operator 和 Agent 模式的简化结构。

```
kubeminds/
├── api/
│   └── v1alpha1/            # CRD 定义 (DiagnosisTask, API types)
│       └── diagnosistask_types.go
├── cmd/
│   └── manager/             # 二进制入口
│       └── main.go
├── config/                  # Kustomize 配置 (crd, rbac, manager)
├── internal/
│   ├── controller/          # Operator 控制器逻辑
│   │   └── diagnosistask_controller.go
│   ├── agent/               # Agent 核心引擎
│   │   ├── engine.go        # Agent Loop 实现
│   │   ├── memory.go        # L1 简单内存实现
│   │   └── types.go         # Agent 内部上下文定义
│   ├── llm/                 # LLM 客户端适配
│   │   ├── client.go        # OpenAI/Ollama 统一接口
│   │   └── prompt.go        # 基础 System Prompt 模板
│   └── tools/               # 工具集实现
│       ├── router.go        # 简单工具路由
│       ├── k8s_logs.go      # Pod 日志获取工具
│       ├── k8s_events.go    # K8s 事件获取工具
│       └── k8s_spec.go      # Pod Spec 获取工具
├── deploy/                  # 快速部署 YAML (MVP)
│   └── bundle.yaml
├── go.mod
└── go.sum
```

## 关键文件说明

*   `api/v1alpha1/diagnosistask_types.go`: 定义诊断任务的数据结构，MVP 仅需 `Spec.Target` (Namespace, PodName) 和 `Status.Report`。
*   `internal/controller/diagnosistask_controller.go`: 监听 CR 变化，触发 Agent 运行，处理超时和最终状态更新。
*   `internal/agent/engine.go`: 核心循环 `Think -> Act -> Observe` 的实现。
*   `internal/tools/router.go`: 简单的 map 结构，将 LLM 的函数名映射到 Go 函数。
