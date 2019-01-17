package rwlock

import (
	"context"
	"time"
)

// Options used to configure locker.
type Options struct {
	// LockTTL sets lock duration timeout.
	// This allows to release the lock even if whole program crashes.
	// Recommended not to make it too big or too small as it may affect performance.
	// It should be less than RetryCount * RetryInterval in order to avoid undesired ErrTimeouts.
	// Minimum: 100 milliseconds
	// Default: 1 second
	LockTTL time.Duration

	// RetryCount limits number of attempts to acquire lock.
	// Default: 200
	RetryCount int

	// RetryInterval sets interval between attemts to acquire lock.
	// Minimum: 1 millisecond
	// Default: 10 milliseconds
	RetryInterval time.Duration

	// Context of the execution.
	// Default: Background
	Context context.Context

	// AppID is used as writer lock token prefix.
	// Used for debugging.
	AppID string

	// ReaderLockToken should be the same for all readers group.
	// You can override default token here to create subgroups of readers.
	ReaderLockToken string
}

func prepareOpts(opts *Options) {
	const (
		ttlMin           = 100 * time.Millisecond
		retryCountMin    = 1
		retryIntervalMin = time.Millisecond

		ttlDef           = time.Second
		retryCountDef    = 200
		retryIntervalDef = 10 * time.Millisecond
		readerTokenDef   = "read_c2d-75a1-4b5b-a6fb-b0754224c666"
	)

	if opts.LockTTL == 0 {
		opts.LockTTL = ttlDef
	} else if opts.LockTTL < ttlMin {
		opts.LockTTL = ttlMin
	}

	if opts.RetryCount == 0 {
		opts.RetryCount = retryCountDef
	}
	if opts.RetryCount < retryCountMin {
		opts.RetryCount = retryCountMin
	}

	if opts.RetryInterval == 0 {
		opts.RetryInterval = retryIntervalDef
	}
	if opts.RetryInterval < retryIntervalMin {
		opts.RetryInterval = retryIntervalMin
	}

	if opts.Context == nil {
		opts.Context = context.Background()
	}

	if len(opts.ReaderLockToken) == 0 {
		opts.ReaderLockToken = readerTokenDef
	}
}
