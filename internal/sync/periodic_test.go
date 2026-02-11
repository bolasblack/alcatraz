package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartPeriodicRefresh_StopReturnsCachedConflicts(t *testing.T) {
	mock := &mockSyncSessionClient{
		listSyncSessionsFn: func(namePrefix string) ([]string, error) {
			return []string{"alca-proj-sync"}, nil
		},
		listSessionJSONFn: func(sessionName string) ([]byte, error) {
			return []byte(`[{"conflicts":[{
				"root":"src/config.yaml",
				"alphaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}],
				"betaChanges":[{"path":"","old":{"kind":"file"},"new":{"kind":"file"}}]
			}]}]`), nil
		},
	}

	env := newTestSyncEnv(mock)
	stop := startPeriodicRefresh(context.Background(), env, "proj", "/project", 10*time.Millisecond)

	// Wait for at least one tick to complete
	time.Sleep(50 * time.Millisecond)

	conflicts := stop()
	if len(conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1", len(conflicts))
	}
	if conflicts[0].Path != "src/config.yaml" {
		t.Errorf("got path %q, want %q", conflicts[0].Path, "src/config.yaml")
	}
}

func TestStartPeriodicRefresh_StopReturnsNilWhenNoCache(t *testing.T) {
	mock := &mockSyncSessionClient{
		listSyncSessionsFn: func(namePrefix string) ([]string, error) {
			return nil, nil
		},
	}

	env := newTestSyncEnv(mock)
	stop := startPeriodicRefresh(context.Background(), env, "proj", "/project", 10*time.Millisecond)

	// Stop immediately — no tick has fired, no cache written
	conflicts := stop()
	if conflicts != nil {
		t.Fatalf("got %v, want nil", conflicts)
	}
}

func TestStartPeriodicRefresh_RefreshesOnTick(t *testing.T) {
	var calls atomic.Int32

	mock := &mockSyncSessionClient{
		listSyncSessionsFn: func(namePrefix string) ([]string, error) {
			calls.Add(1)
			return nil, nil
		},
	}

	env := newTestSyncEnv(mock)
	stop := startPeriodicRefresh(context.Background(), env, "proj", "/project", 10*time.Millisecond)

	// Wait for multiple ticks
	time.Sleep(80 * time.Millisecond)
	stop()

	got := calls.Load()
	if got < 2 {
		t.Errorf("expected at least 2 refresh calls, got %d", got)
	}
}
