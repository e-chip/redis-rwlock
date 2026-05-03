package rwlock

import "context"

// RedisClient is the minimal interface required by the locker.
// Implement it with any Redis client library using the provided adapters or a custom wrapper.
type RedisClient interface {
	// Eval executes a Lua script atomically.
	// Returns 1 on success, 0 on failure (as defined by the script).
	Eval(ctx context.Context, script string, keys []string, args ...any) (int64, error)
}
