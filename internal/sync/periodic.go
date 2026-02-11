package sync

import (
	"context"
	"sync"
	"time"
)

const (
	// PeriodicRefreshInterval is the default interval between cache refreshes.
	PeriodicRefreshInterval = 30 * time.Second
	// RefreshTimeout is the maximum time to wait for a single refresh.
	RefreshTimeout = 10 * time.Second
)

// StartPeriodicRefresh starts a background goroutine that silently refreshes
// the sync conflict cache at regular intervals. Returns a stop function that
// stops the ticker and returns the latest cached conflicts.
func StartPeriodicRefresh(ctx context.Context, env *SyncEnv, projectID, projectRoot string) (stop func() []ConflictInfo) {
	return startPeriodicRefresh(ctx, env, projectID, projectRoot, PeriodicRefreshInterval)
}

func startPeriodicRefresh(ctx context.Context, env *SyncEnv, projectID, projectRoot string, interval time.Duration) (stop func() []ConflictInfo) {
	done := make(chan struct{})
	ticker := time.NewTicker(interval)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				tickCtx, cancel := context.WithTimeout(ctx, RefreshTimeout)
				_, _ = SyncUpdateCache(tickCtx, env, projectID, projectRoot)
				cancel()
			}
		}
	}()

	return func() []ConflictInfo {
		close(done)
		wg.Wait()
		cache, err := ReadCache(env.Fs, projectRoot)
		if err != nil || cache == nil {
			return nil
		}
		return cache.Conflicts
	}
}
