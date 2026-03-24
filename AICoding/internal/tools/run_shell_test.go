package tools

import (
	"bytes"
	"strings"
	"testing"
)

func TestAssessShellCommandAllowsReadOnlyCommands(t *testing.T) {
	assessment := assessShellCommand("git status")
	if assessment.Risk != shellRiskSafe {
		t.Fatalf("expected safe command, got %#v", assessment)
	}
}

func TestAssessShellCommandRequiresApprovalForWriteRedirection(t *testing.T) {
	assessment := assessShellCommand("echo hi > out.txt")
	if assessment.Risk != shellRiskApproval {
		t.Fatalf("expected approval risk, got %#v", assessment)
	}
	if !strings.Contains(assessment.Reason, "redirection") {
		t.Fatalf("expected redirection reason, got %#v", assessment)
	}
}

func TestAssessShellCommandBlocksDangerousGitReset(t *testing.T) {
	assessment := assessShellCommand("git reset --hard HEAD~1")
	if assessment.Risk != shellRiskBlocked {
		t.Fatalf("expected blocked command, got %#v", assessment)
	}
}

func TestAssessShellCommandSplitsSegments(t *testing.T) {
	assessment := assessShellCommand("git status && go test ./...")
	if assessment.Risk != shellRiskApproval {
		t.Fatalf("expected approval risk from second segment, got %#v", assessment)
	}
}

func TestRequireApprovalOnRequestAllowsReadOnlyWithoutPrompt(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("git status", &ExecutionContext{
		ApprovalPolicy: "on-request",
		Stdin:          strings.NewReader(""),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt for safe command, got %q", out.String())
	}
}

func TestRequireApprovalOnRequestPromptsForRiskyCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "on-request",
		Stdin:          strings.NewReader("yes\n"),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Approve shell command") {
		t.Fatalf("expected approval prompt, got %q", out.String())
	}
}

func TestRequireApprovalAlwaysPromptsEvenForSafeCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("git status", &ExecutionContext{
		ApprovalPolicy: "always",
		Stdin:          strings.NewReader("y\n"),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Approve shell command") {
		t.Fatalf("expected prompt for always policy, got %q", out.String())
	}
}

func TestRequireApprovalNeverSkipsPromptForRiskyCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "never",
		Stdin:          strings.NewReader(""),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt for never policy, got %q", out.String())
	}
}

func TestRequireApprovalBlocksDangerousCommandRegardlessOfPolicy(t *testing.T) {
	err := requireApproval("rm -rf .", &ExecutionContext{ApprovalPolicy: "never"})
	if err == nil {
		t.Fatal("expected dangerous command to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked dangerous shell command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireApprovalNeedsStdinWhenPrompting(t *testing.T) {
	err := requireApproval("go test ./...", &ExecutionContext{ApprovalPolicy: "always"})
	if err == nil {
		t.Fatal("expected missing stdin error")
	}
	if !strings.Contains(err.Error(), "no stdin") {
		t.Fatalf("unexpected error: %v", err)
	}
}
