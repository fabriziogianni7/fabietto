package sessionlock

import (
	"sync"

	"custom-agent/session"
)

// locks holds a mutex per session key. Different sessions run in parallel;
// same session serializes.
var locks sync.Map

// Lock acquires a per-session lock. Call the returned function to release.
// Only one message processes at a time per (platform, userID).
// Returns a no-op unlock for invalid session keys.
func Lock(platform, userID string) (unlock func()) {
	key := session.SessionKey(platform, userID)
	if key == "" || key == "_" {
		return func() {}
	}
	mu, _ := locks.LoadOrStore(key, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return func() { m.Unlock() }
}
