package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"aicoding/internal/llm"
	"aicoding/internal/session"
)

type ExecutionContext struct {
	Workspace      string
	ApprovalPolicy string
	Session        *session.Session
	Stdin          io.Reader
	Stdout         io.Writer
}

type Tool interface {
	Definition() llm.ToolDefinition
	Run(context.Context, json.RawMessage, *ExecutionContext) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func DefaultRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.Add(ListFilesTool{})
	r.Add(ReadFileTool{})
	r.Add(SearchTextTool{})
	r.Add(WriteFileTool{})
	r.Add(ReplaceInFileTool{})
	r.Add(ApplyPatchTool{})
	r.Add(UpdatePlanTool{})
	r.Add(RunShellTool{})
	return r
}

func (r *Registry) Add(tool Tool) {
	r.tools[tool.Definition().Function.Name] = tool
}

func (r *Registry) Definitions() []llm.ToolDefinition {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}

func (r *Registry) Execute(ctx context.Context, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if rawArgs == "" {
		rawArgs = "{}"
	}
	return tool.Run(ctx, json.RawMessage(rawArgs), execCtx)
}
