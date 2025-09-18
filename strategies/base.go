package strategies

import (
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// BaseStrategy provides common functionality for all rate limiting strategies
type BaseStrategy struct {
	storage backends.Backend
	mu      sync.Map // per-key locks
}

// getLock returns a mutex for the given key, tracking usage time
func (b *BaseStrategy) getLock(key string) *sync.Mutex {
	actual, _ := b.mu.LoadOrStore(key, &LockInfo{
		mutex:    &sync.Mutex{},
		lastUsed: time.Now(),
	})

	lockInfo := actual.(*LockInfo)
	lockInfo.mu.Lock()
	lockInfo.lastUsed = time.Now()
	lockInfo.mu.Unlock()
	return lockInfo.mutex
}

// CleanupLocks removes locks that haven't been used recently
func (b *BaseStrategy) CleanupLocks(maxAge time.Duration) {
	b.mu.Range(func(key, value any) bool {
		lockInfo := value.(*LockInfo)
		lockInfo.mu.Lock()
		lastUsed := lockInfo.lastUsed
		lockInfo.mu.Unlock()

		if time.Since(lastUsed) > maxAge {
			b.mu.Delete(key)
		}
		return true
	})
}
