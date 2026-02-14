package agent

import "sync"

// L1Memory implements a simple in-memory storage for conversation history
type L1Memory struct {
	mu       sync.RWMutex
	messages []Message
}

// NewL1Memory creates a new instance of L1Memory
func NewL1Memory() *L1Memory {
	return &L1Memory{
		messages: make([]Message, 0),
	}
}

// AddUserMessage adds a user message to the history
func (m *L1Memory) AddUserMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
		Type:    MessageTypeUser,
		Content: content,
	})
}

// AddAssistantMessage adds an assistant message to the history
func (m *L1Memory) AddAssistantMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
		Type:    MessageTypeAssistant,
		Content: content,
	})
}

// AddToolOutput adds a tool execution result to the history
func (m *L1Memory) AddToolOutput(toolCallID string, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
		Type:       MessageTypeTool,
		Content:    content,
		ToolCallID: toolCallID,
	})
}

// AddAssistantToolCall adds an assistant message that requests a tool call
func (m *L1Memory) AddAssistantToolCall(toolCalls []ToolCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{
		Type:      MessageTypeAssistant,
		ToolCalls: toolCalls,
	})
}

// GetHistory returns the full conversation history
func (m *L1Memory) GetHistory() []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy to avoid race conditions if the caller modifies it
	history := make([]Message, len(m.messages))
	copy(history, m.messages)
	return history
}
