package llm

// AnthropicProvider implements agent.LLMProvider for Anthropic Claude models.
//
// Anthropic's Messages API uses a different message format than OpenAI:
//   - The system prompt is a top-level field, not a message role.
//   - Tool results must be wrapped in a user-role message as "tool_result" content blocks.
//   - Tool calls from the assistant are "tool_use" content blocks (not a separate field).
//
// This provider converts between our internal OpenAI-style message format and
// Anthropic's format transparently. The rest of the agent loop is unaware of the
// underlying provider.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"kubeminds/internal/agent"
)

// defaultMaxTokens is the default max_tokens sent to Anthropic.
// Anthropic requires this field; 4096 is a safe default for diagnostic tasks.
const defaultMaxTokens int64 = 4096

// AnthropicProvider implements agent.LLMProvider using the Anthropic SDK.
type AnthropicProvider struct {
	client *anthropic.Client
	model  string
}

// NewAnthropicProvider creates a new AnthropicProvider.
//
// apiKey is your Anthropic API key (https://console.anthropic.com/).
// model is the Claude model identifier (e.g. "claude-sonnet-4-6", "claude-opus-4-6").
// baseURL overrides the default Anthropic API endpoint; leave empty to use the default.
func NewAnthropicProvider(apiKey, model, baseURL string) *AnthropicProvider {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	c := anthropic.NewClient(opts...)
	return &AnthropicProvider{
		client: &c,
		model:  model,
	}
}

// Chat sends messages to Anthropic Claude and returns the response.
// It converts from our internal OpenAI-style format to Anthropic's format,
// makes the API call with exponential-backoff retry, and converts the response back.
func (p *AnthropicProvider) Chat(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.Message, error) {
	// --- Convert tools ---
	anthropicTools, err := convertTools(tools)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to convert tools: %w", err)
	}

	// --- Split system prompt from the rest of the messages ---
	// Anthropic accepts the system prompt as a top-level []TextBlockParam, not as a message.
	var systemBlocks []anthropic.TextBlockParam
	var chatMessages []anthropic.MessageParam

	for _, msg := range messages {
		switch msg.Type {
		case agent.MessageTypeSystem:
			// Collect all system messages into the top-level system field.
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: msg.Content})

		case agent.MessageTypeUser:
			chatMessages = append(chatMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))

		case agent.MessageTypeAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant turn: one or more tool_use content blocks.
				blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls)+1)

				// Include the text content if present alongside tool calls.
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					// The tool call arguments are a JSON string in our internal format;
					// unmarshal them so Anthropic receives a proper JSON object as input.
					var inputObj any
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &inputObj); err != nil {
						// If arguments are not valid JSON, pass the raw string as-is.
						inputObj = tc.Function.Arguments
					}
					blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, inputObj, tc.Function.Name))
				}
				chatMessages = append(chatMessages, anthropic.NewAssistantMessage(blocks...))
			} else {
				// Plain text assistant turn.
				chatMessages = append(chatMessages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(msg.Content),
				))
			}

		case agent.MessageTypeTool:
			// Tool results in Anthropic must be user-role messages with tool_result blocks.
			chatMessages = append(chatMessages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
		}
	}

	// --- Build request params ---
	reqParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: defaultMaxTokens,
		Messages:  chatMessages,
		System:    systemBlocks,
	}
	if len(anthropicTools) > 0 {
		reqParams.Tools = anthropicTools
	}

	// --- Call API with exponential-backoff retry ---
	resp, err := p.callWithRetry(ctx, reqParams)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// --- Convert response back to our internal format ---
	return convertResponse(resp)
}

// callWithRetry calls the Anthropic Messages API with exponential backoff.
// Max 3 attempts; delays: 1s, 2s (capped at 10s). Only network/5xx/429 errors are retried.
func (p *AnthropicProvider) callWithRetry(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	const maxRetries = 3
	baseDelay := time.Second

	var resp *anthropic.Message
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = p.client.Messages.New(ctx, params)
		if err == nil {
			return resp, nil
		}

		if attempt < maxRetries-1 && isRetryableError(err) {
			delay := time.Duration(math.Min(
				float64(baseDelay.Milliseconds()*int64(math.Pow(2, float64(attempt)))),
				10000,
			)) * time.Millisecond

			select {
			case <-time.After(delay):
				// continue to next attempt
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
		} else {
			break
		}
	}

	return nil, err
}

// convertTools converts our internal agent.Tool slice to Anthropic's ToolParam slice.
func convertTools(tools []agent.Tool) ([]anthropic.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		// Parse the JSON Schema string from our tool interface.
		var schemaObj struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal([]byte(t.Schema()), &schemaObj); err != nil {
			return nil, fmt.Errorf("failed to parse schema for tool %q: %w", t.Name(), err)
		}

		toolParam := anthropic.ToolParam{
			Name:        t.Name(),
			Description: param.NewOpt(t.Description()),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: schemaObj.Properties,
				Required:   schemaObj.Required,
			},
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &toolParam})
	}
	return result, nil
}

// convertResponse converts an Anthropic Message response to our internal agent.Message.
// It extracts text content and any tool_use blocks into the appropriate fields.
func convertResponse(resp *anthropic.Message) (*agent.Message, error) {
	result := &agent.Message{
		Type: agent.MessageTypeAssistant,
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			// Accumulate text blocks (there's usually just one).
			if result.Content != "" {
				result.Content += "\n"
			}
			result.Content += block.Text

		case "tool_use":
			// Marshal the tool input (a JSON object) back to the string form our
			// internal ToolCall.Function.Arguments expects.
			argsBytes, err := json.Marshal(block.Input)
			if err != nil {
				return nil, fmt.Errorf("anthropic: failed to marshal tool_use input for %q: %w", block.Name, err)
			}
			result.ToolCalls = append(result.ToolCalls, agent.ToolCall{
				ID: block.ID,
				Function: agent.FunctionCall{
					Name:      block.Name,
					Arguments: string(argsBytes),
				},
			})
		}
		// Other block types (thinking, redacted_thinking, etc.) are ignored.
	}

	return result, nil
}
