package agent

import (
	"context"
	"fmt"
	"time"

	"kubeminds/api/v1alpha1"
)

// ErrWaitingForApproval is returned when a tool execution is blocked pending user approval
type ErrWaitingForApproval struct {
	ToolName string
}

func (e *ErrWaitingForApproval) Error() string {
	return fmt.Sprintf("tool %s requires approval", e.ToolName)
}

// ErrToolForbidden is returned when a tool execution is forbidden
type ErrToolForbidden struct {
	ToolName string
}

func (e *ErrToolForbidden) Error() string {
	return fmt.Sprintf("tool %s is forbidden", e.ToolName)
}

// Agent defines the interface for the AI agent
type Agent interface {
	// Run executes the agent loop for a given goal
	Run(ctx context.Context, goal string, approved bool) (*Result, error)
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

// SafetyLevel defines the risk level of a tool
type SafetyLevel string

const (
	SafetyLevelReadOnly  SafetyLevel = "ReadOnly"
	SafetyLevelLowRisk   SafetyLevel = "LowRisk"
	SafetyLevelHighRisk  SafetyLevel = "HighRisk"
	SafetyLevelForbidden SafetyLevel = "Forbidden"
)

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
	// SafetyLevel returns the risk level of the tool
	SafetyLevel() SafetyLevel
}

// ToolProvider defines the interface for providing tools
type ToolProvider interface {
	// ListTools returns a list of available tools
	ListTools(ctx context.Context) ([]Tool, error)
}

// LLMProvider defines the interface for the Large Language Model provider
type LLMProvider interface {
	// Chat sends a chat request to the LLM and returns the response
	Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error)
}

// AlertEvent represents a recent alert event stored in the L2 event stream.
type AlertEvent struct {
	AlertName string
	Namespace string
	Pod       string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// EventStore is the L2 interface for reading and writing recent alert events.
// Both methods must be nil-safe and tolerate unavailable backends gracefully.
type EventStore interface {
	// AppendAlertEvent writes a new alert event to the event stream.
	AppendAlertEvent(ctx context.Context, event AlertEvent) error
	// GetRecentEvents retrieves the most recent events for the given namespace.
	// If pod is non-empty, only events for that pod are returned.
	GetRecentEvents(ctx context.Context, namespace, pod string, limit int) ([]AlertEvent, error)
}

// KnowledgeFinding represents a completed diagnosis stored in the L3 knowledge base.
type KnowledgeFinding struct {
	ID         string
	AlertName  string
	Namespace  string
	RootCause  string
	Suggestion string
	CreatedAt  time.Time
}

// KnowledgeBase is the L3 interface for storing and retrieving historical diagnoses.
// Embeddings are owned by the caller to keep this interface storage-agnostic.
type KnowledgeBase interface {
	// InitSchema creates the required database schema if it does not already exist.
	InitSchema(ctx context.Context) error
	// SaveDiagnosis persists a completed diagnosis alongside its embedding vector.
	SaveDiagnosis(ctx context.Context, finding KnowledgeFinding, embedding []float32) error
	// SearchSimilar returns the top-k historically similar diagnoses ordered by
	// cosine similarity to queryEmbedding.
	SearchSimilar(ctx context.Context, queryEmbedding []float32, limit int) ([]KnowledgeFinding, error)
}

// EmbeddingProvider generates dense vector embeddings for text.
// The interface lives here so the controller can reference it without importing
// the llm package (which would create an import cycle: controller → llm → agent).
type EmbeddingProvider interface {
	// Embed returns a float32 embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
}
