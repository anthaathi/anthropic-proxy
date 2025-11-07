package transform

// Provider types
const (
	ProviderTypeAnthropic = "anthropic"
	ProviderTypeOpenAI    = "openai"
)

// Anthropic Request/Response Types
type AnthropicRequest struct {
	Model       string                   `json:"model"`
	Messages    []AnthropicMessage       `json:"messages"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	TopK        *int                     `json:"top_k,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
	StopSeq     []string                 `json:"stop_sequences,omitempty"`
	System      interface{}              `json:"system,omitempty"`
	Tools       []interface{}            `json:"tools,omitempty"`
	ToolChoice  interface{}              `json:"tool_choice,omitempty"`
	Thinking    *map[string]interface{}  `json:"thinking,omitempty"`
	Metadata    *map[string]interface{}  `json:"metadata,omitempty"`
}

type AnthropicMessage struct {
	Role    string                   `json:"role"`
	Content interface{}              `json:"content"` // Can be string or []ContentBlock
}

type AnthropicContentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`    // For tool_use blocks
	Name  string                 `json:"name,omitempty"`  // For tool_use blocks
	Input map[string]interface{} `json:"input,omitempty"` // For tool_use blocks
}

type AnthropicResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Content      []AnthropicContentBlock  `json:"content"`
	Model        string                   `json:"model"`
	StopReason   string                   `json:"stop_reason,omitempty"`
	StopSequence string                   `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage           `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// OpenAI Request/Response Types
type OpenAIRequest struct {
	Model            string                   `json:"model"`
	Messages         []OpenAIMessage          `json:"messages"`
	MaxTokens        int                      `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                   `json:"max_completion_tokens,omitempty"`
	Temperature      *float64                 `json:"temperature,omitempty"`
	TopP             *float64                 `json:"top_p,omitempty"`
	N                int                      `json:"n,omitempty"`
	Stream           bool                     `json:"stream,omitempty"`
	Stop             interface{}              `json:"stop,omitempty"` // Can be string or []string
	Tools            []interface{}            `json:"tools,omitempty"`
	ToolChoice       interface{}              `json:"tool_choice,omitempty"`
	PresencePenalty  *float64                 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64                 `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]float64       `json:"logit_bias,omitempty"`
	User             string                   `json:"user,omitempty"`
}

type OpenAIMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content,omitempty"` // Can be string, null, or array
	Name       string        `json:"name,omitempty"`
	ToolCalls  []interface{} `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"` // For tool role messages
}

type OpenAIResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []OpenAIChoice      `json:"choices"`
	Usage   OpenAIUsage         `json:"usage"`
	SystemFingerprint string    `json:"system_fingerprint,omitempty"`
}

type OpenAIChoice struct {
	Index        int             `json:"index"`
	Message      OpenAIMessage   `json:"message"`
	FinishReason string          `json:"finish_reason"`
	Logprobs     interface{}     `json:"logprobs,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI Streaming Types
type OpenAIStreamChunk struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAIStreamChoice     `json:"choices"`
}

type OpenAIStreamChoice struct {
	Index        int                  `json:"index"`
	Delta        OpenAIStreamDelta    `json:"delta"`
	FinishReason *string              `json:"finish_reason"`
}

type OpenAIStreamDelta struct {
	Role    string      `json:"role,omitempty"`
	Content interface{} `json:"content,omitempty"`
}

// Anthropic Streaming Types
type AnthropicStreamEvent struct {
	Type          string                      `json:"type"`
	Message       *AnthropicResponse          `json:"message,omitempty"`
	Index         int                         `json:"index,omitempty"`
	ContentBlock  *AnthropicContentBlock      `json:"content_block,omitempty"`
	Delta         *AnthropicDelta             `json:"delta,omitempty"`
	Usage         *AnthropicUsage             `json:"usage,omitempty"`
}

type AnthropicDelta struct {
	Type         string `json:"type,omitempty"`
	Text         string `json:"text,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}
