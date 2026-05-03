package rwlock

import "time"

// Options configures the Locker. All zero values are replaced with safe defaults.
type Options struct {
	// LockTTL sets lock expiry duration.
	// Automatically refreshed every LockTTL/2 while held.
	// Should be less than RetryCount * RetryInterval to avoid spurious ErrTimeout.
	// Minimum: 100 milliseconds. Default: 1 second.
	LockTTL time.Duration

	// RetryCount limits the number of lock acquisition attempts before returning ErrTimeout.
	// Default: 200.
	RetryCount int

	// RetryInterval is the pause between lock acquisition attempts.
	// Minimum: 1 millisecond. Default: 10 milliseconds.
	RetryInterval time.Duration

	// AppID is prepended to the writer token (e.g. "myservice").
	// Useful when debugging which process holds the lock in Redis.
	AppID string

	// ReaderLockToken is the value stored in the lock key while any reader holds it.
	// Must be identical across all members of a reader group.
	// Override to create independent reader groups on the same key prefix.
	ReaderLockToken string
}

func prepareOpts(opts *Options) {
	const (
		ttlMin = 100 * time.Millisecond
		ttlDef = time.Second

		retryCountMin = 1
		retryCountDef = 200

		retryIntervalMin = time.Millisecond
		retryIntervalDef = 10 * time.Millisecond

		readerTokenDef = "read_c2d-75a1-4b5b-a6fb-b0754224c666"
	)

	switch {
	case opts.LockTTL == 0:
		opts.LockTTL = ttlDef
	case opts.LockTTL < ttlMin:
		opts.LockTTL = ttlMin
	}

	if opts.RetryCount == 0 {
		opts.RetryCount = retryCountDef
	} else if opts.RetryCount < retryCountMin {
		opts.RetryCount = retryCountMin
	}

	switch {
	case opts.RetryInterval == 0:
		opts.RetryInterval = retryIntervalDef
	case opts.RetryInterval < retryIntervalMin:
		opts.RetryInterval = retryIntervalMin
	}

	if opts.ReaderLockToken == "" {
		opts.ReaderLockToken = readerTokenDef
	}
}
