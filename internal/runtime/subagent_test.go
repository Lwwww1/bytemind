package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestInMemorySubAgentCoordinatorSpawnAndWaitSync(t *testing.T) {
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(_ context.Context, task Task) ([]byte, error) {
			return []byte("child:" + task.Spec.Name), nil
		}),
	)
	quota := NewInMemoryQuotaManager(1, nil)
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	taskID, err := coordinator.Spawn(context.Background(), TaskSpec{
		SessionID: "sess-1",
		Name:      "demo-child",
		Metadata: map[string]string{
			"quota_key": "subagent",
		},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	result, err := coordinator.Wait(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if string(result.Output) != "child:demo-child" {
		t.Fatalf("expected output %q, got %q", "child:demo-child", string(result.Output))
	}
}

func TestInMemorySubAgentCoordinatorQuotaExceededDoesNotSubmitTask(t *testing.T) {
	blocker := make(chan struct{})
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
			select {
			case <-blocker:
				return []byte("released"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	quota := NewInMemoryQuotaManager(1, map[string]int{"shared": 1})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	firstID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "first",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}

	// Ensure the first task has started and holds quota.
	waitUntilTaskStatus(t, manager, firstID, corepkg.TaskRunning, 2*time.Second)

	_, err = coordinator.Spawn(context.Background(), TaskSpec{
		Name: "second",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err == nil {
		t.Fatal("expected quota exceeded error for second spawn")
	}
	if !hasErrorCode(err, ErrorCodeQuotaExceeded) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeQuotaExceeded, errorCode(err))
	}

	manager.mu.RLock()
	taskCount := len(manager.tasks)
	manager.mu.RUnlock()
	if taskCount != 1 {
		t.Fatalf("expected only one submitted task, got %d", taskCount)
	}

	close(blocker)
	_, err = coordinator.Wait(context.Background(), firstID)
	if err != nil {
		t.Fatalf("Wait first task failed: %v", err)
	}
}

func TestInMemoryTaskManagerParentCancelPropagatesToChild(t *testing.T) {
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, task Task) ([]byte, error) {
			_ = task
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	)

	parentID, err := manager.Submit(context.Background(), TaskSpec{
		Name: "parent",
	})
	if err != nil {
		t.Fatalf("Submit parent failed: %v", err)
	}
	childID, err := manager.Submit(context.Background(), TaskSpec{
		Name:         "child",
		ParentTaskID: parentID,
	})
	if err != nil {
		t.Fatalf("Submit child failed: %v", err)
	}

	waitUntilTaskStatus(t, manager, parentID, corepkg.TaskRunning, 2*time.Second)
	waitUntilTaskStatus(t, manager, childID, corepkg.TaskRunning, 2*time.Second)

	if err := manager.Cancel(context.Background(), parentID, "cancel parent"); err != nil {
		t.Fatalf("Cancel parent failed: %v", err)
	}

	parentResult, err := manager.Wait(context.Background(), parentID)
	if err != nil {
		t.Fatalf("Wait parent failed: %v", err)
	}
	childResult, err := manager.Wait(context.Background(), childID)
	if err != nil {
		t.Fatalf("Wait child failed: %v", err)
	}

	if parentResult.Status != corepkg.TaskKilled {
		t.Fatalf("expected parent killed, got %s", parentResult.Status)
	}
	if childResult.Status != corepkg.TaskKilled {
		t.Fatalf("expected child killed, got %s", childResult.Status)
	}
	if childResult.ErrorCode != ErrorCodeTaskCancelled {
		t.Fatalf("expected child cancel error code %q, got %q", ErrorCodeTaskCancelled, childResult.ErrorCode)
	}
}

func TestInMemorySubAgentCoordinatorWaitContextTimeoutDoesNotReleaseQuotaBeforeTerminal(t *testing.T) {
	blocker := make(chan struct{})
	manager := NewInMemoryTaskManager(
		WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
			select {
			case <-blocker:
				return []byte("done"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	quota := NewInMemoryQuotaManager(1, map[string]int{"shared": 1})
	coordinator := NewInMemorySubAgentCoordinator(manager, quota)

	firstID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "first",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}
	waitUntilTaskStatus(t, manager, firstID, corepkg.TaskRunning, 2*time.Second)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer waitCancel()
	_, err = coordinator.Wait(waitCtx, firstID)
	if err == nil {
		t.Fatal("expected wait to fail with deadline exceeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	_, err = coordinator.Spawn(context.Background(), TaskSpec{
		Name: "second-before-terminal",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err == nil {
		t.Fatal("expected quota exceeded before first task reaches terminal state")
	}
	if !hasErrorCode(err, ErrorCodeQuotaExceeded) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeQuotaExceeded, errorCode(err))
	}

	close(blocker)
	result, err := coordinator.Wait(context.Background(), firstID)
	if err != nil {
		t.Fatalf("Wait first task failed: %v", err)
	}
	if result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected first task completed, got %s", result.Status)
	}

	secondID, err := coordinator.Spawn(context.Background(), TaskSpec{
		Name: "second-after-terminal",
		Metadata: map[string]string{
			"quota_key": "shared",
		},
	})
	if err != nil {
		t.Fatalf("second Spawn failed after terminal release: %v", err)
	}
	_, err = coordinator.Wait(context.Background(), secondID)
	if err != nil {
		t.Fatalf("Wait second task failed: %v", err)
	}
}

func waitUntilTaskStatus(t *testing.T, manager *InMemoryTaskManager, id corepkg.TaskID, expected corepkg.TaskStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		task, err := manager.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get task %q failed: %v", id, err)
		}
		if task.Status == expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %q did not reach status %s (current=%s)", id, expected, task.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
