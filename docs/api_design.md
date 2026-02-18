# KubeMinds RESTful API Design

## 1. Overview
This document describes the RESTful API for the KubeMinds SRE Dashboard. The API provides endpoints to manage DiagnosisTasks, Skills, and Tool Configurations.

- **Base URL**: `/api/v1`
- **Port**: 8081 (default)
- **Format**: JSON
- **Auth**: TBD (MVP: None/Basic)

## 2. Diagnosis Tasks

### 2.1 List Tasks
Retrieve a list of DiagnosisTasks.

- **GET** `/tasks`
- **Query Parameters**:
  - `status`: Filter by status (e.g., `Running`, `Failed`, `Completed`).
  - `namespace`: Filter by namespace.
  - `limit`: Max records (default: 50).
- **Response**:
```json
{
  "items": [
    {
      "name": "oom-nginx-123",
      "namespace": "default",
      "target": {
        "kind": "Pod",
        "name": "nginx-pod"
      },
      "status": {
        "phase": "Running",
        "startedAt": "2024-02-15T10:00:00Z",
        "matchedSkill": "oom_diagnosis"
      }
    }
  ],
  "total": 1
}
```

### 2.2 Get Task Details
Retrieve full details of a specific task, including the Agent's execution history (checkpoint/findings).

- **GET** `/tasks/:namespace/:name`
- **Response**:
```json
{
  "metadata": { ... },
  "spec": { ... },
  "status": {
    "phase": "WaitingApproval",
    "report": { ... },
    "checkpoint": [
      {
        "step": 1,
        "tool": "get_pod_logs",
        "summary": "Found OOM error in logs",
        "timestamp": "2024-02-15T10:01:00Z"
      }
    ],
    "history": [ ... ]
  }
}
```

### 2.3 Create Task (Manual Trigger)
Manually trigger a diagnosis task.

- **POST** `/tasks`
- **Body**:
```json
{
  "target": {
    "namespace": "default",
    "name": "nginx-pod",
    "kind": "Pod"
  },
  "alertContext": {
    "labels": {
      "reason": "OOMKilled"
    }
  }
}
```
- **Response**: `201 Created`

### 2.4 Approve/Reject Task
Approve a task that is in `WaitingApproval` state (e.g., for remediation).

- **POST** `/tasks/:namespace/:name/approve`
- **Body**:
```json
{
  "approved": true,
  "comment": "Proceed with restart"
}
```

### 2.5 Stop Task
Terminate a running task.

- **DELETE** `/tasks/:namespace/:name`
- **Response**: `204 No Content`

## 3. Skills

### 3.1 List Skills
List all available skills (Base + Domain).

- **GET** `/skills`
- **Response**:
```json
{
  "items": [
    {
      "name": "base_skill",
      "description": "General troubleshooting",
      "type": "Base"
    },
    {
      "name": "oom_diagnosis",
      "description": "Diagnose OOMKilled issues",
      "type": "Domain"
    }
  ]
}
```

### 3.2 Get Skill Details
Get the YAML definition of a skill.

- **GET** `/skills/:name`
- **Response**:
```json
{
  "name": "oom_diagnosis",
  "definition_yaml": "..."
}
```

### 3.3 Update Skill
Update a Domain Skill definition.

- **PUT** `/skills/:name`
- **Body**:
```json
{
  "definition_yaml": "..."
}
```

## 4. Tool Configuration

### 4.1 Get Tool Config
Get current tool configurations, including MCP servers and safety levels.

- **GET** `/config/tools`
- **Response**:
```json
{
  "mcp_servers": [
    {
      "name": "slack",
      "status": "connected"
    }
  ],
  "safety_policies": {
    "delete_pod": "LowRisk",
    "restart_deployment": "HighRisk"
  }
}
```

### 4.2 Update Tool Config
Update tool safety levels.

- **PUT** `/config/tools`
- **Body**:
```json
{
  "safety_policies": {
    "delete_pod": "HighRisk"
  }
}
```
