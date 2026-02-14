package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"kubeminds/internal/agent"
)

// OpenAIProvider implements the LLMProvider interface for OpenAI
type OpenAIProvider struct {
	client *openai.Client
	model  string
}

// NewOpenAIProvider creates a new OpenAIProvider
func NewOpenAIProvider(apiKey string, model string, baseURL string) *OpenAIProvider {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	return &OpenAIProvider{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// Chat sends a chat request to the LLM and returns the response
func (p *OpenAIProvider) Chat(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.Message, error) {
	openaiMessages := make([]openai.ChatCompletionMessage, 0, len(messages))

	for _, msg := range messages {
		openaiMsg := openai.ChatCompletionMessage{
			Content: msg.Content,
		}

		switch msg.Type {
		case agent.MessageTypeUser:
			openaiMsg.Role = openai.ChatMessageRoleUser
		case agent.MessageTypeAssistant:
			openaiMsg.Role = openai.ChatMessageRoleAssistant
			if len(msg.ToolCalls) > 0 {
				openaiMsg.ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					openaiMsg.ToolCalls[i] = openai.ToolCall{
						ID:   tc.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
			}
		case agent.MessageTypeTool:
			openaiMsg.Role = openai.ChatMessageRoleTool
			openaiMsg.ToolCallID = msg.ToolCallID
		case agent.MessageTypeSystem:
			openaiMsg.Role = openai.ChatMessageRoleSystem
		}

		openaiMessages = append(openaiMessages, openaiMsg)
	}

	openaiTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		var params json.RawMessage
		if err := json.Unmarshal([]byte(tool.Schema()), &params); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool schema for %s: %w", tool.Name(), err)
		}

		openaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  params,
			},
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: openaiMessages,
		Tools:    openaiTools,
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from openai")
	}

	choice := resp.Choices[0]
	result := &agent.Message{
		Type:    agent.MessageTypeAssistant,
		Content: choice.Message.Content,
	}

	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]agent.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = agent.ToolCall{
				ID: tc.ID,
				Function: agent.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return result, nil
}
