package llm

// GeminiProvider wraps OpenAIProvider using Google's official OpenAI-compatible endpoint.
//
// Google Gemini exposes an OpenAI-compatible API at a well-known URL, so all
// message formatting, tool-call handling, and retry logic from OpenAIProvider are
// reused without modification. Only the base URL differs.
//
// Reference: https://ai.google.dev/gemini-api/docs/openai
const geminiCompatBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"

// NewGeminiProvider creates a Gemini LLM provider.
//
// apiKey is your Google AI Studio API key (https://aistudio.google.com/app/apikey).
// model is the Gemini model name (e.g. "gemini-2.0-flash", "gemini-1.5-pro").
// baseURL overrides the default Gemini OpenAI-compat endpoint; leave empty to use
// the default (geminiCompatBaseURL above).
func NewGeminiProvider(apiKey, model, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = geminiCompatBaseURL
	}
	return NewOpenAIProvider(apiKey, model, baseURL)
}
