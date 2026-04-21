package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientErrorHelpers(t *testing.T) {
	var nilErr *ClientError
	if nilErr.Error() != "" {
		t.Fatalf("expected empty string for nil error, got %q", nilErr.Error())
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("expected nil unwrap for nil error")
	}

	cause := errors.New("boom")
	err := &ClientError{Code: ClientErrorProtocol, Message: "  ", Err: cause}
	if err.Error() != string(ClientErrorProtocol) {
		t.Fatalf("unexpected message without text: %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected unwrap to include cause")
	}

	wrapped := newClientError(ClientErrorInvalidArgs, "  bad args  ", nil)
	var clientErr *ClientError
	if !errors.As(wrapped, &clientErr) {
		t.Fatalf("expected ClientError, got %T", wrapped)
	}
	if clientErr.Message != "bad args" {
		t.Fatalf("expected trimmed message, got %q", clientErr.Message)
	}
}

func TestValidateServerConfig(t *testing.T) {
	valid := ServerConfig{ID: "id", Command: "cmd"}
	if err := validateServerConfig(valid, true); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	cases := []struct {
		name string
		cfg  ServerConfig
		req  bool
		code ClientErrorCode
	}{
		{
			name: "missing id",
			cfg:  ServerConfig{Command: "cmd"},
			req:  true,
			code: ClientErrorInvalidConfig,
		},
		{
			name: "missing command when required",
			cfg:  ServerConfig{ID: "id"},
			req:  true,
			code: ClientErrorInvalidConfig,
		},
		{
			name: "missing cwd",
			cfg: ServerConfig{
				ID:      "id",
				Command: "cmd",
				CWD:     filepath.Join(t.TempDir(), "missing"),
			},
			req:  true,
			code: ClientErrorInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateServerConfig(tc.cfg, tc.req)
			assertClientErrorCode(t, err, tc.code)
		})
	}

	if err := validateServerConfig(ServerConfig{ID: "id"}, false); err != nil {
		t.Fatalf("command should be optional when not required, got %v", err)
	}
}

func TestWithTimeoutIfMissing(t *testing.T) {
	ctx := context.Background()
	withoutTimeout, cancel := withTimeoutIfMissing(ctx, 0)
	defer cancel()
	if _, has := withoutTimeout.Deadline(); has {
		t.Fatal("did not expect deadline when timeout <= 0")
	}

	parent, parentCancel := context.WithTimeout(context.Background(), time.Second)
	defer parentCancel()
	inherited, inheritedCancel := withTimeoutIfMissing(parent, 5*time.Second)
	defer inheritedCancel()
	parentDeadline, _ := parent.Deadline()
	inheritedDeadline, hasDeadline := inherited.Deadline()
	if !hasDeadline {
		t.Fatal("expected inherited deadline")
	}
	if !inheritedDeadline.Equal(parentDeadline) {
		t.Fatalf("expected inherited deadline %v, got %v", parentDeadline, inheritedDeadline)
	}

	withTimeout, withTimeoutCancel := withTimeoutIfMissing(context.Background(), 50*time.Millisecond)
	defer withTimeoutCancel()
	if _, has := withTimeout.Deadline(); !has {
		t.Fatal("expected deadline when timeout is set")
	}
}

func TestMapRPCError(t *testing.T) {
	if err := mapRPCError("initialize", nil); err != nil {
		t.Fatalf("expected nil rpc error to map to nil, got %v", err)
	}

	cases := []struct {
		name   string
		method string
		err    *rpcError
		code   ClientErrorCode
	}{
		{
			name:   "initialize maps handshake",
			method: "initialize",
			err:    &rpcError{Code: -32000, Message: "x"},
			code:   ClientErrorHandshakeFailed,
		},
		{
			name:   "tools list maps list error",
			method: "tools/list",
			err:    &rpcError{Code: -32000, Message: "x"},
			code:   ClientErrorListToolsFailed,
		},
		{
			name:   "tools call invalid args",
			method: "tools/call",
			err:    &rpcError{Code: -32602, Message: "x"},
			code:   ClientErrorInvalidArgs,
		},
		{
			name:   "tools call permission",
			method: "tools/call",
			err:    &rpcError{Code: -32001, Message: "x"},
			code:   ClientErrorPermission,
		},
		{
			name:   "tools call default",
			method: "tools/call",
			err:    &rpcError{Code: -32099, Message: "x"},
			code:   ClientErrorCallFailed,
		},
		{
			name:   "unknown method",
			method: "unknown",
			err:    &rpcError{Code: -32099, Message: "x"},
			code:   ClientErrorProtocol,
		},
		{
			name:   "empty message fallback",
			method: "initialize",
			err:    &rpcError{Code: -32000, Message: "   "},
			code:   ClientErrorHandshakeFailed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := mapRPCError(tc.method, tc.err)
			assertClientErrorCode(t, mapped, tc.code)
		})
	}
}

func TestCompactJSONHelpers(t *testing.T) {
	output, err := compactJSON(nil)
	if err != nil {
		t.Fatalf("compactJSON(nil) failed: %v", err)
	}
	if output != "" {
		t.Fatalf("expected empty output, got %q", output)
	}

	output, err = compactJSON(json.RawMessage(`{ "a": 1 }`))
	if err != nil {
		t.Fatalf("compactJSON(valid) failed: %v", err)
	}
	if output != `{"a":1}` {
		t.Fatalf("unexpected compact output: %q", output)
	}

	if _, err := compactJSON(json.RawMessage(`{`)); err == nil {
		t.Fatal("expected invalid json error")
	}
}

func TestWriteRPCRequestAndReadRPCResponseErrors(t *testing.T) {
	roundtrip := &strings.Builder{}
	writer := bufio.NewWriter(roundtrip)
	if err := writeRPCRequest(writer, newRPCRequest(1, "ping", map[string]any{"a": 1})); err != nil {
		t.Fatalf("writeRPCRequest roundtrip failed: %v", err)
	}
	reader := bufio.NewReader(strings.NewReader(roundtrip.String()))
	response, err := readRPCResponse(reader)
	if err != nil {
		t.Fatalf("readRPCResponse roundtrip failed: %v", err)
	}
	if response.ID != 1 {
		t.Fatalf("unexpected response id: %#v", response)
	}

	errWriter := bufio.NewWriterSize(alwaysFailWriter{err: io.ErrClosedPipe}, 1)
	if err := writeRPCRequest(errWriter, newRPCRequest(1, "ping", map[string]any{"a": 1})); err == nil {
		t.Fatal("expected write error")
	}

	if _, err := readRPCResponse(bufio.NewReader(strings.NewReader("\n"))); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF for blank line, got %v", err)
	}
	if _, err := readRPCResponse(bufio.NewReader(strings.NewReader("{\n"))); err == nil {
		t.Fatal("expected invalid json error")
	}
	if _, err := readRPCResponse(bufio.NewReader(alwaysFailReader{err: io.ErrUnexpectedEOF})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected wrapped reader error, got %v", err)
	}
}

func TestDiscoverFallbackAndDecodeErrorPaths(t *testing.T) {
	client := NewStdioClient()

	cfg := helperServerConfig(t, "discover_empty_server_info")
	cfg.Name = "fallback-name"
	cfg.Version = "9.9.9"
	snapshot, err := client.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover fallback scenario failed: %v", err)
	}
	if snapshot.Name != "fallback-name" {
		t.Fatalf("expected fallback name, got %q", snapshot.Name)
	}
	if snapshot.Version != "9.9.9" {
		t.Fatalf("expected fallback version, got %q", snapshot.Version)
	}

	cfg = helperServerConfig(t, "discover_invalid_initialize_result")
	_, err = client.Discover(context.Background(), cfg)
	assertClientErrorCode(t, err, ClientErrorProtocol)

	cfg = helperServerConfig(t, "discover_invalid_tools_result")
	_, err = client.Discover(context.Background(), cfg)
	assertClientErrorCode(t, err, ClientErrorProtocol)
}

func TestCallToolErrorAndFallbackPaths(t *testing.T) {
	client := NewStdioClient()

	cfg := helperServerConfig(t, "call_ok")
	_, err := client.CallTool(context.Background(), cfg, "", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorInvalidArgs)

	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`[]`))
	assertClientErrorCode(t, err, ClientErrorInvalidArgs)

	cfg = helperServerConfig(t, "call_fail")
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorCallFailed)

	cfg = helperServerConfig(t, "call_is_error")
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorCallFailed)

	cfg = helperServerConfig(t, "call_compact_fallback")
	output, err := client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("compact fallback call failed: %v", err)
	}
	if output != `{"foo":"bar"}` {
		t.Fatalf("expected compact fallback output, got %q", output)
	}

	cfg = helperServerConfig(t, "bad_response_id")
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorProtocol)

	cfg = helperServerConfig(t, "invalid_json_line")
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorProtocol)

	cfg = helperServerConfig(t, "eof_with_stderr")
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorTransport)

	cfg = helperServerConfig(t, "sleep")
	cfg.CallTimeout = 20 * time.Millisecond
	_, err = client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{}`))
	assertClientErrorCode(t, err, ClientErrorTimeout)
}

func TestRunRPCZeroRequestAndStopCommandBranches(t *testing.T) {
	client := NewStdioClient()
	responses, err := client.runRPC(context.Background(), ServerConfig{}, nil)
	if err != nil {
		t.Fatalf("runRPC with empty requests failed: %v", err)
	}
	if responses != nil {
		t.Fatalf("expected nil responses for empty request list, got %#v", responses)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to resolve test executable: %v", err)
	}

	quick := exec.Command(exe, "-test.run=^TestMCPHelperProcess$")
	quick.Env = append(os.Environ(), "BYTEMIND_MCP_HELPER=1", "BYTEMIND_MCP_SCENARIO=eof_with_stderr")
	quickIn, err := quick.StdinPipe()
	if err != nil {
		t.Fatalf("quick stdin pipe failed: %v", err)
	}
	if err := quick.Start(); err != nil {
		t.Fatalf("quick start failed: %v", err)
	}
	stopCommand(quick, quickIn)

	slow := exec.Command(exe, "-test.run=^TestMCPHelperProcess$")
	slow.Env = append(os.Environ(), "BYTEMIND_MCP_HELPER=1", "BYTEMIND_MCP_SCENARIO=sleep")
	if _, err := slow.StdinPipe(); err != nil {
		t.Fatalf("slow stdin pipe failed: %v", err)
	}
	if err := slow.Start(); err != nil {
		t.Fatalf("slow start failed: %v", err)
	}
	stopCommand(slow, nil)
}

func TestNormalizeIDAndCloneMapHelpers(t *testing.T) {
	if got := normalizeID("  GitHub/Repo.Name  "); got != "github-repo-name" {
		t.Fatalf("unexpected normalized id: %q", got)
	}
	if got := normalizeID("   "); got != "" {
		t.Fatalf("expected empty id, got %q", got)
	}

	if cloneMap(nil) != nil {
		t.Fatal("expected nil clone for nil map")
	}
	if cloneStringMap(nil) != nil {
		t.Fatal("expected nil clone for nil string map")
	}

	source := map[string]any{"a": 1}
	cloned := cloneMap(source)
	cloned["a"] = 2
	if source["a"].(int) != 1 {
		t.Fatal("cloneMap should not mutate source map")
	}

	sourceEnv := map[string]string{"A": "1"}
	clonedEnv := cloneStringMap(sourceEnv)
	clonedEnv["A"] = "2"
	if sourceEnv["A"] != "1" {
		t.Fatal("cloneStringMap should not mutate source map")
	}
}

func assertClientErrorCode(t *testing.T, err error, code ClientErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected client error %q, got nil", code)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T (%v)", err, err)
	}
	if clientErr.Code != code {
		t.Fatalf("expected code %q, got %q (err=%v)", code, clientErr.Code, err)
	}
}

type alwaysFailWriter struct {
	err error
}

func (w alwaysFailWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

type alwaysFailReader struct {
	err error
}

func (r alwaysFailReader) Read(_ []byte) (int, error) {
	return 0, r.err
}
