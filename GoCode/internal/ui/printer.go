package ui

import (
	"fmt"
	"io"
	"strings"

	"gocode/internal/config"
	"gocode/internal/session"
)

type Printer struct {
	out io.Writer
}

func NewPrinter(out io.Writer) *Printer {
	return &Printer{out: out}
}

func (p *Printer) PrintWelcome(cfg config.Config) {
	fmt.Fprintf(p.out, "GoCode CLI 已启动\n工作区: %s\n模型: %s\n可用命令: /plan /diff /files /undo /exit\n\n", cfg.Workspace, cfg.Model)
}

func (p *Printer) PrintTask(task session.TaskRecord) {
	if task.Input == "" && task.Summary == "" {
		return
	}
	fmt.Fprintln(p.out, "---")
	fmt.Fprintf(p.out, "任务: %s\n", firstNonEmpty(task.Summary, task.Input))

	if len(task.Plan) > 0 {
		fmt.Fprintln(p.out, "计划:")
		for i, step := range task.Plan {
			fmt.Fprintf(p.out, "  %d. %s\n", i+1, step)
		}
	}

	if len(task.ToolCalls) > 0 {
		fmt.Fprintln(p.out, "工具:")
		for _, name := range uniqueToolNames(task.ToolCalls) {
			fmt.Fprintf(p.out, "  - %s\n", name)
		}
	}

	if len(task.Files) > 0 {
		fmt.Fprintln(p.out, "涉及文件:")
		for _, file := range task.Files {
			fmt.Fprintf(p.out, "  - %s\n", file)
		}
	}

	if len(task.Changes) > 0 {
		fmt.Fprintln(p.out, "改动摘要:")
		for _, change := range task.Changes {
			if change.Detail != "" {
				fmt.Fprintf(p.out, "  - [%s] %s: %s\n", change.Action, change.Path, change.Detail)
				continue
			}
			fmt.Fprintf(p.out, "  - [%s] %s\n", change.Action, change.Path)
		}
	}

	if len(task.Commands) > 0 {
		fmt.Fprintln(p.out, "命令结果:")
		for _, command := range task.Commands {
			fmt.Fprintf(p.out, "  - (%d) %s @ %s\n", command.ExitCode, command.Command, firstNonEmpty(command.Cwd, "."))
			for _, line := range previewLines(command.Output, 6) {
				fmt.Fprintf(p.out, "      %s\n", line)
			}
		}
	}

	fmt.Fprintf(p.out, "状态: %s\n", firstNonEmpty(task.Status, "completed"))
	if strings.TrimSpace(task.Assistant) != "" {
		fmt.Fprintf(p.out, "说明: %s\n", task.Assistant)
	}
	fmt.Fprintln(p.out)
}

func (p *Printer) PrintPlan(plan []string) {
	if len(plan) == 0 {
		p.PrintInfo("暂无可展示的计划。")
		return
	}
	fmt.Fprintln(p.out, "当前计划:")
	for i, step := range plan {
		fmt.Fprintf(p.out, "  %d. %s\n", i+1, step)
	}
	fmt.Fprintln(p.out)
}

func (p *Printer) PrintFiles(files []string) {
	if len(files) == 0 {
		p.PrintInfo("暂无涉及文件。")
		return
	}
	fmt.Fprintln(p.out, "最近一次任务涉及文件:")
	for _, file := range files {
		fmt.Fprintf(p.out, "  - %s\n", file)
	}
	fmt.Fprintln(p.out)
}

func (p *Printer) PrintDiff(task session.TaskRecord) {
	if len(task.Changes) == 0 {
		p.PrintInfo("最近一次任务没有记录到文件改动。")
		return
	}
	fmt.Fprintf(p.out, "最近一次任务改动: %s\n", firstNonEmpty(task.Summary, task.Input))
	for _, change := range task.Changes {
		if change.Detail != "" {
			fmt.Fprintf(p.out, "  - [%s] %s: %s\n", change.Action, change.Path, change.Detail)
			continue
		}
		fmt.Fprintf(p.out, "  - [%s] %s\n", change.Action, change.Path)
	}
	fmt.Fprintln(p.out)
}

func (p *Printer) PrintInfo(message string) {
	fmt.Fprintln(p.out, message)
}

func (p *Printer) PrintWarning(message string) {
	fmt.Fprintf(p.out, "提示: %s\n", message)
}

func (p *Printer) PrintError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(p.out, "错误: %v\n", err)
}

func uniqueToolNames(calls []session.ToolCall) []string {
	seen := make(map[string]struct{}, len(calls))
	result := make([]string, 0, len(calls))
	for _, call := range calls {
		if _, ok := seen[call.Name]; ok {
			continue
		}
		seen[call.Name] = struct{}{}
		result = append(result, call.Name)
	}
	return result
}

func previewLines(output string, max int) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []string{"(no output)"}
	}
	lines := strings.Split(trimmed, "\n")
	if max > 0 && len(lines) > max {
		lines = append(lines[:max], "...")
	}
	return lines
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
