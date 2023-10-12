// Package rwlock is an adapter package to pkg/rwlock.
// Consider using pkg/rwlock package in new projects as this file may be eventually removed.
package rwlock

import (
	"github.com/e-chip/redis-rwlock/pkg/rwlock"
	"github.com/redis/go-redis/v9"
)

// Locker is an alias type to #rwlock.Locker
type Locker = rwlock.Locker

// Options is an alias type to #rwlock.Options
type Options = rwlock.Options

// Make new instance of RW-Locker.
// Deprecated due to incorrect naming of the function.
// Use #rwlock.New instead.
func Make(redisClient *redis.Client, keyLock, keyReadersCount, keyWriterIntent string, opts *Options) Locker {
	return New(redisClient, keyLock, keyReadersCount, keyWriterIntent, opts)
}

// New instance of RW-Locker.
// Use #rwlock.New instead.
func New(redisClient *redis.Client, keyLock, keyReadersCount, keyWriterIntent string, opts *Options) Locker {
	if opts == nil {
		opts = &Options{}
	}
	return rwlock.New(redisClient, keyLock, keyReadersCount, keyWriterIntent, *opts)
}
