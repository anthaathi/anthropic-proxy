package transform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AnthropicToOpenAIRequest converts an Anthropic request to OpenAI format
func AnthropicToOpenAIRequest(anthropicBody []byte) ([]byte, error) {
	var anthropicReq AnthropicRequest
	if err := json.Unmarshal(anthropicBody, &anthropicReq); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}

	// Convert messages
	openaiMessages := make([]OpenAIMessage, 0, len(anthropicReq.Messages)+1)

	// Add system message if present
	if anthropicReq.System != nil {
		systemText := extractSystemText(anthropicReq.System)
		if systemText != "" {
			openaiMessages = append(openaiMessages, OpenAIMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// Convert regular messages
	for _, msg := range anthropicReq.Messages {
		// Handle assistant messages with tool_use
		if msg.Role == "assistant" && hasToolUse(msg.Content) {
			// Extract text content
			textContent := extractTextContent(msg.Content)

			// Extract tool_calls
			toolCalls := convertToolUseToOpenAI(msg.Content)

			openaiMsg := OpenAIMessage{
				Role:      "assistant",
				ToolCalls: toolCalls,
			}

			// Only set content if there's text (OpenAI requires content to be null or omitted if only tool_calls)
			if textContent != "" {
				openaiMsg.Content = textContent
			}

			openaiMessages = append(openaiMessages, openaiMsg)
			continue
		}

		// Handle user messages with tool_result
		if msg.Role == "user" && hasToolResult(msg.Content) {
			// Extract and convert tool_result blocks to separate tool messages
			var blocks []interface{}
			switch v := msg.Content.(type) {
			case []interface{}:
				blocks = v
			default:
				// Try to marshal and unmarshal
				data, _ := json.Marshal(msg.Content)
				json.Unmarshal(data, &blocks)
			}

			var textContent string
			for _, block := range blocks {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}

				blockType, _ := blockMap["type"].(string)

				if blockType == "tool_result" {
					// Create a tool message for this tool_result
					toolUseID, _ := blockMap["tool_use_id"].(string)
					content := blockMap["content"]

					// Convert content to string
					var contentStr string
					switch v := content.(type) {
					case string:
						contentStr = v
					default:
						contentBytes, _ := json.Marshal(content)
						contentStr = string(contentBytes)
					}

					toolMsg := OpenAIMessage{
						Role:       "tool",
						ToolCallID: toolUseID,
						Content:    contentStr,
					}
					openaiMessages = append(openaiMessages, toolMsg)
				} else if blockType == "text" {
					// Collect text content
					if text, ok := blockMap["text"].(string); ok {
						if textContent != "" {
							textContent += "\n"
						}
						textContent += text
					}
				}
			}

			// If there was text content, add it as a user message
			if textContent != "" {
				openaiMessages = append(openaiMessages, OpenAIMessage{
					Role:    "user",
					Content: textContent,
				})
			}
			continue
		}

		// Handle regular messages (text only)
		openaiMsg := OpenAIMessage{
			Role: msg.Role,
		}

		// Convert content from Anthropic format to OpenAI format
		content := extractTextContent(msg.Content)
		openaiMsg.Content = content

		openaiMessages = append(openaiMessages, openaiMsg)
	}

	// Build OpenAI request
	openaiReq := OpenAIRequest{
		Model:       anthropicReq.Model,
		Messages:    openaiMessages,
		MaxTokens:   anthropicReq.MaxTokens,
		Temperature: anthropicReq.Temperature,
		TopP:        anthropicReq.TopP,
		Stream:      anthropicReq.Stream,
	}

	// Convert stop sequences
	if len(anthropicReq.StopSeq) > 0 {
		if len(anthropicReq.StopSeq) == 1 {
			openaiReq.Stop = anthropicReq.StopSeq[0]
		} else {
			openaiReq.Stop = anthropicReq.StopSeq
		}
	}

	// Convert tools from Anthropic format to OpenAI format
	if len(anthropicReq.Tools) > 0 {
		openaiReq.Tools = convertToolsToOpenAI(anthropicReq.Tools)
	}

	// Convert tool_choice if present
	if anthropicReq.ToolChoice != nil {
		openaiReq.ToolChoice = convertToolChoiceToOpenAI(anthropicReq.ToolChoice)
	}

	// Serialize to JSON
	openaiBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	return openaiBody, nil
}

// extractSystemText extracts text from Anthropic system field
// System can be a string or an array of content blocks
func extractSystemText(system interface{}) string {
	if system == nil {
		return ""
	}

	switch v := system.(type) {
	case string:
		// Already a string
		return v
	case []interface{}:
		// Array of content blocks - extract text from all text blocks
		var texts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "text" {
					if text, ok := blockMap["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		// Try to marshal and unmarshal as content blocks
		data, err := json.Marshal(system)
		if err != nil {
			return ""
		}

		var blocks []AnthropicContentBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return ""
		}

		var texts []string
		for _, block := range blocks {
			if block.Type == "text" {
				texts = append(texts, block.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
}

// convertToolsToOpenAI converts Anthropic tools to OpenAI format
// Anthropic: {"name": "...", "description": "...", "input_schema": {...}}
// OpenAI: {"type": "function", "function": {"name": "...", "description": "...", "parameters": {...}}}
func convertToolsToOpenAI(anthropicTools []interface{}) []interface{} {
	openaiTools := make([]interface{}, 0, len(anthropicTools))

	for _, tool := range anthropicTools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		// Convert Anthropic tool to OpenAI format
		openaiTool := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        toolMap["name"],
				"description": toolMap["description"],
				"parameters":  toolMap["input_schema"], // Anthropic uses "input_schema", OpenAI uses "parameters"
			},
		}

		openaiTools = append(openaiTools, openaiTool)
	}

	return openaiTools
}

// convertToolChoiceToOpenAI converts Anthropic tool_choice to OpenAI format
func convertToolChoiceToOpenAI(anthropicToolChoice interface{}) interface{} {
	// Anthropic tool_choice can be:
	// - {"type": "auto"} -> "auto"
	// - {"type": "any"} -> "required"
	// - {"type": "tool", "name": "..."} -> {"type": "function", "function": {"name": "..."}}

	toolChoiceMap, ok := anthropicToolChoice.(map[string]interface{})
	if !ok {
		return anthropicToolChoice
	}

	choiceType, ok := toolChoiceMap["type"].(string)
	if !ok {
		return anthropicToolChoice
	}

	switch choiceType {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		// Convert specific tool choice
		if name, ok := toolChoiceMap["name"].(string); ok {
			return map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": name,
				},
			}
		}
	}

	return anthropicToolChoice
}

// extractTextContent extracts text from Anthropic content format
// Anthropic content can be a string or an array of content blocks
func extractTextContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		// Already a string
		return v
	case []interface{}:
		// Array of content blocks - extract text from all text blocks
		var texts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "text" {
					if text, ok := blockMap["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		// Try to marshal and unmarshal as content blocks
		data, err := json.Marshal(content)
		if err != nil {
			return ""
		}

		var blocks []AnthropicContentBlock
		if err := json.Unmarshal(data, &blocks); err != nil {
			return ""
		}

		var texts []string
		for _, block := range blocks {
			if block.Type == "text" {
				texts = append(texts, block.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
}

// convertToolUseToOpenAI converts Anthropic tool_use blocks to OpenAI tool_calls format
func convertToolUseToOpenAI(content interface{}) []interface{} {
	var toolCalls []interface{}

	// Parse content blocks
	var blocks []interface{}
	switch v := content.(type) {
	case []interface{}:
		blocks = v
	default:
		// Try to marshal and unmarshal
		data, err := json.Marshal(content)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(data, &blocks); err != nil {
			return nil
		}
	}

	// Convert tool_use blocks to tool_calls
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_use" {
			// Extract tool_use fields
			id, _ := blockMap["id"].(string)
			name, _ := blockMap["name"].(string)
			input := blockMap["input"]

			// Convert input to JSON string (OpenAI expects arguments as JSON string)
			var argsStr string
			if input != nil {
				argsBytes, err := json.Marshal(input)
				if err == nil {
					argsStr = string(argsBytes)
				}
			}

			// Create OpenAI tool_call format
			toolCall := map[string]interface{}{
				"id":   id,
				"type": "function",
				"function": map[string]interface{}{
					"name":      name,
					"arguments": argsStr,
				},
			}

			toolCalls = append(toolCalls, toolCall)
		}
	}

	return toolCalls
}

// hasToolUse checks if content contains tool_use blocks
func hasToolUse(content interface{}) bool {
	var blocks []interface{}
	switch v := content.(type) {
	case []interface{}:
		blocks = v
	default:
		return false
	}

	for _, block := range blocks {
		if blockMap, ok := block.(map[string]interface{}); ok {
			if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_use" {
				return true
			}
		}
	}

	return false
}

// hasToolResult checks if content contains tool_result blocks
func hasToolResult(content interface{}) bool {
	var blocks []interface{}
	switch v := content.(type) {
	case []interface{}:
		blocks = v
	default:
		return false
	}

	for _, block := range blocks {
		if blockMap, ok := block.(map[string]interface{}); ok {
			if blockType, ok := blockMap["type"].(string); ok && blockType == "tool_result" {
				return true
			}
		}
	}

	return false
}
