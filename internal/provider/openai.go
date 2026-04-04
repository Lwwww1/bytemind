package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"bytemind/internal/llm"
)

type Config struct {
	Type             string
	BaseURL          string
	APIKey           string
	Model            string
	AnthropicVersion string
}

type OpenAICompatible struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	return &OpenAICompatible{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *OpenAICompatible) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	payload, err := c.chatPayload(req, false)
	if err != nil {
		return llm.Message{}, err
	}
	respBody, err := c.postJSON(ctx, c.baseURL+"/chat/completions", payload)
	if err != nil {
		return llm.Message{}, err
	}

	var completion struct {
		Choices []struct {
			Message openAIWireMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Choices) == 0 {
		return llm.Message{}, fmt.Errorf("provider returned no choices")
	}
	return fromOpenAIWireMessage(completion.Choices[0].Message), nil
}

func (c *OpenAICompatible) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	payload, err := c.chatPayload(req, true)
	if err != nil {
		return llm.Message{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return llm.Message{}, llm.MapProviderError("openai", resp.StatusCode, string(respBody), nil)
	}

	assembled := llm.Message{Role: llm.RoleAssistant}
	toolCalls := map[int]*llm.ToolCall{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Role      string           `json:"role"`
					Content   string           `json:"content"`
					ToolCalls []openAIToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return llm.Message{}, err
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Role != "" {
				assembled.Role = llm.Role(choice.Delta.Role)
			}
			if choice.Delta.Content != "" {
				assembled.Content += choice.Delta.Content
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			for _, callDelta := range choice.Delta.ToolCalls {
				call, ok := toolCalls[callDelta.Index]
				if !ok {
					call = &llm.ToolCall{Type: "function"}
					toolCalls[callDelta.Index] = call
				}
				if callDelta.ID != "" {
					call.ID = callDelta.ID
				}
				if callDelta.Type != "" {
					call.Type = callDelta.Type
				}
				if callDelta.Function.Name != "" {
					call.Function.Name += callDelta.Function.Name
				}
				if callDelta.Function.Arguments != "" {
					call.Function.Arguments += callDelta.Function.Arguments
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return llm.Message{}, err
	}

	if len(toolCalls) > 0 {
		indexes := make([]int, 0, len(toolCalls))
		for index := range toolCalls {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		assembled.ToolCalls = make([]llm.ToolCall, 0, len(indexes))
		for _, index := range indexes {
			assembled.ToolCalls = append(assembled.ToolCalls, *toolCalls[index])
		}
	}
	assembled.Normalize()

	return assembled, nil
}

func (c *OpenAICompatible) chatPayload(req llm.ChatRequest, stream bool) (map[string]any, error) {
	messages, err := openAIMessages(req)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model":       choose(req.Model, c.model),
		"messages":    messages,
		"temperature": req.Temperature,
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		payload["tool_choice"] = "auto"
	}
	if stream {
		payload["stream"] = true
	}
	return payload, nil
}

func openAIMessages(req llm.ChatRequest) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(req.Messages))

	for _, message := range req.Messages {
		message.Normalize()
		content := make([]map[string]any, 0, len(message.Parts))
		toolCalls := make([]map[string]any, 0, len(message.Parts))
		toolResults := make([]map[string]any, 0, len(message.Parts))

		for _, part := range message.Parts {
			switch part.Type {
			case llm.PartText:
				content = append(content, map[string]any{"type": "text", "text": part.Text.Value})
			case llm.PartImageRef:
				asset, ok := req.Assets[part.Image.AssetID]
				if !ok {
					return nil, llm.WrapError("openai", llm.ErrorCodeAssetNotFound, fmt.Errorf("asset %q not found", part.Image.AssetID))
				}
				if len(asset.Data) == 0 {
					return nil, llm.WrapError("openai", llm.ErrorCodeAssetNotFound, fmt.Errorf("asset %q has empty payload", part.Image.AssetID))
				}
				mediaType := strings.TrimSpace(asset.MediaType)
				if mediaType == "" {
					mediaType = "image/png"
				}
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data),
					},
				})
			case llm.PartToolUse:
				toolCalls = append(toolCalls, map[string]any{
					"id":   part.ToolUse.ID,
					"type": "function",
					"function": map[string]any{
						"name":      part.ToolUse.Name,
						"arguments": part.ToolUse.Arguments,
					},
				})
			case llm.PartToolResult:
				toolResults = append(toolResults, map[string]any{
					"role":         "tool",
					"tool_call_id": part.ToolResult.ToolUseID,
					"content":      part.ToolResult.Content,
				})
			}
		}

		if message.Role == "tool" {
			toolID := message.ToolCallID
			if toolID == "" && len(message.Parts) > 0 {
				for _, part := range message.Parts {
					if part.ToolResult != nil {
						toolID = part.ToolResult.ToolUseID
						break
					}
				}
			}
			result = append(result, map[string]any{
				"role":         "tool",
				"tool_call_id": toolID,
				"content":      message.Text(),
			})
			continue
		}

		if len(content) > 0 || len(toolCalls) > 0 {
			wire := map[string]any{"role": string(message.Role)}
			if len(content) > 0 {
				wire["content"] = content
			}
			if len(toolCalls) > 0 {
				wire["tool_calls"] = toolCalls
			}
			result = append(result, wire)
		}
		if len(toolResults) > 0 {
			result = append(result, toolResults...)
		}
	}

	return result, nil
}

type openAIWireMessage struct {
	Role       string           `json:"role"`
	Content    json.RawMessage  `json:"content"`
	ToolCallID string           `json:"tool_call_id"`
	ToolCalls  []openAIToolCall `json:"tool_calls"`
}

type openAIToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func fromOpenAIWireMessage(message openAIWireMessage) llm.Message {
	result := llm.Message{Role: llm.Role(message.Role)}

	text := decodeOpenAIText(message.Content)
	if text != "" {
		result.Content = text
	}
	for _, call := range message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, llm.ToolCall{
			ID:   call.ID,
			Type: choose(call.Type, "function"),
			Function: llm.ToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	if message.ToolCallID != "" {
		result.ToolCallID = message.ToolCallID
	}
	result.Normalize()
	return result
}

func decodeOpenAIText(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var builder strings.Builder
		for _, block := range blocks {
			if block.Type == "text" {
				builder.WriteString(block.Text)
			}
		}
		return builder.String()
	}

	return ""
}

func (c *OpenAICompatible) postJSON(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, llm.MapProviderError("openai", resp.StatusCode, string(respBody), nil)
	}
	return respBody, nil
}

func choose(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
