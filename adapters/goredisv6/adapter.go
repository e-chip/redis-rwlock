package goredisv6

import (
	"context"
	"sync"

	goredis "github.com/go-redis/redis"

	rwlock "github.com/e-chip/redis-rwlock/v2"
)

// Client wraps a go-redis v6 *redis.Client to implement rwlock.RedisClient.
// Scripts are cached by SHA to use EVALSHA with fallback to EVAL.
type Client struct {
	c       *goredis.Client
	scripts sync.Map // map[string]*goredis.Script keyed by script content
}

// New returns a rwlock.RedisClient backed by a go-redis v6 client.
func New(c *goredis.Client) rwlock.RedisClient {
	return &Client{c: c}
}

func (cl *Client) Eval(_ context.Context, script string, keys []string, args ...any) (int64, error) {
	s := cl.loadOrStoreScript(script)
	res, err := s.Run(cl.c, keys, args...).Result()
	if err != nil {
		return 0, err
	}
	v, ok := res.(int64)
	if !ok {
		return 0, nil
	}
	return v, nil
}

func (cl *Client) loadOrStoreScript(script string) *goredis.Script {
	if v, ok := cl.scripts.Load(script); ok {
		return v.(*goredis.Script)
	}
	s := goredis.NewScript(script)
	v, _ := cl.scripts.LoadOrStore(script, s)
	return v.(*goredis.Script)
}
