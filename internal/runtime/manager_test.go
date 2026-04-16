package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestInMemoryTaskManagerSubmitAndCancel(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected task id")
	}
	if err := mgr.Cancel(context.Background(), id, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	task, err := mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != "killed" {
		t.Fatalf("expected killed status, got %s", task.Status)
	}
	if task.ErrorCode != ErrorCodeTaskCancelled {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskCancelled, task.ErrorCode)
	}
}

func TestInMemoryTaskManagerWaitReturnsTerminalResult(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	results := make(chan TaskResult, 1)
	waitErrs := make(chan error, 1)
	go func() {
		result, err := mgr.Wait(context.Background(), id)
		if err != nil {
			waitErrs <- err
			return
		}
		results <- result
	}()

	time.Sleep(10 * time.Millisecond)
	if err := mgr.Cancel(context.Background(), id, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	select {
	case err := <-waitErrs:
		t.Fatalf("Wait failed: %v", err)
	case result := <-results:
		if result.TaskID != id {
			t.Fatalf("expected task id %q, got %q", id, result.TaskID)
		}
		if result.Status != "killed" {
			t.Fatalf("expected killed status, got %s", result.Status)
		}
		if result.ErrorCode != ErrorCodeTaskCancelled {
			t.Fatalf("expected error code %q, got %q", ErrorCodeTaskCancelled, result.ErrorCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait timed out")
	}
}

func TestInMemoryTaskManagerWaitRespectsContextCancellation(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = mgr.Wait(ctx, id)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestInMemoryTaskManagerGetUnknownTaskReturnsTaskNotFound(t *testing.T) {
	mgr := NewInMemoryTaskManager()

	_, err := mgr.Get(context.Background(), "unknown-task")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
	if !hasErrorCode(err, ErrorCodeTaskNotFound) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskNotFound, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryFromFailedResetsTaskForRetry(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:       "demo",
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	startedAt := time.Now().UTC().Add(-2 * time.Second)
	finishedAt := time.Now().UTC().Add(-1 * time.Second)
	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskFailed
	task.Attempt = 1
	task.StartedAt = &startedAt
	task.FinishedAt = &finishedAt
	task.ErrorCode = ErrorCodeTaskTimeout
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	retriedID, err := mgr.Retry(context.Background(), id)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}
	if retriedID != id {
		t.Fatalf("expected retried id %q, got %q", id, retriedID)
	}

	task, err = mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != corepkg.TaskPending {
		t.Fatalf("expected pending status, got %s", task.Status)
	}
	if task.Attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", task.Attempt)
	}
	if task.ErrorCode != "" {
		t.Fatalf("expected cleared error code, got %q", task.ErrorCode)
	}
	if task.StartedAt != nil {
		t.Fatal("expected startedAt to reset on retry")
	}
	if task.FinishedAt != nil {
		t.Fatal("expected finishedAt to reset on retry")
	}
}

func TestInMemoryTaskManagerRetryRejectsNonFailedTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	_, err = mgr.Retry(context.Background(), id)
	if err == nil {
		t.Fatal("expected retry error for non-failed task")
	}
	if !hasErrorCode(err, ErrorCodeInvalidTransition) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeInvalidTransition, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryExhausted(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:       "demo",
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskFailed
	task.Attempt = 1
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	_, err = mgr.Retry(context.Background(), id)
	if err == nil {
		t.Fatal("expected retry exhausted error")
	}
	if !hasErrorCode(err, ErrorCodeRetryExhausted) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeRetryExhausted, errorCode(err))
	}
}

func TestInMemoryTaskManagerRetryUnknownTaskReturnsTaskNotFound(t *testing.T) {
	mgr := NewInMemoryTaskManager()

	_, err := mgr.Retry(context.Background(), "missing-task")
	if err == nil {
		t.Fatal("expected retry error for unknown task")
	}
	if !hasErrorCode(err, ErrorCodeTaskNotFound) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskNotFound, errorCode(err))
	}
}

func TestInMemoryTaskManagerCancelIsIdempotent(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	if err := mgr.Cancel(context.Background(), id, "first cancel"); err != nil {
		t.Fatalf("first cancel failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "second cancel"); err != nil {
		t.Fatalf("second cancel should be idempotent, got: %v", err)
	}
}

func TestInMemoryTaskManagerCancelRejectsCompletedTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	finishedAt := time.Now().UTC()
	mgr.mu.Lock()
	task := mgr.tasks[id]
	task.Status = corepkg.TaskCompleted
	task.FinishedAt = &finishedAt
	mgr.tasks[id] = task
	mgr.mu.Unlock()

	err = mgr.Cancel(context.Background(), id, "cancel completed")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if !hasErrorCode(err, ErrorCodeInvalidTransition) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeInvalidTransition, errorCode(err))
	}
}

func TestInMemoryTaskManagerWaitReturnsImmediatelyForTerminalTask(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "terminal"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := mgr.Wait(ctx, id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskKilled {
		t.Fatalf("expected killed status, got %s", result.Status)
	}
}

func TestInMemoryTaskManagerWaitWithNilContextUsesBackground(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "demo"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "terminal"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	result, err := mgr.Wait(nil, id)
	if err != nil {
		t.Fatalf("Wait with nil context failed: %v", err)
	}
	if result.TaskID != id {
		t.Fatalf("expected task id %q, got %q", id, result.TaskID)
	}
}

func TestInMemoryTaskManagerStreamReplaysHistoryAndLiveUpdates(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "stream"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := mgr.Stream(ctx, id)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	first := mustReceiveEvent(t, stream)
	if first.Status != corepkg.TaskPending {
		t.Fatalf("expected first status pending, got %s", first.Status)
	}
	if first.Offset != 0 {
		t.Fatalf("expected first offset 0, got %d", first.Offset)
	}

	if err := mgr.Cancel(context.Background(), id, "cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	second := mustReceiveEvent(t, stream)
	if second.Status != corepkg.TaskKilled {
		t.Fatalf("expected second status killed, got %s", second.Status)
	}
	if second.Offset != 1 {
		t.Fatalf("expected second offset 1, got %d", second.Offset)
	}
	mustCloseStream(t, stream)
}

func TestInMemoryTaskManagerStreamReplaysTerminalHistoryAndCloses(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "terminal-history"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	stream, err := mgr.Stream(context.Background(), id)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	first := mustReceiveEvent(t, stream)
	second := mustReceiveEvent(t, stream)
	if first.Status != corepkg.TaskPending || second.Status != corepkg.TaskKilled {
		t.Fatalf("expected replayed statuses pending->killed, got %s->%s", first.Status, second.Status)
	}
	mustCloseStream(t, stream)
}

func TestInMemoryTaskManagerReadIncrementSupportsOffsetWindowAndBounds(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "logs"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	items, next, hasMore, err := mgr.ReadIncrement(context.Background(), id, 0, 1)
	if err != nil {
		t.Fatalf("ReadIncrement failed: %v", err)
	}
	if len(items) != 1 || next != 1 || !hasMore {
		t.Fatalf("expected len=1 next=1 hasMore=true, got len=%d next=%d hasMore=%v", len(items), next, hasMore)
	}
	if string(items[0].Payload) != "status=pending" {
		t.Fatalf("expected first log payload %q, got %q", "status=pending", string(items[0].Payload))
	}

	items, next, hasMore, err = mgr.ReadIncrement(context.Background(), id, next, 10)
	if err != nil {
		t.Fatalf("ReadIncrement second page failed: %v", err)
	}
	if len(items) != 1 || next != 2 || hasMore {
		t.Fatalf("expected len=1 next=2 hasMore=false, got len=%d next=%d hasMore=%v", len(items), next, hasMore)
	}
	if string(items[0].Payload) != "status=killed" {
		t.Fatalf("expected second log payload %q, got %q", "status=killed", string(items[0].Payload))
	}

	items, next, hasMore, err = mgr.ReadIncrement(context.Background(), id, 0, 0)
	if err != nil {
		t.Fatalf("ReadIncrement default limit failed: %v", err)
	}
	if len(items) != 2 || next != 2 || hasMore {
		t.Fatalf("expected len=2 next=2 hasMore=false, got len=%d next=%d hasMore=%v", len(items), next, hasMore)
	}

	items, next, hasMore, err = mgr.ReadIncrement(context.Background(), id, 100, 10)
	if err != nil {
		t.Fatalf("ReadIncrement large offset failed: %v", err)
	}
	if len(items) != 0 || next != 2 || hasMore {
		t.Fatalf("expected len=0 next=2 hasMore=false, got len=%d next=%d hasMore=%v", len(items), next, hasMore)
	}
}

func TestInMemoryTaskManagerReadIncrementUnknownTaskReturnsTaskNotFound(t *testing.T) {
	mgr := NewInMemoryTaskManager()

	_, _, _, err := mgr.ReadIncrement(context.Background(), "missing-task", 0, 10)
	if err == nil {
		t.Fatal("expected task not found error")
	}
	if !hasErrorCode(err, ErrorCodeTaskNotFound) {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskNotFound, errorCode(err))
	}
}

func TestInMemoryTaskManagerBridgesEventAndLogToStore(t *testing.T) {
	store := &captureTaskEventStore{}
	mgr := NewInMemoryTaskManager(WithTaskEventStore(store))
	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "bridge"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := mgr.Cancel(context.Background(), id, "cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.events) != 2 {
		t.Fatalf("expected 2 bridged events, got %d", len(store.events))
	}
	if len(store.logs) != 2 {
		t.Fatalf("expected 2 bridged logs, got %d", len(store.logs))
	}
	if store.events[0].Offset != 0 || store.events[1].Offset != 1 {
		t.Fatalf("expected bridged event offsets [0,1], got [%d,%d]", store.events[0].Offset, store.events[1].Offset)
	}
}

func TestInMemoryTaskManagerSubmitRunsExecutorAndCompletes(t *testing.T) {
	mgr := NewInMemoryTaskManager(WithTaskExecutor(func(_ context.Context, task Task) ([]byte, error) {
		if task.Status != corepkg.TaskRunning {
			t.Fatalf("expected task status running in executor, got %s", task.Status)
		}
		return []byte("done"), nil
	}))

	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "execute"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := mgr.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if string(result.Output) != "done" {
		t.Fatalf("expected output %q, got %q", "done", string(result.Output))
	}
}

func TestInMemoryTaskManagerTimeoutMapsToTaskTimeout(t *testing.T) {
	mgr := NewInMemoryTaskManager(WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))

	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:    "timeout",
		Timeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := mgr.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskFailed {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
	if result.ErrorCode != ErrorCodeTaskTimeout {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskTimeout, result.ErrorCode)
	}
}

func TestInMemoryTaskManagerCancelPropagatesToRunningTask(t *testing.T) {
	started := make(chan struct{}, 1)
	mgr := NewInMemoryTaskManager(WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}))

	id, err := mgr.Submit(context.Background(), TaskSpec{Name: "cancel"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not start")
	}

	if err := mgr.Cancel(context.Background(), id, "test cancel"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	task, err := mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if task.Status != corepkg.TaskKilled {
		t.Fatalf("expected killed status, got %s", task.Status)
	}
	if task.ErrorCode != ErrorCodeTaskCancelled {
		t.Fatalf("expected error code %q, got %q", ErrorCodeTaskCancelled, task.ErrorCode)
	}
}

func TestInMemoryTaskManagerRetrySchedulesNewAttempt(t *testing.T) {
	var runs atomic.Int32
	mgr := NewInMemoryTaskManager(WithTaskExecutor(func(_ context.Context, _ Task) ([]byte, error) {
		if runs.Add(1) == 1 {
			return nil, errors.New("first attempt failed")
		}
		return []byte("recovered"), nil
	}))

	id, err := mgr.Submit(context.Background(), TaskSpec{
		Name:       "retry",
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	waitCtx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()
	first, err := mgr.Wait(waitCtx1, id)
	if err != nil {
		t.Fatalf("Wait first attempt failed: %v", err)
	}
	if first.Status != corepkg.TaskFailed {
		t.Fatalf("expected first attempt failed, got %s", first.Status)
	}
	if first.ErrorCode != ErrorCodeTaskExecutionFailed {
		t.Fatalf("expected first attempt error code %q, got %q", ErrorCodeTaskExecutionFailed, first.ErrorCode)
	}

	_, err = mgr.Retry(context.Background(), id)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	waitCtx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	second, err := mgr.Wait(waitCtx2, id)
	if err != nil {
		t.Fatalf("Wait second attempt failed: %v", err)
	}
	if second.Status != corepkg.TaskCompleted {
		t.Fatalf("expected second attempt completed, got %s", second.Status)
	}
	if string(second.Output) != "recovered" {
		t.Fatalf("expected output %q, got %q", "recovered", string(second.Output))
	}
}

func TestInMemoryTaskManagerSubmitDetachesCallerContext(t *testing.T) {
	mgr := NewInMemoryTaskManager(WithTaskExecutor(func(ctx context.Context, _ Task) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Millisecond):
			return []byte("done"), nil
		}
	}))

	submitCtx, cancel := context.WithCancel(context.Background())
	id, err := mgr.Submit(submitCtx, TaskSpec{Name: "detached"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	cancel()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	result, err := mgr.Wait(waitCtx, id)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if result.Status != corepkg.TaskCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if result.ErrorCode != "" {
		t.Fatalf("expected empty error code, got %q", result.ErrorCode)
	}
}

func TestInMemoryTaskManagerSubmitSnapshotsMutableTaskSpec(t *testing.T) {
	mgr := NewInMemoryTaskManager()
	spec := TaskSpec{
		Name:     "snapshot",
		Input:    []byte("immutable"),
		Metadata: map[string]string{"owner": "runtime"},
	}

	id, err := mgr.Submit(context.Background(), spec)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	spec.Input[0] = 'X'
	spec.Metadata["owner"] = "mutated"
	spec.Metadata["extra"] = "value"

	task, err := mgr.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(task.Spec.Input) != "immutable" {
		t.Fatalf("expected task input snapshot %q, got %q", "immutable", string(task.Spec.Input))
	}
	if got := task.Spec.Metadata["owner"]; got != "runtime" {
		t.Fatalf("expected metadata owner %q, got %q", "runtime", got)
	}
	if _, ok := task.Spec.Metadata["extra"]; ok {
		t.Fatal("expected metadata not to include caller-side mutations")
	}
}

type captureTaskEventStore struct {
	mu     sync.Mutex
	events []TaskEvent
	logs   []TaskLogEntry
}

func (s *captureTaskEventStore) AppendTaskEvent(_ context.Context, event TaskEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneTaskEvent(event))
	return nil
}

func (s *captureTaskEventStore) AppendTaskLog(_ context.Context, _ corepkg.TaskID, entry TaskLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, cloneTaskLogEntry(entry))
	return nil
}

func mustReceiveEvent(t *testing.T, stream <-chan TaskEvent) TaskEvent {
	t.Helper()
	select {
	case event, ok := <-stream:
		if !ok {
			t.Fatal("expected event, stream is closed")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream event")
		return TaskEvent{}
	}
}

func mustCloseStream(t *testing.T, stream <-chan TaskEvent) {
	t.Helper()
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("expected stream to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream close")
	}
}
