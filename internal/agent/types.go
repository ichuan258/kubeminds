package agent

import (
	"context"
	"kubeminds/api/v1alpha1"
)

// Agent defines the interface for the AI agent
type Agent interface {
	// Run executes the agent loop for a given goal
	Run(ctx context.Context, goal string) (*Result, error)
	// Restore restores the agent's memory from a list of findings
	Restore(findings []v1alpha1.Finding)
}

// Result contains the outcome of the agent's execution
type Result struct {
	RootCause  string
	Suggestion string
}

// Memory defines the interface for storing conversation history
type Memory interface {
	// AddUserMessage adds a user message to the history
	AddUserMessage(content string)
	// AddAssistantMessage adds an assistant message to the history
	AddAssistantMessage(content string)
	// AddToolOutput adds a tool execution result to the history
	AddToolOutput(toolCallID string, content string)
	// AddAssistantToolCall adds an assistant message that requests a tool call
	AddAssistantToolCall(toolCalls []ToolCall)
	// GetHistory returns the full conversation history
	GetHistory() []Message
}

// MessageType defines the type of the message
type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
	MessageTypeTool      MessageType = "tool"
	MessageTypeSystem    MessageType = "system"
)

// Message represents a single message in the conversation history
type Message struct {
	Type       MessageType
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// ToolCall represents a request to execute a tool
type ToolCall struct {
	ID       string
	Function FunctionCall
}

// FunctionCall represents the function to be called
type FunctionCall struct {
	Name      string
	Arguments string
}

// Tool defines the interface for tools that the agent can use
type Tool interface {
	// Name returns the name of the tool
	Name() string
	// Description returns a description of what the tool does
	Description() string
	// Execute runs the tool with the given arguments
	Execute(ctx context.Context, args string) (string, error)
	// Schema returns the JSON schema for the tool's arguments
	Schema() string
}

// LLMProvider defines the interface for the Large Language Model provider
type LLMProvider interface {
	// Chat sends a chat request to the LLM and returns the response
	Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error)
}
