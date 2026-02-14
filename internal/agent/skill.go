package agent

// Skill defines a specific diagnosis capability (e.g., OOM Diagnosis, CrashLoopBackOff Diagnosis)
type Skill struct {
	// Name of the skill (e.g., "oom_diagnosis")
	Name string
	// Description of what this skill does
	Description string
	// SystemPrompt to inject domain expert knowledge
	SystemPrompt string
	// AllowedTools lists the names of tools this skill is allowed to use.
	// If empty, all tools are allowed.
	AllowedTools []string
}

// Built-in skills
var (
	// BaseSkill is the default skill for general troubleshooting
	BaseSkill = Skill{
		Name:        "base_skill",
		Description: "General Kubernetes troubleshooting skill",
		SystemPrompt: `You are a Kubernetes Expert Agent. Your goal is to diagnose issues in a K8s cluster.
You have access to a set of tools to gather information.
Follow this process:
1. Think: Analyze the current situation and decide what information you need.
2. Act: Execute a tool to gather that information.
3. Observe: Analyze the tool output.
4. Repeat until you identify the root cause.
5. Conclude: Provide a Root Cause and a Suggestion.`,
		AllowedTools: nil, // All tools allowed
	}

	// OOMSkill focuses on memory issues
	OOMSkill = Skill{
		Name:        "oom_diagnosis",
		Description: "Specialized skill for diagnosing OOMKilled pods",
		SystemPrompt: `You are a Kubernetes Memory Expert. You are diagnosing a Pod that was OOMKilled.
Focus your investigation on:
1. Check the Pod's memory limit in its Spec.
2. Check the container's actual memory usage (if metrics are available) or look for "Out of Memory" logs.
3. Determine if the limit is too tight or if there is a memory leak.
Do NOT suggest increasing limits immediately; first identify why it is consuming so much memory.`,
		AllowedTools: []string{"get_pod_logs", "get_pod_events", "get_pod_spec"},
	}
)
