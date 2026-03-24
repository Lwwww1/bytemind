package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	model      string
	apiKey     string
}

func NewClient(baseURL, model, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 90 * time.Second},
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		model:      strings.TrimSpace(model),
		apiKey:     strings.TrimSpace(apiKey),
	}
}

func (c *Client) Configured() bool {
	return c.apiKey != ""
}

func (c *Client) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, error) {
	if !c.Configured() {
		return Message{}, fmt.Errorf("missing OPENAI_API_KEY")
	}

	reqBody := chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.1,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
		reqBody.ToolChoice = "auto"
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, fmt.Errorf("marshal chat request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("create chat request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Message{}, fmt.Errorf("send chat request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return Message{}, fmt.Errorf("read chat response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return Message{}, fmt.Errorf("chat request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Message{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Message{}, fmt.Errorf("chat response did not include choices")
	}

	return decoded.Choices[0].Message, nil
}

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}
