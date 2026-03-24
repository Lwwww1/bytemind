package main

import (
	"bytes"
	"strings"
	"testing"

	"aicoding/internal/session"
)

func TestCompleteSlashCommand(t *testing.T) {
	completed, suggestions := completeSlashCommand("/pla")
	if len(suggestions) != 0 {
		t.Fatalf("expected unique completion, got suggestions %#v", suggestions)
	}
	if completed != "/plan" {
		t.Fatalf("expected /plan, got %q", completed)
	}
}

func TestCompleteSlashCommandReturnsSuggestionsForAmbiguousPrefix(t *testing.T) {
	completed, suggestions := completeSlashCommand("/sess")
	if completed != "/sess" {
		t.Fatalf("expected input to remain unchanged, got %q", completed)
	}
	if len(suggestions) != 2 || suggestions[0] != "/session" || suggestions[1] != "/sessions" {
		t.Fatalf("unexpected suggestions: %#v", suggestions)
	}
}

func TestResolveSessionIDSupportsUniquePrefix(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := session.New(`E:\\repo`)
	first.ID = "20260324-120000-abcd"
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}

	second := session.New(`E:\\repo`)
	second.ID = "20260324-130000-efgh"
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveSessionID(store, "20260324-1300")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != second.ID {
		t.Fatalf("expected %q, got %q", second.ID, resolved)
	}
}

func TestHandleSlashCommandRejectsResumeAcrossWorkspaces(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	current := session.New(`E:\\repo-a`)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	other := session.New(`E:\\repo-b`)
	other.ID = "other"
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, shouldExit, handled, err := handleSlashCommand(&out, store, current, "/resume other")
	if err == nil {
		t.Fatal("expected cross-workspace resume to fail")
	}
	if !strings.Contains(err.Error(), "belongs to workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled || shouldExit {
		t.Fatalf("expected handled command without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if next != current {
		t.Fatal("expected current session to remain active")
	}
}

func TestHandleSlashCommandPlanCreateAndShow(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, shouldExit, handled, err := handleSlashCommand(&out, store, sess, "/plan create inspect repo | implement change | verify result")
	if err != nil {
		t.Fatal(err)
	}
	if shouldExit || !handled {
		t.Fatalf("expected handled command without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if len(next.Plan) != 3 {
		t.Fatalf("expected 3 plan steps, got %#v", next.Plan)
	}
	if next.Plan[0].Status != "in_progress" || next.Plan[1].Status != "pending" {
		t.Fatalf("unexpected plan statuses: %#v", next.Plan)
	}
	if !strings.Contains(out.String(), "plan") {
		t.Fatalf("expected command output, got %q", out.String())
	}
}

func TestHandleSlashCommandPlanAutoCreatesFromGoal(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	next, _, _, err := handleSlashCommand(&bytes.Buffer{}, store, sess, "/plan 做一个扫雷小游戏")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plan) < 3 {
		t.Fatalf("expected generated multi-step plan, got %#v", next.Plan)
	}
	if next.Plan[0].Status != "in_progress" {
		t.Fatalf("expected first generated step in progress, got %#v", next.Plan)
	}
	if !strings.Contains(next.Plan[0].Step, "页面") && !strings.Contains(next.Plan[0].Step, "棋盘") {
		t.Fatalf("unexpected generated steps: %#v", next.Plan)
	}
}

func TestHandleSlashCommandPlanCreateSingleGoalUsesAutoPlanner(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	next, _, _, err := handleSlashCommand(&bytes.Buffer{}, store, sess, "/plan create 做一个扫雷小游戏")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plan) < 3 {
		t.Fatalf("expected generated multi-step plan, got %#v", next.Plan)
	}
}

func TestGeneratePlanStepsForFallbackGoal(t *testing.T) {
	steps := generatePlanSteps("优化项目结构")
	if len(steps) < 3 {
		t.Fatalf("expected fallback multi-step plan, got %#v", steps)
	}
}

func TestHandleSlashCommandPlanStatusUpdates(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	sess.Plan = []session.PlanItem{
		{Step: "inspect", Status: "in_progress"},
		{Step: "implement", Status: "pending"},
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, _, _, err := handleSlashCommand(&out, store, sess, "/plan done 1")
	if err != nil {
		t.Fatal(err)
	}
	if next.Plan[0].Status != "completed" {
		t.Fatalf("expected first step completed, got %#v", next.Plan)
	}

	out.Reset()
	next, _, _, err = handleSlashCommand(&out, store, next, "/plan start 2")
	if err != nil {
		t.Fatal(err)
	}
	if next.Plan[1].Status != "in_progress" {
		t.Fatalf("expected second step in progress, got %#v", next.Plan)
	}
}

func TestHandleSlashCommandPlanClear(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	sess.Plan = []session.PlanItem{{Step: "inspect", Status: "in_progress"}}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	_, _, _, err = handleSlashCommand(&bytes.Buffer{}, store, sess, "/plan clear")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Plan) != 0 {
		t.Fatalf("expected plan to be cleared, got %#v", sess.Plan)
	}
}

func TestSameWorkspaceNormalizesPaths(t *testing.T) {
	if !sameWorkspace(`E:\\Repo`, `E:\\Repo\\.`) {
		t.Fatal("expected normalized paths to match")
	}
}
