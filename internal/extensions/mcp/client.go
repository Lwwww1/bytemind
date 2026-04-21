package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultStartupTimeout = 5 * time.Second
	defaultCallTimeout    = 30 * time.Second
)

type ServerConfig struct {
	ID             string
	Name           string
	Version        string
	Command        string
	Args           []string
	Env            map[string]string
	CWD            string
	StartupTimeout time.Duration
	CallTimeout    time.Duration
}

type ToolDescriptor struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ServerSnapshot struct {
	ID      string
	Name    string
	Version string
	Tools   []ToolDescriptor
}

type Client interface {
	Discover(ctx context.Context, cfg ServerConfig) (ServerSnapshot, error)
	CallTool(ctx context.Context, cfg ServerConfig, toolName string, raw json.RawMessage) (string, error)
}

type ClientErrorCode string

const (
	ClientErrorInvalidConfig   ClientErrorCode = "invalid_config"
	ClientErrorTransport       ClientErrorCode = "transport_error"
	ClientErrorProtocol        ClientErrorCode = "protocol_error"
	ClientErrorTimeout         ClientErrorCode = "timeout"
	ClientErrorHandshakeFailed ClientErrorCode = "handshake_failed"
	ClientErrorListToolsFailed ClientErrorCode = "tools_list_failed"
	ClientErrorCallFailed      ClientErrorCode = "call_failed"
	ClientErrorPermission      ClientErrorCode = "permission_denied"
	ClientErrorInvalidArgs     ClientErrorCode = "invalid_args"
)

type ClientError struct {
	Code    ClientErrorCode
	Message string
	Err     error
}

func (e *ClientError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newClientError(code ClientErrorCode, message string, err error) error {
	return &ClientError{
		Code:    code,
		Message: strings.TrimSpace(message),
		Err:     err,
	}
}

type StdioClient struct{}

func NewStdioClient() *StdioClient {
	return &StdioClient{}
}

func (c *StdioClient) Discover(ctx context.Context, cfg ServerConfig) (ServerSnapshot, error) {
	cfg = normalizeServerConfig(cfg)
	if err := validateServerConfig(cfg, true); err != nil {
		return ServerSnapshot{}, err
	}

	callCtx, cancel := withTimeoutIfMissing(ctx, cfg.StartupTimeout)
	defer cancel()

	responses, err := c.runRPC(callCtx, cfg, []rpcRequest{
		newRPCRequest(1, "initialize", map[string]any{
			"protocolVersion": "2026-04-01",
			"clientInfo": map[string]any{
				"name":    "bytemind",
				"version": "dev",
			},
		}),
		newRPCRequest(2, "tools/list", map[string]any{}),
	})
	if err != nil {
		return ServerSnapshot{}, err
	}
	if len(responses) != 2 {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "mcp server returned incomplete discovery responses", nil)
	}

	initResult := struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}{}
	if err := json.Unmarshal(responses[0].Result, &initResult); err != nil {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "failed to decode initialize result", err)
	}

	toolsResult := struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"tools"`
	}{}
	if err := json.Unmarshal(responses[1].Result, &toolsResult); err != nil {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "failed to decode tools/list result", err)
	}

	descriptors := make([]ToolDescriptor, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		schema := cloneMap(tool.InputSchema)
		if len(schema) == 0 {
			schema = cloneMap(tool.Parameters)
		}
		descriptors = append(descriptors, ToolDescriptor{
			Name:        strings.TrimSpace(tool.Name),
			Description: strings.TrimSpace(tool.Description),
			InputSchema: schema,
		})
	}

	name := strings.TrimSpace(initResult.ServerInfo.Name)
	if name == "" {
		name = cfg.Name
	}
	version := strings.TrimSpace(initResult.ServerInfo.Version)
	if version == "" {
		version = cfg.Version
	}
	return ServerSnapshot{
		ID:      cfg.ID,
		Name:    name,
		Version: version,
		Tools:   descriptors,
	}, nil
}

func (c *StdioClient) CallTool(ctx context.Context, cfg ServerConfig, toolName string, raw json.RawMessage) (string, error) {
	cfg = normalizeServerConfig(cfg)
	if err := validateServerConfig(cfg, true); err != nil {
		return "", err
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", newClientError(ClientErrorInvalidArgs, "tool name is required", nil)
	}

	args := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", newClientError(ClientErrorInvalidArgs, "tool arguments must be a JSON object", err)
		}
	}

	callCtx, cancel := withTimeoutIfMissing(ctx, cfg.CallTimeout)
	defer cancel()

	responses, err := c.runRPC(callCtx, cfg, []rpcRequest{
		newRPCRequest(1, "initialize", map[string]any{
			"protocolVersion": "2026-04-01",
			"clientInfo": map[string]any{
				"name":    "bytemind",
				"version": "dev",
			},
		}),
		newRPCRequest(2, "tools/call", map[string]any{
			"name":      toolName,
			"arguments": args,
		}),
	})
	if err != nil {
		return "", err
	}
	if len(responses) != 2 {
		return "", newClientError(ClientErrorProtocol, "mcp server returned incomplete call responses", nil)
	}

	callResult := struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}{}
	if err := json.Unmarshal(responses[1].Result, &callResult); err == nil {
		if callResult.IsError {
			return "", newClientError(ClientErrorCallFailed, fmt.Sprintf("mcp tool %q returned isError", toolName), nil)
		}
		parts := make([]string, 0, len(callResult.Content))
		for _, item := range callResult.Content {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			parts = append(parts, item.Text)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
	}

	compact, err := compactJSON(responses[1].Result)
	if err != nil {
		return "", newClientError(ClientErrorProtocol, "failed to normalize tools/call result", err)
	}
	return compact, nil
}

func (c *StdioClient) runRPC(ctx context.Context, cfg ServerConfig, requests []rpcRequest) ([]rpcResponse, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if strings.TrimSpace(cfg.CWD) != "" {
		cmd.Dir = cfg.CWD
	}
	cmd.Env = mergeCommandEnv(cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to open mcp stdin pipe", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to open mcp stdout pipe", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to start mcp server process", err)
	}
	defer stopCommand(cmd, stdin)

	reader := bufio.NewReader(stdout)
	writer := bufio.NewWriter(stdin)
	responses := make([]rpcResponse, 0, len(requests))

	for _, request := range requests {
		if err := writeRPCRequest(writer, request); err != nil {
			return nil, newClientError(ClientErrorTransport, "failed to write mcp request", err)
		}
		response, err := readRPCResponse(reader)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, newClientError(ClientErrorTimeout, "mcp request timed out", err)
			}
			if errors.Is(err, io.EOF) {
				message := "mcp server closed stdout before replying"
				if trimmed := strings.TrimSpace(stderr.String()); trimmed != "" {
					message = fmt.Sprintf("%s: %s", message, trimmed)
				}
				return nil, newClientError(ClientErrorTransport, message, err)
			}
			return nil, newClientError(ClientErrorProtocol, "failed to read mcp response", err)
		}
		if response.ID != request.ID {
			return nil, newClientError(ClientErrorProtocol, "mcp response id mismatch", nil)
		}
		if response.Error != nil {
			return nil, mapRPCError(request.Method, response.Error)
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func normalizeServerConfig(cfg ServerConfig) ServerConfig {
	cfg.ID = normalizeID(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Version = strings.TrimSpace(cfg.Version)
	cfg.Command = strings.TrimSpace(cfg.Command)
	cfg.CWD = strings.TrimSpace(cfg.CWD)
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = defaultCallTimeout
	}
	if cfg.Args == nil {
		cfg.Args = []string{}
	}
	cfg.Env = cloneStringMap(cfg.Env)
	return cfg
}

func validateServerConfig(cfg ServerConfig, requireCommand bool) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return newClientError(ClientErrorInvalidConfig, "mcp server id is required", nil)
	}
	if requireCommand && strings.TrimSpace(cfg.Command) == "" {
		return newClientError(ClientErrorInvalidConfig, "mcp server command is required", nil)
	}
	if strings.TrimSpace(cfg.CWD) != "" {
		if _, err := os.Stat(cfg.CWD); err != nil {
			return newClientError(ClientErrorInvalidConfig, "mcp server cwd is not accessible", err)
		}
	}
	return nil
}

func withTimeoutIfMissing(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func mergeCommandEnv(extra map[string]string) []string {
	base := map[string]string{}
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		base[parts[0]] = parts[1]
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		base[key] = value
	}
	env := make([]string, 0, len(base))
	for key, value := range base {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

func stopCommand(cmd *exec.Cmd, stdin io.WriteCloser) {
	if stdin != nil {
		_ = stdin.Close()
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(200 * time.Millisecond):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-done
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newRPCRequest(id int, method string, params any) rpcRequest {
	return rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

func writeRPCRequest(writer *bufio.Writer, request rpcRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	if err := writer.WriteByte('\n'); err != nil {
		return err
	}
	return writer.Flush()
}

func readRPCResponse(reader *bufio.Reader) (rpcResponse, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return rpcResponse{}, err
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return rpcResponse{}, io.EOF
	}
	var response rpcResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return rpcResponse{}, err
	}
	return response, nil
}

func mapRPCError(method string, rpcErr *rpcError) error {
	if rpcErr == nil {
		return nil
	}
	message := strings.TrimSpace(rpcErr.Message)
	if message == "" {
		message = "mcp server returned an unknown error"
	}
	switch strings.TrimSpace(method) {
	case "initialize":
		return newClientError(ClientErrorHandshakeFailed, message, nil)
	case "tools/list":
		return newClientError(ClientErrorListToolsFailed, message, nil)
	case "tools/call":
		switch rpcErr.Code {
		case -32602:
			return newClientError(ClientErrorInvalidArgs, message, nil)
		case -32001:
			return newClientError(ClientErrorPermission, message, nil)
		default:
			return newClientError(ClientErrorCallFailed, message, nil)
		}
	default:
		return newClientError(ClientErrorProtocol, message, nil)
	}
}

func compactJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return "", err
	}
	return out.String(), nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func normalizeID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-_")
	if raw == "" {
		return ""
	}
	return raw
}
