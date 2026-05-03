package rwlock

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	// ErrTimeout is returned when the lock cannot be acquired within RetryCount attempts.
	ErrTimeout = errors.New("timeout exceeded but lock not acquired")
	// ErrInterrupted is returned when the context is cancelled while waiting for the lock.
	ErrInterrupted = errors.New("interrupted")
	// ErrNotReleased is returned when the lock was acquired but could not be released.
	ErrNotReleased = errors.New("lock was not released")
)

// Locker provides distributed read-write locking backed by Redis.
type Locker interface {
	// Read executes fn with shared reader access.
	// Multiple readers may hold the lock concurrently.
	Read(ctx context.Context, fn func(ctx context.Context) error) error

	// Write executes fn with exclusive writer access.
	// No other reader or writer may hold the lock concurrently.
	Write(ctx context.Context, fn func(ctx context.Context) error) error
}

// New returns a Locker backed by two Redis keys derived from keyPrefix.
// keyPrefix must be non-empty and unique per logical lock.
func New(client RedisClient, keyPrefix string, opts Options) (Locker, error) {
	if keyPrefix == "" {
		return nil, errors.New("rwlock: keyPrefix must not be empty")
	}
	prepareOpts(&opts)
	return &lockerImpl{
		redisClient: client,
		options:     opts,
		keyLock:     keyPrefix + ":lock",
		keyCounter:  keyPrefix + ":counter",
		writerToken: makeToken(opts.AppID),
		lockTTL:     strconv.FormatInt(int64(opts.LockTTL/time.Millisecond), 10),
	}, nil
}

func makeToken(appID string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	token := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	if appID != "" {
		return appID + "_" + token
	}
	return token
}
