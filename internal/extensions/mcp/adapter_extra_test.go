package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	extensionspkg "bytemind/internal/extensions"
	toolspkg "bytemind/internal/tools"
)

func TestWithClientOptionHandlesNilOptions(t *testing.T) {
	opt := WithClient(&stubClient{})
	opt(nil)
}

func TestFromMCPServerInvalidConfigAndDefaultClientPath(t *testing.T) {
	_, err := FromMCPServer(ServerConfig{
		Command: "echo",
	}, WithClient(&stubClient{}))
	if err == nil {
		t.Fatal("expected invalid config error when id is missing")
	}
	var extErr *extensionspkg.ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != extensionspkg.ErrCodeInvalidSource {
		t.Fatalf("expected invalid source code, got %q", extErr.Code)
	}

	ext, err := FromMCPServer(ServerConfig{
		ID:   "auto-client",
		Name: "Auto Client",
	})
	if err != nil {
		t.Fatalf("default client construction should not fail hard, got %v", err)
	}
	info := ext.Info()
	if info.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded status for missing command with default client, got %q", info.Status)
	}
}

func TestFromMCPServerHandlesNilOptionAndNilNowOverride(t *testing.T) {
	ext, err := FromMCPServer(ServerConfig{
		ID:   "github",
		Name: "GitHub",
	}, nil, WithClient(&stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "github",
			Name:    "GitHub",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{Name: "list_prs"},
			},
		},
	}), func(opts *adapterOptions) {
		opts.now = nil
	})
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	info := ext.Info()
	if info.ID != "mcp.github" {
		t.Fatalf("unexpected extension id: %q", info.ID)
	}
	if info.Health.CheckedAtUTC == "" {
		t.Fatal("expected health timestamp to be set")
	}
}

func TestAdapterNilReceiverBranches(t *testing.T) {
	var adapter *Adapter
	if info := adapter.Info(); !info.IsZero() {
		t.Fatalf("expected zero info for nil receiver, got %#v", info)
	}

	if _, err := adapter.ResolveTools(context.Background()); err == nil {
		t.Fatal("expected resolve error for nil receiver")
	}
	if _, err := adapter.Health(context.Background()); err == nil {
		t.Fatal("expected health error for nil receiver")
	}
	if err := adapter.refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error for nil receiver")
	}
}

func TestResolveToolsReturnsContextError(t *testing.T) {
	client := &stubClient{
		discoverErr: context.DeadlineExceeded,
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:   "timeout",
		Name: "Timeout",
	}, WithClient(client))
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	_, err = ext.ResolveTools(context.Background())
	if err == nil {
		t.Fatal("expected context error from ResolveTools")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestAdapterRefreshActiveMessageWithoutSkippedTools(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "server",
			Name:    "Server",
			Version: "2.0.0",
			Tools: []ToolDescriptor{
				{Name: "echo"},
			},
		},
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:   "server",
		Name: "Server",
	}, WithClient(client), func(opts *adapterOptions) {
		opts.now = func() time.Time {
			return time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC)
		}
	})
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	info := ext.Info()
	if info.Health.Message != "mcp server active" {
		t.Fatalf("expected active message without skipped tools, got %q", info.Health.Message)
	}
}

func TestMCPToolDefinitionSpecAndRunBranches(t *testing.T) {
	tool := mcpTool{
		descriptor: ToolDescriptor{},
	}
	def := tool.Definition()
	if def.Function.Name != "mcp_tool" {
		t.Fatalf("expected fallback name, got %q", def.Function.Name)
	}
	if def.Function.Description != "MCP tool mcp_tool" {
		t.Fatalf("expected fallback description, got %q", def.Function.Description)
	}
	if def.Function.Parameters["type"] != "object" {
		t.Fatalf("expected normalized object schema, got %#v", def.Function.Parameters)
	}

	spec := tool.Spec()
	if spec.SafetyClass != toolspkg.SafetyClassSensitive {
		t.Fatalf("expected sensitive safety class, got %q", spec.SafetyClass)
	}
	if spec.Name != "mcp_tool" {
		t.Fatalf("expected fallback spec name, got %q", spec.Name)
	}

	namedTool := mcpTool{
		descriptor: ToolDescriptor{Name: "echo", Description: "echo"},
		client:     &stubClient{callOutput: "ok"},
	}
	spec = namedTool.Spec()
	if spec.Name != "echo" {
		t.Fatalf("expected spec name to keep descriptor name, got %q", spec.Name)
	}

	tool = mcpTool{}
	_, err := tool.Run(context.Background(), json.RawMessage(`{}`), nil)
	if err == nil {
		t.Fatal("expected internal error when client is nil")
	}
	execErr, ok := toolspkg.AsToolExecError(err)
	if !ok || execErr.Code != toolspkg.ToolErrorInternal {
		t.Fatalf("expected internal tool error, got %v", err)
	}

	tool = mcpTool{
		client: &stubClient{
			callErr: &ClientError{Code: ClientErrorPermission, Message: "denied"},
		},
	}
	_, err = tool.Run(context.Background(), json.RawMessage(`{}`), nil)
	execErr, ok = toolspkg.AsToolExecError(err)
	if !ok || execErr.Code != toolspkg.ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tool = mcpTool{
		client:     &stubClient{callOutput: "ok"},
		descriptor: ToolDescriptor{Name: "echo"},
	}
	output, err := tool.Run(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("expected success with deadline context, got %v", err)
	}
	if output != "ok" {
		t.Fatalf("unexpected output: %q", output)
	}
}
