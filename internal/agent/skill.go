package agent

// TriggerRule defines conditions under which a skill should be activated
type TriggerRule struct {
	AlertName string            `yaml:"alert_name"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// MemoryPolicy defines how the agent should handle memory for this skill
type MemoryPolicy struct {
	ShortTermWindow string   `yaml:"short_term_window"` // e.g. "1h"
	LongTermTopK    int      `yaml:"long_term_top_k"`
	RelevantMetrics []string `yaml:"relevant_metrics"`
}

// Skill defines a specific diagnosis capability (e.g., OOM Diagnosis, CrashLoopBackOff Diagnosis)
type Skill struct {
	// Name of the skill (e.g., "oom_diagnosis")
	Name string `yaml:"name"`
	// Description of what this skill does
	Description string `yaml:"description"`
	// Parent skill to inherit from (optional)
	Parent string `yaml:"parent,omitempty"`
	// Triggers that activate this skill
	Triggers []TriggerRule `yaml:"triggers,omitempty"`
	// SystemPrompt to inject domain expert knowledge
	SystemPrompt string `yaml:"system_prompt"`
	// AllowedTools lists the names of tools this skill is allowed to use.
	// If empty, all tools are allowed.
	AllowedTools []string `yaml:"allowed_tools,omitempty"`
	// MemoryPolicy for this skill
	MemoryPolicy *MemoryPolicy `yaml:"memory_policy,omitempty"`
}

// MergeWith merges a domain skill into a base skill
func (base *Skill) MergeWith(domain *Skill) *Skill {
	merged := *base
	merged.Name = domain.Name
	merged.Description = domain.Description
	merged.Parent = domain.Parent
	merged.Triggers = domain.Triggers

	// Merge System Prompts
	if domain.SystemPrompt != "" {
		merged.SystemPrompt = base.SystemPrompt + "\n\n" + domain.SystemPrompt
	}

	// Override Tools if specified
	if len(domain.AllowedTools) > 0 {
		merged.AllowedTools = domain.AllowedTools
	}

	// Override Memory Policy
	if domain.MemoryPolicy != nil {
		merged.MemoryPolicy = domain.MemoryPolicy
	}

	return &merged
}

// Built-in skills (kept for backward compatibility during refactor, but can be removed later)
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
4. Do NOT suggest increasing limits immediately; first identify why it is consuming so much memory.`,
		AllowedTools: []string{"get_pod_logs", "get_pod_events", "get_pod_spec"},
	}
)
