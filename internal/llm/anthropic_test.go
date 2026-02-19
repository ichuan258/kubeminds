package llm

import (
	"context"
	"encoding/json"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"kubeminds/internal/agent"
)

// TestConvertTools verifies that agent.Tool definitions are correctly mapped to
// Anthropic's ToolUnionParam format, including name, description, and JSON schema.
func TestConvertTools_Basic(t *testing.T) {
	tools := []agent.Tool{
		&fakeToolForAnthropicTest{
			name:        "get_pod_logs",
			description: "Retrieve logs from a pod",
			schema: `{
				"type": "object",
				"properties": {
					"namespace": {"type": "string"},
					"podName":   {"type": "string"}
				},
				"required": ["namespace", "podName"]
			}`,
		},
	}

	result, err := convertTools(tools)
	if err != nil {
		t.Fatalf("convertTools() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("convertTools() len = %d, want 1", len(result))
	}

	tp := result[0].OfTool
	if tp == nil {
		t.Fatal("convertTools() OfTool is nil")
	}
	if tp.Name != "get_pod_logs" {
		t.Errorf("Name = %q, want %q", tp.Name, "get_pod_logs")
	}
	descVal := tp.Description.Or("")
	if descVal != "Retrieve logs from a pod" {
		t.Errorf("Description = %q, want %q", descVal, "Retrieve logs from a pod")
	}
}

func TestConvertTools_Empty(t *testing.T) {
	result, err := convertTools(nil)
	if err != nil {
		t.Fatalf("convertTools(nil) error = %v", err)
	}
	if result != nil {
		t.Errorf("convertTools(nil) = %v, want nil", result)
	}
}

// TestConvertResponse_TextOnly verifies that a plain text response is mapped correctly.
func TestConvertResponse_TextOnly(t *testing.T) {
	resp := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: "Root cause: OOM kill"},
		},
	}

	msg, err := convertResponse(resp)
	if err != nil {
		t.Fatalf("convertResponse() error = %v", err)
	}
	if msg.Type != agent.MessageTypeAssistant {
		t.Errorf("Type = %v, want MessageTypeAssistant", msg.Type)
	}
	if msg.Content != "Root cause: OOM kill" {
		t.Errorf("Content = %q, want %q", msg.Content, "Root cause: OOM kill")
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("ToolCalls len = %d, want 0", len(msg.ToolCalls))
	}
}

// TestConvertResponse_ToolUse verifies that tool_use blocks become ToolCalls.
func TestConvertResponse_ToolUse(t *testing.T) {
	inputJSON := json.RawMessage(`{"namespace":"default","podName":"my-pod"}`)

	resp := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    "toolu_01",
				Name:  "get_pod_logs",
				Input: inputJSON,
			},
		},
	}

	msg, err := convertResponse(resp)
	if err != nil {
		t.Fatalf("convertResponse() error = %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}

	tc := msg.ToolCalls[0]
	if tc.ID != "toolu_01" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "toolu_01")
	}
	if tc.Function.Name != "get_pod_logs" {
		t.Errorf("ToolCall.Name = %q, want %q", tc.Function.Name, "get_pod_logs")
	}

	// Arguments should be valid JSON.
	var args map[string]string
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Errorf("ToolCall.Arguments is not valid JSON: %v", err)
	}
	if args["podName"] != "my-pod" {
		t.Errorf("args[podName] = %q, want %q", args["podName"], "my-pod")
	}
}

// TestConvertResponse_MixedContent verifies that text and tool_use can coexist.
func TestConvertResponse_MixedContent(t *testing.T) {
	inputJSON := json.RawMessage(`{}`)
	resp := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: "Let me check the logs."},
			{Type: "tool_use", ID: "toolu_02", Name: "get_pod_events", Input: inputJSON},
		},
	}

	msg, err := convertResponse(resp)
	if err != nil {
		t.Fatalf("convertResponse() error = %v", err)
	}
	if msg.Content != "Let me check the logs." {
		t.Errorf("Content = %q, want %q", msg.Content, "Let me check the logs.")
	}
	if len(msg.ToolCalls) != 1 {
		t.Errorf("ToolCalls len = %d, want 1", len(msg.ToolCalls))
	}
}

// --- helpers ---

// fakeToolForAnthropicTest implements agent.Tool for use in tests only.
type fakeToolForAnthropicTest struct {
	name        string
	description string
	schema      string
}

func (f *fakeToolForAnthropicTest) Name() string        { return f.name }
func (f *fakeToolForAnthropicTest) Description() string { return f.description }
func (f *fakeToolForAnthropicTest) Schema() string      { return f.schema }
func (f *fakeToolForAnthropicTest) Execute(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (f *fakeToolForAnthropicTest) SafetyLevel() agent.SafetyLevel { return agent.SafetyLevelReadOnly }
