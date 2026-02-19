package llm

import (
	"context"

	"kubeminds/internal/agent"
)

// MockProvider implements LLMProvider for testing without real API calls
type MockProvider struct {
	responses map[string]string
	callCount int
}

// NewMockProvider creates a new MockProvider with predefined responses
func NewMockProvider() *MockProvider {
	return &MockProvider{
		responses: map[string]string{
			"oom":       "Root Cause: Pod memory limit exceeded (OOMKilled)\nSuggestion: Increase memory limit in pod spec or optimize application memory usage",
			"imagepull": "Root Cause: Container image pull failed - registry unreachable or image does not exist\nSuggestion: Verify image URL, check registry credentials, or ensure registry is accessible",
			"crashloop": "Root Cause: Container crashes immediately after startup - application error or misconfiguration\nSuggestion: Check application logs for startup errors, verify environment variables and config maps",
			"notready":  "Root Cause: Node status NotReady due to kubelet or network issues\nSuggestion: Check node kubelet status, verify network connectivity, drain and restart node if necessary",
			"default":   "Root Cause: Pod diagnostic inconclusive - requires deeper investigation\nSuggestion: Collect more logs and events, check application health endpoints",
		},
		callCount: 0,
	}
}

// Chat returns a mock response based on the agent's goal
// In a real scenario, you would analyze the messages and tools to determine response
func (m *MockProvider) Chat(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.Message, error) {
	m.callCount++

	// Infer response type from message content (simple heuristic for testing)
	responseKey := "default"
	for _, msg := range messages {
		if contains(msg.Content, "OOM") || contains(msg.Content, "memory") {
			responseKey = "oom"
		} else if contains(msg.Content, "ImagePull") || contains(msg.Content, "image") {
			responseKey = "imagepull"
		} else if contains(msg.Content, "CrashLoop") || contains(msg.Content, "crash") {
			responseKey = "crashloop"
		} else if contains(msg.Content, "NotReady") || contains(msg.Content, "node") {
			responseKey = "notready"
		}
	}

	response := m.responses[responseKey]

	return &agent.Message{
		Type:    agent.MessageTypeAssistant,
		Content: response,
	}, nil
}

// SetResponse allows tests to customize responses
func (m *MockProvider) SetResponse(key string, response string) {
	m.responses[key] = response
}

// GetCallCount returns how many times Chat was called
func (m *MockProvider) GetCallCount() int {
	return m.callCount
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
