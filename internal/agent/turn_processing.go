package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
)

type turnProcessParams struct {
	Session          *session.Session
	RunMode          planpkg.AgentMode
	Messages         []llm.Message
	Assets           map[llm.AssetID]llm.ImageAsset
	AllowedToolNames []string
	DeniedToolNames  []string
	AllowedTools     map[string]struct{}
	DeniedTools      map[string]struct{}
	SequenceTracker  *runtimepkg.ToolSequenceTracker
	ExecutedTools    *[]string
	Approval         tools.ApprovalHandler
	TaskReport       *runtimepkg.TaskReport
	Out              io.Writer
}

func (e *defaultEngine) processTurn(ctx context.Context, p turnProcessParams) (string, bool, error) {
	if e == nil || e.runner == nil {
		return "", false, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	if runner.registry == nil {
		return "", false, fmt.Errorf("tool registry is unavailable")
	}
	filteredTools := runner.registry.DefinitionsForModeWithFilters(p.RunMode, p.AllowedToolNames, p.DeniedToolNames)
	request := contextpkg.BuildChatRequest(contextpkg.ChatRequestInput{
		Model:       runner.config.Provider.Model,
		Messages:    p.Messages,
		Tools:       filteredTools,
		Assets:      p.Assets,
		Temperature: 0.2,
	})
	request.Model = runner.modelID()

	streamedText := false
	turnStart := time.Now()
	reply, err := e.completeTurn(ctx, request, p.Out, &streamedText)
	turnLatency := time.Since(turnStart)
	if err != nil {
		estimatedUsage := tokenusage.ResolveTurnUsage(request, nil)
		runner.recordTokenUsage(ctx, p.Session, request, estimatedUsage, turnLatency, false)
		return "", false, err
	}
	reply.Normalize()
	turnUsage := tokenusage.ResolveTurnUsage(request, &reply)
	runner.recordTokenUsage(ctx, p.Session, request, turnUsage, turnLatency, true)
	runner.emitUsageEvent(p.Session, &turnUsage)

	if len(reply.ToolCalls) == 0 {
		answer, finalizeErr := e.finalizeTurnWithoutTools(p.RunMode, p.Session, reply, p.Out, streamedText)
		return answer, true, finalizeErr
	}

	if err := llm.ValidateMessage(reply); err != nil {
		return "", false, err
	}
	sequenceObservation := p.SequenceTracker.Observe(reply.ToolCalls)
	if sequenceObservation.ReachedThreshold {
		summary := runtimepkg.BuildStopSummary(runtimepkg.StopSummaryInput{
			SessionID:     corepkg.SessionID(p.Session.ID),
			Reason:        fmt.Sprintf("I stopped because the assistant repeated the same tool sequence %d times in a row (%s).", sequenceObservation.RepeatCount, strings.Join(sequenceObservation.UniqueToolNames, ", ")),
			ExecutedTools: *p.ExecutedTools,
			TaskReport:    p.TaskReport,
		})
		answer, summaryErr := e.finishWithSummary(p.Session, summary, p.Out, streamedText)
		return answer, true, summaryErr
	}

	p.Session.Messages = append(p.Session.Messages, reply)
	if runner.store != nil {
		if err := runner.store.Save(p.Session); err != nil {
			return "", false, err
		}
	}

	if streamedText && p.Out != nil {
		_, _ = io.WriteString(p.Out, "\n")
	}
	for index, call := range reply.ToolCalls {
		*p.ExecutedTools = append(*p.ExecutedTools, call.Function.Name)
		emitTurnEvent(ctx, TurnEvent{
			Type: TurnEventToolUse,
			Payload: map[string]any{
				"tool_name":      call.Function.Name,
				"tool_arguments": call.Function.Arguments,
				"tool_call_id":   call.ID,
			},
		})
		if err := e.executeToolCall(ctx, p.Session, p.RunMode, call, p.Out, p.AllowedTools, p.DeniedTools, p.Approval); err != nil {
			return "", false, err
		}
		envelope, ok := latestToolResultEnvelope(p.Session)
		if ok && p.TaskReport != nil && envelope.Status == statusDenied {
			p.TaskReport.RecordDenied(call.Function.Name)
		}
		if !isAwayPermissionDenied(envelope, runner.config.ApprovalMode) {
			continue
		}

		if p.TaskReport != nil {
			p.TaskReport.RecordDenied(call.Function.Name)
			p.TaskReport.RecordPendingApproval(call.Function.Name)
		}

		remaining := reply.ToolCalls[index+1:]
		failFast := normalizeAwayPolicy(runner.config.AwayPolicy) == awayPolicyFailFast
		for _, skippedCall := range remaining {
			if p.TaskReport != nil {
				p.TaskReport.RecordSkippedDueToDependency(skippedCall.Function.Name)
			}
			if failFast {
				continue
			}
			if err := e.appendSkippedDependencyResult(ctx, p.Session, skippedCall, p.Out); err != nil {
				return "", false, err
			}
		}
		if failFast {
			return "", false, fmt.Errorf("away_policy=fail_fast stopped run after permission denial")
		}
		break
	}
	return "", false, nil
}

func (r *Runner) processTurn(ctx context.Context, p turnProcessParams) (string, bool, error) {
	engine := &defaultEngine{runner: r}
	return engine.processTurn(ctx, p)
}

const (
	statusError                = "error"
	statusDenied               = "denied"
	statusSkipped              = "skipped"
	reasonCodePermissionDenied = "permission_denied"
	reasonCodeDeniedDependency = "denied_dependency"
	approvalModeInteractive    = "interactive"
	approvalModeAway           = "away"
	awayPolicyAutoDenyContinue = "auto_deny_continue"
	awayPolicyFailFast         = "fail_fast"
)

type toolResultEnvelope struct {
	OK         *bool  `json:"ok"`
	Error      string `json:"error"`
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code"`
}

func latestToolResultEnvelope(sess *session.Session) (toolResultEnvelope, bool) {
	if sess == nil || len(sess.Messages) == 0 {
		return toolResultEnvelope{}, false
	}
	last := sess.Messages[len(sess.Messages)-1]
	content := strings.TrimSpace(last.Content)
	if content == "" {
		return toolResultEnvelope{}, false
	}
	var envelope toolResultEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return toolResultEnvelope{}, false
	}
	envelope.Status = strings.ToLower(strings.TrimSpace(envelope.Status))
	envelope.ReasonCode = strings.ToLower(strings.TrimSpace(envelope.ReasonCode))
	return envelope, true
}

func isAwayPermissionDenied(envelope toolResultEnvelope, approvalMode string) bool {
	if normalizeApprovalMode(approvalMode) != approvalModeAway {
		return false
	}
	return envelope.Status == statusDenied && envelope.ReasonCode == reasonCodePermissionDenied
}

func normalizeApprovalMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return approvalModeInteractive
	}
	return mode
}

func normalizeAwayPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return awayPolicyAutoDenyContinue
	}
	return policy
}

func (e *defaultEngine) appendSkippedDependencyResult(
	ctx context.Context,
	sess *session.Session,
	call llm.ToolCall,
	out io.Writer,
) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	errText := fmt.Sprintf("%s: tool %q was skipped because a prior approval-required action was denied in away mode", reasonCodeDeniedDependency, call.Function.Name)
	result := marshalToolResult(map[string]any{
		"ok":          false,
		"error":       errText,
		"status":      statusSkipped,
		"reason_code": reasonCodeDeniedDependency,
	})
	if out != nil {
		runner.renderToolFeedback(out, call.Function.Name, result)
	}

	runner.emit(Event{
		Type:       EventToolCallCompleted,
		SessionID:  corepkg.SessionID(sess.ID),
		ToolName:   call.Function.Name,
		ToolResult: result,
		Error:      errText,
	})
	emitTurnEvent(ctx, TurnEvent{
		Type: TurnEventToolResult,
		Payload: map[string]any{
			"tool_name":    call.Function.Name,
			"tool_call_id": call.ID,
			"tool_result":  result,
			"error":        errText,
		},
	})

	toolMessage := llm.NewToolResultMessage(call.ID, result)
	if err := llm.ValidateMessage(toolMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, toolMessage)
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	return nil
}
