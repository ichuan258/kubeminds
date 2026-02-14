package agent

import (
	"context"
	"fmt"
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
	currentStep := m.CallCount
	m.CallCount++

	if err, ok := m.ErrorTrigger[currentStep]; ok {
		return nil, err
	}

	if msg, ok := m.Responses[currentStep]; ok {
		return msg, nil
	}

	return nil, fmt.Errorf("no mock response configured for step %d", currentStep)
}

// MockTool is a mock implementation of Tool for testing
type MockTool struct {
	NameVal        string
	DescVal        string
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
