package tools

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

type RegistrationSource string

const (
	RegistrationSourceBuiltin   RegistrationSource = "builtin"
	RegistrationSourceExtension RegistrationSource = "extension"
)

type RegisterOptions struct {
	Source      RegistrationSource
	ExtensionID string
}

type RegistrationMeta struct {
	ToolKey        string
	Source         RegistrationSource
	ExtensionID    string
	ConflictPolicy string
}

type RegistryErrorCode string

const (
	RegistryErrorDuplicateName  RegistryErrorCode = "duplicate_name"
	RegistryErrorInvalidSource  RegistryErrorCode = "invalid_source"
	RegistryErrorInvalidSchema  RegistryErrorCode = "invalid_schema"
	RegistryErrorInvalidTool    RegistryErrorCode = "invalid_tool"
	RegistryErrorInvalidToolKey RegistryErrorCode = "invalid_tool_key"
	RegistryErrorNotFound       RegistryErrorCode = "not_found"
)

type RegistryError struct {
	Code         RegistryErrorCode
	Message      string
	ToolKey      string
	Source       RegistrationSource
	ExtensionID  string
	ConflictWith RegistrationMeta
	Cause        error
}

func (e *RegistryError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.ToolKey) != "" {
		return fmt.Sprintf("registry error for tool %q", e.ToolKey)
	}
	return string(e.Code)
}

func (e *RegistryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func buildRegistration(tool Tool, opts RegisterOptions) (RegistrationMeta, ResolvedTool, error) {
	meta := RegistrationMeta{
		Source:         normalizeRegistrationSource(opts.Source),
		ExtensionID:    strings.TrimSpace(opts.ExtensionID),
		ConflictPolicy: "reject",
	}
	if meta.Source == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "registration source is required", Source: opts.Source}
	}
	if meta.Source == RegistrationSourceBuiltin && meta.ExtensionID != "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "builtin registration cannot include extension id", Source: meta.Source, ExtensionID: meta.ExtensionID}
	}
	if meta.Source == RegistrationSourceExtension && meta.ExtensionID == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "extension registration requires extension id", Source: meta.Source}
	}
	definition := cloneToolDefinition(tool.Definition())
	meta.ToolKey = strings.TrimSpace(definition.Function.Name)
	if meta.ToolKey == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidToolKey, Message: "tool key is required", Source: meta.Source, ExtensionID: meta.ExtensionID}
	}
	definition.Function.Name = meta.ToolKey
	if provider, ok := tool.(ToolSpecProvider); ok {
		spec := NormalizeToolSpec(MergeToolSpec(DefaultToolSpec(definition), provider.Spec()))
		if strings.TrimSpace(spec.Name) != "" && spec.Name != meta.ToolKey {
			return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidToolKey, Message: fmt.Sprintf("tool spec name %q must match tool key %q", spec.Name, meta.ToolKey), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID}
		}
		if err := ValidateToolSpec(spec); err != nil {
			return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSchema, Message: err.Error(), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID, Cause: err}
		}
		return meta, ResolvedTool{Definition: definition, Spec: cloneToolSpec(spec), Tool: tool}, nil
	}
	spec := NormalizeToolSpec(DefaultToolSpec(definition))
	if err := ValidateToolSpec(spec); err != nil {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSchema, Message: err.Error(), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID, Cause: err}
	}
	return meta, ResolvedTool{Definition: definition, Spec: cloneToolSpec(spec), Tool: tool}, nil
}

func normalizeRegistrationSource(source RegistrationSource) RegistrationSource {
	switch RegistrationSource(strings.TrimSpace(string(source))) {
	case RegistrationSourceBuiltin:
		return RegistrationSourceBuiltin
	case RegistrationSourceExtension:
		return RegistrationSourceExtension
	default:
		return ""
	}
}

func cloneResolvedTool(resolved ResolvedTool) ResolvedTool {
	return ResolvedTool{Definition: cloneToolDefinition(resolved.Definition), Spec: cloneToolSpec(resolved.Spec), Tool: resolved.Tool}
}

func cloneToolSpec(spec ToolSpec) ToolSpec {
	cloned := spec
	if spec.AllowedModes != nil {
		cloned.AllowedModes = append([]planpkg.AgentMode(nil), spec.AllowedModes...)
	}
	return cloned
}

func cloneToolDefinition(def llm.ToolDefinition) llm.ToolDefinition {
	cloned := def
	cloned.Function = def.Function
	cloned.Function.Parameters = cloneAnyMap(def.Function.Parameters)
	return cloned
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAnySlice(input []any) []any {
	if input == nil {
		return nil
	}
	cloned := make([]any, len(input))
	for i, value := range input {
		cloned[i] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		return cloneAnySlice(typed)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []map[string]any:
		cloned := make([]map[string]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneAnyMap(item)
		}
		return cloned
	case []map[string]string:
		cloned := make([]map[string]string, len(typed))
		for i, item := range typed {
			next := make(map[string]string, len(item))
			for key, value := range item {
				next[key] = value
			}
			cloned[i] = next
		}
		return cloned
	default:
		return typed
	}
}
