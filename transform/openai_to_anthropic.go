package transform

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OpenAIToAnthropicResponse converts an OpenAI response to Anthropic format
func OpenAIToAnthropicResponse(openaiBody []byte, model string) ([]byte, error) {
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(openaiBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	// Build Anthropic response
	anthropicResp := AnthropicResponse{
		ID:    openaiResp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: model,
		Usage: AnthropicUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
	}

	// Convert first choice content to Anthropic format
	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]

		// Convert content and tool_calls to content blocks
		contentBlocks := openAIContentToTextBlocks(choice.Message.Content)
		if contentBlocks == nil {
			contentBlocks = []AnthropicContentBlock{}
		}

		// Convert tool_calls to tool_use blocks
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				toolCallMap, ok := toolCall.(map[string]interface{})
				if !ok {
					continue
				}

				// Extract function call details
				if functionData, ok := toolCallMap["function"].(map[string]interface{}); ok {
					toolUseBlock := AnthropicContentBlock{
						Type: "tool_use",
						ID:   getToolCallID(toolCallMap),
						Name: getString(functionData, "name"),
					}

					// Parse arguments JSON string to map
					if argsStr, ok := functionData["arguments"].(string); ok {
						var argsMap map[string]interface{}
						if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
							toolUseBlock.Input = argsMap
						}
					}

					contentBlocks = append(contentBlocks, toolUseBlock)
				}
			}
		}

		anthropicResp.Content = contentBlocks

		// Map finish reason
		switch choice.FinishReason {
		case "stop":
			anthropicResp.StopReason = "end_turn"
		case "length":
			anthropicResp.StopReason = "max_tokens"
		case "content_filter":
			anthropicResp.StopReason = "stop_sequence"
		case "tool_calls":
			anthropicResp.StopReason = "tool_use"
		default:
			anthropicResp.StopReason = "end_turn"
		}
	}

	// Serialize to JSON
	anthropicBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic response: %w", err)
	}

	return anthropicBody, nil
}

// OpenAIStreamToAnthropicStream converts an OpenAI stream chunk to Anthropic SSE format
func OpenAIStreamToAnthropicStream(openaiChunk []byte, model string) ([]string, error) {
	var chunk OpenAIStreamChunk
	if err := json.Unmarshal(openaiChunk, &chunk); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI stream chunk: %w", err)
	}

	var events []string

	if len(chunk.Choices) == 0 {
		return events, nil
	}

	choice := chunk.Choices[0]

	// Handle first chunk with role
	if choice.Delta.Role != "" {
		// Send message_start event
		messageStart := AnthropicStreamEvent{
			Type: "message_start",
			Message: &AnthropicResponse{
				ID:    chunk.ID,
				Type:  "message",
				Role:  "assistant",
				Model: model,
				Usage: AnthropicUsage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
		}
		data, _ := json.Marshal(messageStart)
		events = append(events, string(data))

		// Send content_block_start
		contentBlockStart := AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: &AnthropicContentBlock{
				Type: "text",
				Text: "",
			},
		}
		data, _ = json.Marshal(contentBlockStart)
		events = append(events, string(data))
	}

	// Handle content delta
	textDelta := joinTextSegments(ExtractOpenAIText(choice.Delta.Content))
	if textDelta != "" {
		contentDelta := AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &AnthropicDelta{
				Type: "text_delta",
				Text: textDelta,
			},
		}
		data, _ := json.Marshal(contentDelta)
		events = append(events, string(data))
	}

	// Handle finish reason
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		// Send content_block_stop
		contentBlockStop := AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: 0,
		}
		data, _ := json.Marshal(contentBlockStop)
		events = append(events, string(data))

		// Map finish reason
		stopReason := "end_turn"
		switch *choice.FinishReason {
		case "stop":
			stopReason = "end_turn"
		case "length":
			stopReason = "max_tokens"
		case "content_filter":
			stopReason = "stop_sequence"
		}

		// Send message_delta with stop reason
		messageDelta := AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: stopReason,
			},
			Usage: &AnthropicUsage{
				OutputTokens: 0, // OpenAI doesn't provide token counts in stream
			},
		}
		data, _ = json.Marshal(messageDelta)
		events = append(events, string(data))

		// Send message_stop
		messageStop := AnthropicStreamEvent{
			Type: "message_stop",
		}
		data, _ = json.Marshal(messageStop)
		events = append(events, string(data))
	}

	return events, nil
}

// GenerateAnthropicMessageID generates an Anthropic-style message ID
func GenerateAnthropicMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// Helper functions for tool conversion
func getToolCallID(toolCallMap map[string]interface{}) string {
	if id, ok := toolCallMap["id"].(string); ok {
		return id
	}
	return fmt.Sprintf("toolu_%d", time.Now().UnixNano())
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// ExtractOpenAIText normalizes OpenAI content (string or array blocks) into plain text segments.
func ExtractOpenAIText(content interface{}) []string {
	var texts []string

	switch v := content.(type) {
	case string:
		if v != "" {
			texts = append(texts, v)
		}
	case []interface{}:
		for _, part := range v {
			if text := extractTextFromContentPart(part); text != "" {
				texts = append(texts, text)
			}
		}
	case map[string]interface{}:
		if text := extractTextFromMap(v); text != "" {
			texts = append(texts, text)
		}
	default:
		if content == nil {
			return nil
		}

		if data, err := json.Marshal(content); err == nil {
			var arr []map[string]interface{}
			if err := json.Unmarshal(data, &arr); err == nil {
				for _, part := range arr {
					if text := extractTextFromMap(part); text != "" {
						texts = append(texts, text)
					}
				}
				if len(texts) > 0 {
					return texts
				}
			}

			var block map[string]interface{}
			if err := json.Unmarshal(data, &block); err == nil {
				if text := extractTextFromMap(block); text != "" {
					texts = append(texts, text)
				}
			}
		}
	}

	if len(texts) == 0 {
		return nil
	}
	return texts
}

func extractTextFromContentPart(part interface{}) string {
	block, ok := part.(map[string]interface{})
	if !ok {
		return ""
	}
	return extractTextFromMap(block)
}

func extractTextFromMap(block map[string]interface{}) string {
	if block == nil {
		return ""
	}

	if text, ok := block["text"].(string); ok && text != "" {
		return text
	}

	if content, ok := block["content"].(string); ok && content != "" {
		return content
	}

	if inner, ok := block["input"].(map[string]interface{}); ok {
		if text, ok := inner["text"].(string); ok && text != "" {
			return text
		}
	}

	return ""
}

func openAIContentToTextBlocks(content interface{}) []AnthropicContentBlock {
	textSegments := ExtractOpenAIText(content)
	if len(textSegments) == 0 {
		return nil
	}

	blocks := make([]AnthropicContentBlock, 0, len(textSegments))
	for _, text := range textSegments {
		blocks = append(blocks, AnthropicContentBlock{
			Type: "text",
			Text: text,
		})
	}
	return blocks
}

func joinTextSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(segment)
	}
	return builder.String()
}

// OpenAIToAnthropicRequest converts an OpenAI request to Anthropic format
func OpenAIToAnthropicRequest(openaiBody []byte) ([]byte, error) {
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	// Convert messages - separate system messages from regular messages
	var systemMessage string
	anthropicMessages := make([]AnthropicMessage, 0, len(openaiReq.Messages))

	for _, msg := range openaiReq.Messages {
		if msg.Role == "system" || msg.Role == "developer" {
			// Anthropic uses a separate system field
			contentStr, _ := msg.Content.(string)
			if systemMessage != "" {
				systemMessage += "\n\n" + contentStr
			} else {
				systemMessage = contentStr
			}
		} else {
			// Convert regular messages (keep as interface{} since Anthropic can handle it)
			anthropicMessages = append(anthropicMessages, AnthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Build Anthropic request
	anthropicReq := AnthropicRequest{
		Model:       openaiReq.Model,
		Messages:    anthropicMessages,
		MaxTokens:   openaiReq.MaxTokens,
		Temperature: openaiReq.Temperature,
		TopP:        openaiReq.TopP,
		Stream:      openaiReq.Stream,
		System:      systemMessage,
	}

	// Handle max_completion_tokens (OpenAI) vs max_tokens (Anthropic)
	if openaiReq.MaxCompletionTokens > 0 && anthropicReq.MaxTokens == 0 {
		anthropicReq.MaxTokens = openaiReq.MaxCompletionTokens
	}

	// Convert stop sequences
	if openaiReq.Stop != nil {
		switch stop := openaiReq.Stop.(type) {
		case string:
			anthropicReq.StopSeq = []string{stop}
		case []interface{}:
			stopSeq := make([]string, 0, len(stop))
			for _, s := range stop {
				if str, ok := s.(string); ok {
					stopSeq = append(stopSeq, str)
				}
			}
			anthropicReq.StopSeq = stopSeq
		}
	}

	// Serialize to JSON
	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	return anthropicBody, nil
}
