package agent

import (
	"context"
	"fmt"
	"log/slog"
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
	onStepComplete func(v1alpha1.Finding)
	skill          Skill
}

// NewAgent creates a new BaseAgent
func NewAgent(llm LLMProvider, tools []Tool, maxSteps int, logger *slog.Logger, onStepComplete func(v1alpha1.Finding), skill Skill) *BaseAgent {
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
func (a *BaseAgent) Run(ctx context.Context, goal string) (*Result, error) {
	a.logger.Info("Starting agent run", "goal", goal, "skill", a.skill.Name)

	// Initialize memory with the goal
	// If memory is already populated (e.g. via Restore), this appends to it.
	a.memory.AddUserMessage(fmt.Sprintf("Diagnosis Goal: %s", goal))

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

		// Add assistant response to memory
		if len(response.ToolCalls) > 0 {
			a.memory.AddAssistantToolCall(response.ToolCalls)
		} else {
			a.memory.AddAssistantMessage(response.Content)
		}

		// Check if we should stop (no tool calls and has content)
		if len(response.ToolCalls) == 0 {
			a.logger.Info("Agent decided to finish")
			return &Result{
				RootCause:  "See history", // In a real implementation, we'd parse this from the content
				Suggestion: response.Content,
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
				toolOutput, toolErr = selectedTool.Execute(ctx, toolCall.Function.Arguments)
				if toolErr != nil {
					toolOutput = fmt.Sprintf("Error executing tool: %v", toolErr)
				}
			}

			// Observe: Add tool output to memory
			a.memory.AddToolOutput(toolCall.ID, toolOutput)

			// Checkpoint: Notify listener
			if a.onStepComplete != nil {
				// For MVP, simplistic summary: truncate output
				summary := toolOutput
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}

				finding := v1alpha1.Finding{
					Step:      step + 1, // Human readable step
					ToolName:  toolCall.Function.Name,
					ToolArgs:  toolCall.Function.Arguments,
					Summary:   summary,
					Timestamp: time.Now().Format(time.RFC3339),
				}
				a.onStepComplete(finding)
			}
		}
	}

	return nil, fmt.Errorf("agent exceeded maximum steps (%d)", a.maxSteps)
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
