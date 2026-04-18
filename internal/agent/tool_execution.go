package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
)

func (r *Runner) executeToolCall(
	ctx context.Context,
	sess *session.Session,
	runMode planpkg.AgentMode,
	call llm.ToolCall,
	out io.Writer,
	allowedTools map[string]struct{},
	deniedTools map[string]struct{},
) error {
	if r.executor == nil {
		return fmt.Errorf("tool executor is unavailable")
	}
	r.emit(Event{
		Type:          EventToolCallStarted,
		SessionID:     corepkg.SessionID(sess.ID),
		ToolName:      call.Function.Name,
		ToolArguments: call.Function.Arguments,
	})
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID: corepkg.SessionID(sess.ID),
		Actor:     "agent",
		Action:    "tool_execute_start",
		Metadata: map[string]string{
			"tool_name": call.Function.Name,
		},
	})
	if out != nil {
		_, _ = io.WriteString(out, ansiBold+ansiCyan+"tool>"+ansiReset+" "+call.Function.Name+"\n")
	}

	execStartedAt := time.Now()
	result, execErr := r.executor.ExecuteForMode(ctx, runMode, call.Function.Name, call.Function.Arguments, &tools.ExecutionContext{
		Workspace:      r.workspace,
		ApprovalPolicy: r.config.ApprovalPolicy,
		ApprovalMode:   r.config.ApprovalMode,
		AwayPolicy:     r.config.AwayPolicy,
		Approval:       r.approval,
		Session:        sess,
		TaskManager:    r.taskManager,
		Extensions:     r.extensions,
		Mode:           runMode,
		Stdin:          r.stdin,
		Stdout:         r.stdout,
		AllowedTools:   allowedTools,
		DeniedTools:    deniedTools,
	})
	errorReasonCode := ""
	errorStatus := ""
	if execErr != nil {
		errorStatus = "error"
		if code := toolErrorReasonCode(execErr); code != "" {
			errorReasonCode = code
			if code == string(tools.ToolErrorPermissionDenied) {
				errorStatus = "denied"
			}
		}
		payload := map[string]any{
			"ok":     false,
			"error":  execErr.Error(),
			"status": errorStatus,
		}
		if errorReasonCode != "" {
			payload["reason_code"] = errorReasonCode
		}
		result = marshalToolResult(payload)
	}
	if out != nil {
		r.renderToolFeedback(out, call.Function.Name, result)
	}

	errText := ""
	if execErr != nil {
		errText = execErr.Error()
	}
	r.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errText,
	})

	auditResult := "ok"
	if execErr != nil {
		auditResult = "error"
	}
	r.appendAudit(ctx, storagepkg.AuditEvent{
		SessionID:  corepkg.SessionID(sess.ID),
		Actor:      "agent",
		Action:     "tool_execute_result",
		Result:     auditResult,
		ReasonCode: errorReasonCode,
		LatencyMS:  time.Since(execStartedAt).Milliseconds(),
		Metadata: map[string]string{
			"tool_name":   call.Function.Name,
			"error":       errText,
			"status":      errorStatus,
			"reason_code": errorReasonCode,
		},
	})

	toolMessage := llm.NewToolResultMessage(call.ID, result)
	if err := llm.ValidateMessage(toolMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, toolMessage)
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return err
		}
	}
	if call.Function.Name == "update_plan" {
		r.emit(Event{
			Type:      EventPlanUpdated,
			SessionID: corepkg.SessionID(sess.ID),
			Plan:      planpkg.CloneState(sess.Plan),
		})
	}
	if shouldStopRunForAwayFailFast(execErr, r.config.ApprovalMode, r.config.AwayPolicy) {
		return fmt.Errorf("away mode fail_fast stopped run after %s permission denial: %w", call.Function.Name, execErr)
	}
	return nil
}

func shouldStopRunForAwayFailFast(err error, approvalMode, awayPolicy string) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(approvalMode) != "away" || strings.TrimSpace(awayPolicy) != "fail_fast" {
		return false
	}
	execErr, ok := tools.AsToolExecError(err)
	if !ok {
		return false
	}
	return execErr.Code == tools.ToolErrorPermissionDenied
}

func toolErrorReasonCode(err error) string {
	if err == nil {
		return ""
	}
	execErr, ok := tools.AsToolExecError(err)
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(execErr.Code))
}
