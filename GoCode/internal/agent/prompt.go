package agent

import (
	"fmt"
	"strings"

	"gocode/internal/llm"
)

func plannerMessages(workspace, history, task string) []llm.Message {
	userPrompt := fmt.Sprintf(`Workspace: %s

Recent session history:
%s

Task:
%s

Return strict JSON only with this schema:
{
  "summary": "short task understanding",
  "steps": ["step 1", "step 2", "step 3"],
  "validation": ["optional validation command or check"]
}

Rules:
- Keep summary to one sentence.
- Keep steps to 3-5 concrete items.
- Prefer reading/searching before writing.
- Mention validation only when it is useful.`, workspace, history, task)

	return []llm.Message{
		{
			Role:    "system",
			Content: "You are planning a local coding agent task. Respond with valid JSON only and no markdown fences.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}
}

func executorMessages(workspace, history string, plan taskPlan, task string) []llm.Message {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Workspace: %s\n\n", workspace))
	builder.WriteString("Recent session history:\n")
	builder.WriteString(history)
	builder.WriteString("\n\n")
	builder.WriteString("Current task summary:\n")
	builder.WriteString(firstNonEmpty(plan.Summary, task))
	builder.WriteString("\n\nPlan:\n")
	for i, step := range plan.Steps {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}
	if len(plan.Validation) > 0 {
		builder.WriteString("\nSuggested validation:\n")
		for i, step := range plan.Validation {
			builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
	}
	builder.WriteString("\nUser request:\n")
	builder.WriteString(task)

	return []llm.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`You are GoCode, a local coding agent running inside a CLI.

Rules:
- Operate only inside the provided workspace.
- Read or search relevant files before modifying them, unless you are bootstrapping a new project from scratch.
- Prefer apply_patch for existing files and write_file for new files.
- Use commands only when they meaningfully validate or inspect the work.
- Avoid dangerous operations unless they are necessary; if a delete or overwrite tool exists, the runtime will request confirmation.
- Keep tool calls focused and incremental.
- When the task is complete, reply with a concise plain-text status update that mentions what changed, how it was checked, and any remaining risk.`),
		},
		{
			Role:    "user",
			Content: builder.String(),
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
