package session

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu        sync.Mutex
	workspace string
	tasks     []TaskRecord
	current   *TaskRecord
}

func NewStore(workspace string) *Store {
	return &Store{workspace: workspace}
}

func (s *Store) BeginTask(input string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.current = &TaskRecord{
		Input:     strings.TrimSpace(input),
		StartedAt: time.Now(),
		Status:    "running",
	}
}

func (s *Store) SetSummary(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	s.current.Summary = strings.TrimSpace(summary)
}

func (s *Store) SetPlan(plan []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	s.current.Plan = normalizeList(plan)
}

func (s *Store) AddToolCall(name, args string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	s.current.ToolCalls = append(s.current.ToolCalls, ToolCall{
		Name: strings.TrimSpace(name),
		Args: truncate(args, 400),
		At:   time.Now(),
	})
}

func (s *Store) AddFile(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return
	}
	s.current.Files = uniqueAppend(s.current.Files, trimmed)
	sort.Strings(s.current.Files)
}

func (s *Store) AddChange(action, path, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	change := Change{
		Action: strings.TrimSpace(action),
		Path:   strings.TrimSpace(path),
		Detail: strings.TrimSpace(detail),
	}
	s.current.Changes = append(s.current.Changes, change)
	if change.Path != "" {
		s.current.Files = uniqueAppend(s.current.Files, change.Path)
		sort.Strings(s.current.Files)
	}
}

func (s *Store) AddCommand(result CommandResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	result.Output = truncate(result.Output, 4000)
	result.Cwd = strings.TrimSpace(result.Cwd)
	s.current.Commands = append(s.current.Commands, result)
}

func (s *Store) SetAssistant(note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return
	}
	s.current.Assistant = strings.TrimSpace(note)
}

func (s *Store) CompleteTask(status string) TaskRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return TaskRecord{}
	}
	task := cloneTask(*s.current)
	task.Status = strings.TrimSpace(status)
	if task.Status == "" {
		task.Status = "completed"
	}
	task.FinishedAt = time.Now()
	s.tasks = append(s.tasks, task)
	s.current = nil
	return cloneTask(task)
}

func (s *Store) FailCurrentTask(note string) TaskRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return TaskRecord{}
	}
	if strings.TrimSpace(note) != "" {
		s.current.Assistant = strings.TrimSpace(note)
	}
	task := cloneTask(*s.current)
	task.Status = "failed"
	task.FinishedAt = time.Now()
	s.tasks = append(s.tasks, task)
	s.current = nil
	return cloneTask(task)
}

func (s *Store) LastTask() (TaskRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) == 0 {
		return TaskRecord{}, false
	}
	return cloneTask(s.tasks[len(s.tasks)-1]), true
}

func (s *Store) LastPlan() []string {
	task, ok := s.LastTask()
	if !ok {
		return nil
	}
	return append([]string(nil), task.Plan...)
}

func (s *Store) LastFiles() []string {
	task, ok := s.LastTask()
	if !ok {
		return nil
	}
	return append([]string(nil), task.Files...)
}

func (s *Store) MarkLastTaskUndone(note string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) == 0 {
		return
	}
	s.tasks[len(s.tasks)-1].Status = "undone"
	if strings.TrimSpace(note) != "" {
		s.tasks[len(s.tasks)-1].Assistant = strings.TrimSpace(note)
	}
	s.tasks[len(s.tasks)-1].FinishedAt = time.Now()
}

func (s *Store) HistoryDigest(limit int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) == 0 {
		return "No prior tasks."
	}
	if limit <= 0 || limit > len(s.tasks) {
		limit = len(s.tasks)
	}
	start := len(s.tasks) - limit
	var builder strings.Builder
	for i := start; i < len(s.tasks); i++ {
		task := s.tasks[i]
		builder.WriteString(fmt.Sprintf("Task %d\n", i-start+1))
		builder.WriteString(fmt.Sprintf("- Input: %s\n", task.Input))
		builder.WriteString(fmt.Sprintf("- Status: %s\n", task.Status))
		if task.Assistant != "" {
			builder.WriteString(fmt.Sprintf("- Outcome: %s\n", truncate(task.Assistant, 300)))
		}
		if len(task.Files) > 0 {
			builder.WriteString(fmt.Sprintf("- Files: %s\n", strings.Join(task.Files, ", ")))
		}
		if len(task.Changes) > 0 {
			builder.WriteString(fmt.Sprintf("- Changes: %d\n", len(task.Changes)))
		}
	}
	return strings.TrimSpace(builder.String())
}

func cloneTask(task TaskRecord) TaskRecord {
	copyTask := task
	copyTask.Plan = append([]string(nil), task.Plan...)
	copyTask.ToolCalls = append([]ToolCall(nil), task.ToolCalls...)
	copyTask.Files = append([]string(nil), task.Files...)
	copyTask.Changes = append([]Change(nil), task.Changes...)
	copyTask.Commands = append([]CommandResult(nil), task.Commands...)
	return copyTask
}

func normalizeList(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func uniqueAppend(list []string, value string) []string {
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
