package tools

import (
	"fmt"
	"strings"
)

func approvalChannelUnavailableMessage(kind, command string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "operation"
	}
	command = strings.TrimSpace(command)
	if command == "" {
		command = "unknown"
	}
	return fmt.Sprintf("%s %q requires approval but approval channel is unavailable (missing approval handler and stdin fallback)", kind, command)
}

func approvalChannelUnavailableError(kind, command string) *ToolExecError {
	return NewToolExecError(ToolErrorPermissionDenied, approvalChannelUnavailableMessage(kind, command), false, nil)
}
