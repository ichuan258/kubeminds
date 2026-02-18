package agent

import (
	"context"
)

// MockLLMProvider is a mock implementation of LLMProvider for testing
type MockLLMProvider struct {
	// Responses is a map where key is the step number (0-indexed) and value is the message to return
	Responses map[int]*Message
	// ErrorTrigger is a map where key is the step number and value is the error to return
	ErrorTrigger map[int]error
	// CallCount tracks how many times Chat has been called
	CallCount int
}

func NewMockLLMProvider() *MockLLMProvider {
	return &MockLLMProvider{
		Responses:    make(map[int]*Message),
		ErrorTrigger: make(map[int]error),
	}
}

func (m *MockLLMProvider) Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	// Simple Chat Mock: return configured response for the current call count
	// We use CallCount to map to steps in the test.
	currentStep := m.CallCount
	m.CallCount++

	if err, ok := m.ErrorTrigger[currentStep]; ok {
		return nil, err
	}

	if msg, ok := m.Responses[currentStep]; ok {
		return msg, nil
	}

	// Fallback for unexpected calls to prevent panics or errors in simple tests
	return &Message{
		Type:    MessageTypeAssistant,
		Content: "I don't know what to do.",
	}, nil
}

// MockTool is a mock implementation of Tool for testing
type MockTool struct {
	NameVal        string
	DescVal        string
	SafetyLevelVal SafetyLevel
	ExecuteFunc    func(ctx context.Context, args string) (string, error)
	ExecutionCount int
}

func (m *MockTool) Name() string {
	return m.NameVal
}

func (m *MockTool) Description() string {
	return m.DescVal
}

func (m *MockTool) Execute(ctx context.Context, args string) (string, error) {
	m.ExecutionCount++
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, args)
	}
	return "mock output", nil
}

func (m *MockTool) Schema() string {
	return "{}"
}

func (m *MockTool) SafetyLevel() SafetyLevel {
	if m.SafetyLevelVal != "" {
		return m.SafetyLevelVal
	}
	return SafetyLevelReadOnly
}
