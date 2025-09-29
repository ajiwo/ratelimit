package strategies

import (
	"sync"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// checkV1Header validates that the string starts with "v1|"
func checkV1Header(s string) bool {
	return len(s) >= 3 && s[0] == 'v' && s[1] == '1' && s[2] == '|'
}

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

// calcExpiration calculates an appropriate expiration time for storage operations
// based on capacity and rate, with a minimum of 1 second
func calcExpiration(capacity int, rate float64) time.Duration {
	expirationSeconds := float64(capacity) / rate * 2
	if expirationSeconds < 1 {
		expirationSeconds = 1
	}
	return time.Duration(expirationSeconds) * time.Second
}
