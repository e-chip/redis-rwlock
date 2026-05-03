package rwlock

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type lockerImpl struct {
	redisClient RedisClient
	options     Options

	keyLock    string
	keyCounter string

	writerToken string
	lockTTL     string
}

func (l *lockerImpl) Read(ctx context.Context, fn func(context.Context) error) error {
	return l.do(ctx, fn, l.acquireReader, l.refreshReader, l.releaseReader)
}

func (l *lockerImpl) Write(ctx context.Context, fn func(context.Context) error) error {
	return l.do(ctx, fn, l.acquireWriter, l.refreshWriter, l.releaseWriter)
}

func (l *lockerImpl) do(
	ctx context.Context,
	fn func(context.Context) error,
	acquire, refresh, release func(context.Context) (bool, error),
) error {
	acquired, err := l.execute(ctx, acquire)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrTimeout
	}

	// Buffered so the send never blocks, even if keepRefreshing already exited via ctx.Done().
	stopRefreshing := make(chan struct{}, 1)
	go l.keepRefreshing(ctx, refresh, stopRefreshing)

	fnErr := l.runFn(ctx, fn)
	stopRefreshing <- struct{}{}

	// Release with a fresh context: the caller's ctx may already be cancelled,
	// but the lock must always be released.
	releaseCtx, cancel := context.WithTimeout(context.Background(), l.options.LockTTL)
	defer cancel()
	released, releaseErr := release(releaseCtx)

	switch {
	case fnErr != nil && releaseErr != nil:
		return errors.Join(fnErr, releaseErr)
	case fnErr != nil:
		return fnErr
	case releaseErr != nil:
		return releaseErr
	case !released:
		return ErrNotReleased
	default:
		return nil
	}
}

func (l *lockerImpl) runFn(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch t := r.(type) {
			case error:
				err = t
			case string:
				err = errors.New(t)
			default:
				err = fmt.Errorf("unknown panic: %v", t)
			}
		}
	}()
	return fn(ctx)
}

func (l *lockerImpl) execute(ctx context.Context, fn func(context.Context) (bool, error)) (bool, error) {
	for i := range l.options.RetryCount {
		if ok, err := fn(ctx); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		// No wait after the last attempt.
		if i < l.options.RetryCount-1 {
			if err := l.wait(ctx, l.options.RetryInterval); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func (l *lockerImpl) wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ErrInterrupted
	case <-timer.C:
		return nil
	}
}

func (l *lockerImpl) keepRefreshing(ctx context.Context, refresh func(context.Context) (bool, error), stop <-chan struct{}) {
	ticker := time.NewTicker(l.options.LockTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh(ctx) //nolint:errcheck
		}
	}
}

func (l *lockerImpl) acquireReader(ctx context.Context) (bool, error) {
	return l.execScript(ctx, readLockScript, []string{
		l.keyLock,
		l.keyCounter,
	}, l.options.ReaderLockToken, l.lockTTL)
}

func (l *lockerImpl) releaseReader(ctx context.Context) (bool, error) {
	return l.execScript(ctx, readUnlockScript, []string{
		l.keyLock,
		l.keyCounter,
	}, l.options.ReaderLockToken)
}

func (l *lockerImpl) refreshReader(ctx context.Context) (bool, error) {
	return l.execScript(ctx, lockRefreshScript, []string{
		l.keyLock,
	}, l.options.ReaderLockToken, l.lockTTL)
}

func (l *lockerImpl) acquireWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, writeLockScript, []string{
		l.keyLock,
		l.keyCounter,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) releaseWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, writeUnlockScript, []string{
		l.keyLock,
	}, l.writerToken)
}

func (l *lockerImpl) refreshWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, lockRefreshScript, []string{
		l.keyLock,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) execScript(ctx context.Context, script string, keys []string, args ...any) (bool, error) {
	status, err := l.redisClient.Eval(ctx, script, keys, args...)
	if err != nil {
		return false, err
	}
	return status == 1, nil
}
