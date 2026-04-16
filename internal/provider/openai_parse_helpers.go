package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"bytemind/internal/llm"
)

func parseOpenAIUsage(raw json.RawMessage) *llm.Usage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return nil
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &llm.Usage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens}
}

type streamDelta struct {
	Role      llm.Role
	Content   string
	Reasoning string
	ToolCalls []streamToolCallDelta
}

type streamToolCallDelta struct {
	Index             int
	ID                string
	Type              string
	FunctionName      string
	FunctionArguments string
}

func parseOpenAIDelta(raw json.RawMessage) (streamDelta, error) {
	delta := streamDelta{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return delta, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return streamDelta{}, err
	}
	if roleRaw, ok := obj["role"]; ok {
		var role string
		_ = json.Unmarshal(roleRaw, &role)
		delta.Role = llm.Role(strings.TrimSpace(role))
	}
	if contentRaw, ok := obj["content"]; ok {
		_ = json.Unmarshal(contentRaw, &delta.Content)
	}
	if reasoningRaw, ok := obj["reasoning_content"]; ok {
		_ = json.Unmarshal(reasoningRaw, &delta.Reasoning)
	}
	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		delta.ToolCalls = append(delta.ToolCalls, parseStreamToolCalls(toolCallsRaw)...)
	}
	if functionCallRaw, ok := obj["function_call"]; ok {
		legacy := parseLegacyFunctionCall(functionCallRaw)
		if legacy.FunctionName != "" || legacy.FunctionArguments != "" {
			legacy.Index = legacyToolCallIndex
			delta.ToolCalls = append(delta.ToolCalls, legacy)
		}
	}
	return delta, nil
}

func parseOpenAIMessage(raw json.RawMessage) llm.Message {
	msg := llm.Message{Role: llm.RoleAssistant}
	if len(bytes.TrimSpace(raw)) == 0 {
		return msg
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return msg
	}
	if roleRaw, ok := obj["role"]; ok {
		var role string
		_ = json.Unmarshal(roleRaw, &role)
		if strings.TrimSpace(role) != "" {
			msg.Role = llm.Role(role)
		}
	}
	if contentRaw, ok := obj["content"]; ok {
		_ = json.Unmarshal(contentRaw, &msg.Content)
	}
	if msg.Content == "" {
		if reasoningRaw, ok := obj["reasoning_content"]; ok {
			var ignored string
			_ = json.Unmarshal(reasoningRaw, &ignored)
		}
	}
	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		msg.ToolCalls = parseToolCalls(toolCallsRaw)
	}
	if len(msg.ToolCalls) == 0 {
		if functionCallRaw, ok := obj["function_call"]; ok {
			legacy := parseLegacyFunctionCall(functionCallRaw)
			if legacy.FunctionName != "" {
				msg.ToolCalls = []llm.ToolCall{{ID: "call-legacy", Type: "function", Function: llm.ToolFunctionCall{Name: legacy.FunctionName, Arguments: legacy.FunctionArguments}}}
			}
		}
	}
	msg.Normalize()
	return msg
}

func parseToolCalls(raw json.RawMessage) []llm.ToolCall {
	var calls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for i, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("call-%d", i)
		}
		typ := strings.TrimSpace(call.Type)
		if typ == "" {
			typ = "function"
		}
		out = append(out, llm.ToolCall{ID: id, Type: typ, Function: llm.ToolFunctionCall{Name: call.Function.Name, Arguments: argumentString(call.Function.Arguments)}})
	}
	return out
}

func parseStreamToolCalls(raw json.RawMessage) []streamToolCallDelta {
	var calls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]streamToolCallDelta, 0, len(calls))
	for _, call := range calls {
		out = append(out, streamToolCallDelta{Index: call.Index, ID: call.ID, Type: call.Type, FunctionName: call.Function.Name, FunctionArguments: argumentString(call.Function.Arguments)})
	}
	return out
}

func parseLegacyFunctionCall(raw json.RawMessage) streamToolCallDelta {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return streamToolCallDelta{}
	}
	return streamToolCallDelta{Type: "function", FunctionName: call.Name, FunctionArguments: argumentString(call.Arguments)}
}

func argumentString(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err == nil {
			return compact.String()
		}
	}
	return trimmed
}
