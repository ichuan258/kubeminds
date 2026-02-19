package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"kubeminds/api/v1alpha1"
)

// BaseAgent implements the Agent interface
type BaseAgent struct {
	llm            LLMProvider
	tools          []Tool
	memory         Memory
	maxSteps       int
	logger         *slog.Logger
	onStepComplete func(*v1alpha1.Finding, string)
	skill          Skill
}

// NewAgent creates a new BaseAgent
func NewAgent(llm LLMProvider, tools []Tool, maxSteps int, logger *slog.Logger, onStepComplete func(*v1alpha1.Finding, string), skill Skill) *BaseAgent {
	if logger == nil {
		logger = slog.Default()
	}

	// Filter tools based on skill
	var availableTools []Tool
	if len(skill.AllowedTools) == 0 {
		// All tools allowed
		availableTools = tools
	} else {
		allowed := make(map[string]bool)
		for _, name := range skill.AllowedTools {
			allowed[name] = true
		}
		for _, tool := range tools {
			if allowed[tool.Name()] {
				availableTools = append(availableTools, tool)
			}
		}
	}

	agent := &BaseAgent{
		llm:            llm,
		tools:          availableTools,
		memory:         NewL1Memory(),
		maxSteps:       maxSteps,
		logger:         logger,
		onStepComplete: onStepComplete,
		skill:          skill,
	}

	// Inject Skill System Prompt
	if skill.SystemPrompt != "" {
		agent.memory.AddUserMessage(fmt.Sprintf("SYSTEM INSTRUCTION: %s", skill.SystemPrompt))
	}

	return agent
}

// Run executes the agent loop for a given goal
func (a *BaseAgent) Run(ctx context.Context, goal string, approved bool) (*Result, error) {
	a.logger.Info("Starting agent run", "goal", goal, "skill", a.skill.Name, "approved", approved)

	// Initialize memory with the goal
	// If memory is already populated (e.g. via Restore), this appends to it.
	a.memory.AddUserMessage(fmt.Sprintf("Diagnosis Goal: %s\n\nWhen you have enough information to conclude, respond with:\nRoot Cause: <concise root cause>\nSuggestion: <actionable remediation>", goal))

	// recentFindings tracks per-step findings for loop detection
	var recentFindings []v1alpha1.Finding

	for step := 0; step < a.maxSteps; step++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		a.logger.Info("Executing step", "step", step+1)

		// Think: Call LLM
		response, err := a.llm.Chat(ctx, a.memory.GetHistory(), a.tools)
		if err != nil {
			return nil, fmt.Errorf("failed to chat with LLM: %w", err)
		}

		// Notify status update with Think (LLM thought)
		if a.onStepComplete != nil {
			thought := response.Content
			if len(thought) > 500 {
				thought = thought[:500] + "..."
			}
			a.onStepComplete(nil, fmt.Sprintf("Step %d (Think): %s", step+1, thought))
		}

		// Add assistant response to memory
		if len(response.ToolCalls) > 0 {
			a.memory.AddAssistantToolCall(response.ToolCalls)
		} else {
			a.memory.AddAssistantMessage(response.Content)
		}

		// Check if we should stop (no tool calls and has content)
		if len(response.ToolCalls) == 0 {
			a.logger.Info("Agent decided to finish")
			rootCause, suggestion := a.extractRootCause(response.Content)

			if a.onStepComplete != nil {
				a.onStepComplete(nil, fmt.Sprintf("Step %d (Conclude): RootCause: %s | Suggestion: %s", step+1, rootCause, suggestion))
			}

			return &Result{
				RootCause:  rootCause,
				Suggestion: suggestion,
			}, nil
		}

		// Act: Execute tools
		for _, toolCall := range response.ToolCalls {
			a.logger.Info("Executing tool", "tool", toolCall.Function.Name)

			var toolOutput string
			var toolErr error

			// Find the tool
			var selectedTool Tool
			for _, t := range a.tools {
				if t.Name() == toolCall.Function.Name {
					selectedTool = t
					break
				}
			}

			if selectedTool == nil {
				toolOutput = fmt.Sprintf("Error: Tool %s not found", toolCall.Function.Name)
			} else {
				// Safety Check
				safetyLevel := selectedTool.SafetyLevel()
				if safetyLevel == SafetyLevelForbidden {
					toolErr = &ErrToolForbidden{ToolName: selectedTool.Name()}
					a.logger.Warn("Tool forbidden", "tool", selectedTool.Name())
					// We don't execute, and we return error immediately?
					// Or do we feed it back to LLM?
					// For Forbidden, we probably feed it back so LLM can try something else.
					// But for MVP let's feed it back as tool error output.
					toolOutput = fmt.Sprintf("Error: Tool %s is forbidden by safety policy.", selectedTool.Name())
				} else if safetyLevel == SafetyLevelHighRisk && !approved {
					// Blocking required
					a.logger.Warn("Tool requires approval", "tool", selectedTool.Name())
					// We must abort the run and signal the controller
					return nil, &ErrWaitingForApproval{ToolName: selectedTool.Name()}
				} else {
					toolOutput, toolErr = selectedTool.Execute(ctx, toolCall.Function.Arguments)
					if toolErr != nil {
						toolOutput = fmt.Sprintf("Error executing tool: %v", toolErr)
					}
				}
			}

			// Observe: Add tool output to memory
			a.memory.AddToolOutput(toolCall.ID, toolOutput)

			// Checkpoint: Notify listener and track finding for loop detection
			summary := toolOutput
			if len(summary) > 200 {
				summary = summary[:200] + "..."
			}
			finding := v1alpha1.Finding{
				Step:      step + 1,
				ToolName:  toolCall.Function.Name,
				ToolArgs:  toolCall.Function.Arguments,
				Summary:   summary,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			recentFindings = append(recentFindings, finding)

			if a.onStepComplete != nil {
				a.onStepComplete(&finding, fmt.Sprintf("Step %d (Act): %s(%s) -> %s", step+1, toolCall.Function.Name, toolCall.Function.Arguments, summary))
			}
		}

		// Loop detection: abort if the same tool+args repeats 3 consecutive times
		if a.detectLoop(recentFindings, 3) {
			last := recentFindings[len(recentFindings)-1]
			return nil, fmt.Errorf("agent loop detected: tool %q called with identical arguments 3 consecutive times, aborting to prevent infinite token consumption", last.ToolName)
		}
	}

	return nil, fmt.Errorf("agent exceeded maximum steps (%d)", a.maxSteps)
}

// detectLoop returns true if the last windowSize findings all called the same tool with the same args.
func (a *BaseAgent) detectLoop(findings []v1alpha1.Finding, windowSize int) bool {
	if len(findings) < windowSize {
		return false
	}
	tail := findings[len(findings)-windowSize:]
	first := tail[0]
	for _, f := range tail[1:] {
		if f.ToolName != first.ToolName || f.ToolArgs != first.ToolArgs {
			return false
		}
	}
	return true
}

// extractRootCause parses the LLM final response for "Root Cause:" and "Suggestion:" markers.
// Falls back to using the first sentence as root cause and the full content as suggestion.
func (a *BaseAgent) extractRootCause(content string) (rootCause, suggestion string) {
	var rootCauseLines, suggestionLines []string
	inRootCause, inSuggestion := false, false

	for _, line := range strings.Split(content, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(lower, "root cause:") || strings.HasPrefix(lower, "根因:"):
			inRootCause, inSuggestion = true, false
			if val := strings.TrimSpace(line[strings.Index(line, ":")+1:]); val != "" {
				rootCauseLines = append(rootCauseLines, val)
			}
		case strings.HasPrefix(lower, "suggestion:") || strings.HasPrefix(lower, "建议:") || strings.HasPrefix(lower, "remediation:"):
			inSuggestion, inRootCause = true, false
			if val := strings.TrimSpace(line[strings.Index(line, ":")+1:]); val != "" {
				suggestionLines = append(suggestionLines, val)
			}
		case inRootCause:
			rootCauseLines = append(rootCauseLines, line)
		case inSuggestion:
			suggestionLines = append(suggestionLines, line)
		}
	}

	if len(rootCauseLines) > 0 {
		return strings.TrimSpace(strings.Join(rootCauseLines, "\n")),
			strings.TrimSpace(strings.Join(suggestionLines, "\n"))
	}

	// Fallback: first sentence as root cause, full content as suggestion
	if idx := strings.IndexByte(content, '.'); idx >= 0 {
		return strings.TrimSpace(content[:idx]), strings.TrimSpace(content)
	}
	return strings.TrimSpace(content), strings.TrimSpace(content)
}

// InjectContext adds a user message to the agent's memory before Run() is called.
// The controller uses this to inject L2 (recent alert events) and L3 (historical
// similar diagnoses) context retrieved from external stores.
func (a *BaseAgent) InjectContext(msg string) {
	a.memory.AddUserMessage(msg)
}

// Restore restores the agent's memory from a list of findings
func (a *BaseAgent) Restore(findings []v1alpha1.Finding) {
	if len(findings) == 0 {
		return
	}

	a.logger.Info("Restoring from checkpoint", "findings_count", len(findings))

	var summary string
	summary += "Previous diagnosis findings (restored from checkpoint):\n"
	for _, f := range findings {
		summary += fmt.Sprintf("- Step %d [%s]: %s\n", f.Step, f.ToolName, f.Summary)
	}

	// Inject as User message for MVP. Ideally this would be System message or specialized context injection.
	a.memory.AddUserMessage(summary)
}
