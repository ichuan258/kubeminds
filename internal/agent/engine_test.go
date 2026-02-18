package agent

import (
	"context"
	"fmt"
	"testing"

	"kubeminds/api/v1alpha1"
)

func TestAgent_Run_Success(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Step 0: LLM decides to call a tool
	mockLLM.Responses[0] = &Message{
		Type:    MessageTypeAssistant,
		Content: "I need to check the pod logs.",
		ToolCalls: []ToolCall{
			{
				ID: "call_1",
				Function: FunctionCall{
					Name:      "get_logs",
					Arguments: "{\"pod\":\"test-pod\"}",
				},
			},
		},
	}

	// Step 1: LLM analyzes tool output and concludes
	mockLLM.Responses[1] = &Message{
		Type:    MessageTypeAssistant,
		Content: "The logs show a panic. Suggest restarting the pod.",
	}

	mockTool := &MockTool{
		NameVal: "get_logs",
		DescVal: "Get logs",
		ExecuteFunc: func(ctx context.Context, args string) (string, error) {
			return "panic: index out of range", nil
		},
	}

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil, nil, Skill{})

	// Execute
	result, err := ag.Run(context.Background(), "Diagnose pod failure", true)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RootCause == "" {
		// In our simple implementation, RootCause isn't parsed separately yet, so we check Suggestion
	}

	if result.Suggestion != "The logs show a panic. Suggest restarting the pod." {
		t.Errorf("expected suggestion 'The logs show a panic. Suggest restarting the pod.', got '%s'", result.Suggestion)
	}

	if mockTool.ExecutionCount != 1 {
		t.Errorf("expected tool to be called once, got %d", mockTool.ExecutionCount)
	}
}

func TestAgent_Run_HistoryUpdates(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Step 0: LLM decides to call a tool
	mockLLM.Responses[0] = &Message{
		Type:    MessageTypeAssistant,
		Content: "I need to check the pod logs.",
		ToolCalls: []ToolCall{
			{
				ID: "call_1",
				Function: FunctionCall{
					Name:      "get_logs",
					Arguments: "{\"pod\":\"test-pod\"}",
				},
			},
		},
	}

	// Step 1: LLM analyzes tool output and concludes
	mockLLM.Responses[1] = &Message{
		Type:    MessageTypeAssistant,
		Content: "The logs show a panic. Suggest restarting the pod.",
	}

	mockTool := &MockTool{
		NameVal: "get_logs",
		DescVal: "Get logs",
		ExecuteFunc: func(ctx context.Context, args string) (string, error) {
			return "panic: index out of range", nil
		},
	}

	var history []string
	onStepComplete := func(finding *v1alpha1.Finding, historyEntry string) {
		history = append(history, historyEntry)
	}

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil, onStepComplete, Skill{})

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose pod failure", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify History
	// Expected events:
	// 1. Step 1 (Think)
	// 2. Step 1 (Act)
	// 3. Step 2 (Think)
	// 4. Step 2 (Conclude) - Newly added!

	if len(history) != 4 {
		t.Errorf("expected 4 history entries, got %d: %v", len(history), history)
	}

	// Check for Conclude message
	foundConclude := false
	for _, entry := range history {
		if contains(entry, "(Conclude)") {
			foundConclude = true
			if !contains(entry, "RootCause") || !contains(entry, "Suggestion") {
				t.Errorf("conclude message malformed: %s", entry)
			}
		}
	}
	if !foundConclude {
		t.Error("expected Conclude history entry not found")
	}
}

func TestAgent_Run_TruncatesThink(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Generate a very long thought
	longThought := ""
	for i := 0; i < 1000; i++ {
		longThought += "a"
	}

	// Step 0: Long thought, then finish
	mockLLM.Responses[0] = &Message{
		Type:    MessageTypeAssistant,
		Content: longThought,
	}

	var history []string
	onStepComplete := func(finding *v1alpha1.Finding, historyEntry string) {
		history = append(history, historyEntry)
	}

	ag := NewAgent(mockLLM, []Tool{}, 5, nil, onStepComplete, Skill{})

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify History
	if len(history) < 1 {
		t.Fatal("expected at least 1 history entry")
	}

	thinkEntry := history[0]
	// Should be truncated to ~500 chars + overhead
	if len(thinkEntry) > 600 {
		t.Errorf("history entry too long, expected truncation: len=%d", len(thinkEntry))
	}
	if !contains(thinkEntry, "...") {
		t.Error("expected history entry to contain ellipsis '...'")
	}
}

func TestAgent_Run_MaxStepsExceeded(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Always call a tool with distinct arguments so loop detection doesn't fire first
	for i := 0; i < 5; i++ {
		mockLLM.Responses[i] = &Message{
			Type:    MessageTypeAssistant,
			Content: "Thinking...",
			ToolCalls: []ToolCall{
				{
					ID: fmt.Sprintf("call_%d", i),
					Function: FunctionCall{
						Name:      "get_logs",
						Arguments: fmt.Sprintf("{\"step\":%d}", i), // unique per step
					},
				},
			},
		}
	}

	mockTool := &MockTool{
		NameVal: "get_logs",
		DescVal: "Get logs",
	}

	// Max steps = 3
	ag := NewAgent(mockLLM, []Tool{mockTool}, 3, nil, nil, Skill{})

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose", true)

	// Verify
	if err == nil {
		t.Fatal("expected error for max steps exceeded, got nil")
	}

	expectedErr := "agent exceeded maximum steps (3)"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%v'", expectedErr, err)
	}
}

func TestAgent_Run_LoopDetected(t *testing.T) {
	// Setup: agent always calls same tool with same args -> triggers loop detection
	mockLLM := NewMockLLMProvider()
	for i := 0; i < 5; i++ {
		mockLLM.Responses[i] = &Message{
			Type:    MessageTypeAssistant,
			Content: "Thinking...",
			ToolCalls: []ToolCall{
				{
					ID: fmt.Sprintf("call_%d", i),
					Function: FunctionCall{
						Name:      "get_logs",
						Arguments: "{}",
					},
				},
			},
		}
	}

	mockTool := &MockTool{
		NameVal: "get_logs",
		DescVal: "Get logs",
	}

	ag := NewAgent(mockLLM, []Tool{mockTool}, 10, nil, nil, Skill{})

	_, err := ag.Run(context.Background(), "Diagnose", true)

	if err == nil {
		t.Fatal("expected loop detection error, got nil")
	}
	if !contains(err.Error(), "loop detected") {
		t.Errorf("expected loop detection error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

func TestAgent_Run_LLMFailure(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()
	mockLLM.ErrorTrigger[0] = fmt.Errorf("api rate limit exceeded")

	ag := NewAgent(mockLLM, []Tool{}, 5, nil, nil, Skill{})

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose", true)

	// Verify
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAgent_Run_ToolFailure(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Step 0: Call tool
	mockLLM.Responses[0] = &Message{
		Type:      MessageTypeAssistant,
		ToolCalls: []ToolCall{{ID: "1", Function: FunctionCall{Name: "get_logs"}}},
	}

	// Step 1: Analyze error
	mockLLM.Responses[1] = &Message{
		Type:    MessageTypeAssistant,
		Content: "Tool failed, I give up.",
	}

	mockTool := &MockTool{
		NameVal: "get_logs",
		ExecuteFunc: func(ctx context.Context, args string) (string, error) {
			return "", fmt.Errorf("connection refused")
		},
	}

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil, nil, Skill{})

	// Execute
	result, err := ag.Run(context.Background(), "Diagnose", true)

	// Verify
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Suggestion != "Tool failed, I give up." {
		t.Errorf("unexpected suggestion: %s", result.Suggestion)
	}

	// Check if memory recorded the tool error
	history := ag.memory.GetHistory()
	foundError := false
	for _, msg := range history {
		if msg.Type == MessageTypeTool && msg.Content == "Error executing tool: connection refused" {
			foundError = true
			break
		}
	}

	if !foundError {
		t.Error("expected tool error to be recorded in memory")
	}
}

func TestAgent_Run_SafetyCheck(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Step 0: LLM decides to call a high risk tool
	mockLLM.Responses[0] = &Message{
		Type:    MessageTypeAssistant,
		Content: "I need to delete the pod.",
		ToolCalls: []ToolCall{
			{
				ID: "call_1",
				Function: FunctionCall{
					Name:      "delete_pod",
					Arguments: "{\"pod\":\"test-pod\"}",
				},
			},
		},
	}

	// For the "Execute with approved=false" test, the loop will detect ErrWaitingForApproval and exit.
	// Step 1: Mock response for the second run (when approved)
	// IMPORTANT: When the agent restarts with approved=true, it starts from step 0 again (in this test harness).
	// However, NewMockLLMProvider resets its CallCount when created.
	// We are reusing the SAME mockLLM instance for both runs.
	// Run 1 (unapproved): CallCount 0 -> Returns Response[0] -> Agent stops. CallCount becomes 1.
	// Run 2 (approved): CallCount 1 -> Needs Response[1].
	//
	// So Response[1] should be the SAME as Response[0] because the agent is retrying the same operation!
	// After that, the tool executes. Then Agent calls LLM again with tool output. That will be Response[2].

	mockLLM.Responses[1] = mockLLM.Responses[0] // Retry the tool call

	mockLLM.Responses[2] = &Message{
		Type:    MessageTypeAssistant,
		Content: "Pod deleted successfully.",
	}

	mockTool := &MockTool{
		NameVal:        "delete_pod",
		DescVal:        "Delete a pod",
		SafetyLevelVal: SafetyLevelHighRisk,
		ExecuteFunc: func(ctx context.Context, args string) (string, error) {
			return "pod deleted", nil
		},
	}

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil, nil, Skill{})

	// Execute with approved=false
	_, err := ag.Run(context.Background(), "Fix pod", false)

	// Verify
	if err == nil {
		t.Fatal("expected error for unapproved high risk tool, got nil")
	}

	if _, ok := err.(*ErrWaitingForApproval); !ok {
		t.Errorf("expected ErrWaitingForApproval, got %T: %v", err, err)
	}

	if mockTool.ExecutionCount != 0 {
		t.Errorf("expected tool NOT to be executed, got count %d", mockTool.ExecutionCount)
	}

	// Execute with approved=true
	mockTool.ExecutionCount = 0
	_, err = ag.Run(context.Background(), "Fix pod", true)

	if err != nil {
		t.Fatalf("unexpected error when approved: %v", err)
	}

	if mockTool.ExecutionCount != 1 {
		t.Errorf("expected tool to be executed, got count %d", mockTool.ExecutionCount)
	}
}
