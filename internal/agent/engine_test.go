package agent

import (
	"context"
	"fmt"
	"testing"
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

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil)

	// Execute
	result, err := ag.Run(context.Background(), "Diagnose pod failure")

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

func TestAgent_Run_MaxStepsExceeded(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()

	// Always call a tool, never finish
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

	// Max steps = 3
	ag := NewAgent(mockLLM, []Tool{mockTool}, 3, nil)

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose")

	// Verify
	if err == nil {
		t.Fatal("expected error for max steps exceeded, got nil")
	}

	expectedErr := "agent exceeded maximum steps (3)"
	if err.Error() != expectedErr {
		t.Errorf("expected error '%s', got '%v'", expectedErr, err)
	}
}

func TestAgent_Run_LLMFailure(t *testing.T) {
	// Setup
	mockLLM := NewMockLLMProvider()
	mockLLM.ErrorTrigger[0] = fmt.Errorf("api rate limit exceeded")

	ag := NewAgent(mockLLM, []Tool{}, 5, nil)

	// Execute
	_, err := ag.Run(context.Background(), "Diagnose")

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

	ag := NewAgent(mockLLM, []Tool{mockTool}, 5, nil)

	// Execute
	result, err := ag.Run(context.Background(), "Diagnose")

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
