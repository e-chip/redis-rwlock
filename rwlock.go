// Package rwlock is an adapter package to pkg/rwlock.
// Consider using pkg/rwlock package in new projects as this file may be eventually removed.
package rwlock

import (
	"context"

	goredis "github.com/go-redis/redis"

	"github.com/e-chip/redis-rwlock/pkg/rwlock"
)

// Locker is an alias type to #rwlock.Locker
type Locker = rwlock.Locker

// Options is an alias type to #rwlock.Options
type Options = rwlock.Options

// Make new instance of RW-Locker.
// Deprecated due to incorrect naming of the function.
// Use #rwlock.New instead.
func Make(redisClient *goredis.Client, keyLock, keyReadersCount, keyWriterIntent string, opts *Options) Locker {
	return New(redisClient, keyLock, keyReadersCount, keyWriterIntent, opts)
}

// New instance of RW-Locker.
// Use #rwlock.New instead.
func New(redisClient *goredis.Client, keyLock, keyReadersCount, keyWriterIntent string, opts *Options) Locker {
	if opts == nil {
		opts = &Options{}
	}
	return rwlock.New(&v6Client{c: redisClient}, keyLock, keyReadersCount, keyWriterIntent, *opts)
}

// v6Client wraps a go-redis v6 *redis.Client to satisfy rwlock.RedisClient.
type v6Client struct {
	c *goredis.Client
}

func (cl *v6Client) Ping(_ context.Context) error {
	return cl.c.Ping().Err()
}

func (cl *v6Client) Eval(_ context.Context, script string, keys []string, args ...any) (int64, error) {
	res, err := cl.c.Eval(script, keys, args...).Result()
	if err != nil {
		return 0, err
	}
	v, ok := res.(int64)
	if !ok {
		return 0, nil
	}
	return v, nil
}
