package rwlock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type lockerImpl struct {
	redisClient *redis.Client
	options     Options

	keyGlobalLock   string
	keyReadersCount string
	keyWriterIntent string

	writerToken string
	lockTTL     string
}

func (l *lockerImpl) Read(ctx context.Context, fn func()) error {
	return l.do(ctx, fn, l.acquireReader, l.refreshReader, l.releaseReader)
}

func (l *lockerImpl) Write(ctx context.Context, fn func()) error {
	return l.do(ctx, fn, l.acquireWriter, l.refreshWriter, l.releaseWriter)
}

func (l *lockerImpl) do(ctx context.Context, fn func(), acquire func(ctx context.Context) (bool, error), refresh func(ctx context.Context) (bool, error), release func(ctx context.Context) (bool, error)) error {
	if l.redisClient.Ping(ctx).Err() != nil {
		return ErrConnection
	}
	stopRefreshing := make(chan struct{})
	acquired, err := l.execute(ctx, acquire, l.options.RetryCount)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrTimeout
	}
	go l.keepRefreshing(ctx, refresh, stopRefreshing)
	fnErr := l.runFn(fn)
	stopRefreshing <- struct{}{}
	released, err := release(ctx)
	if fnErr != nil {
		return fnErr
	}
	if err != nil {
		return err
	}
	if !released {
		return ErrNotReleased
	}

	return nil

}

func (l *lockerImpl) runFn(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch t := r.(type) {
			case string:
				err = errors.New(t)
			case error:
				err = t
			default:
				err = fmt.Errorf("unknown panic: %v", t)
			}
		}
	}()
	fn()
	return
}

func (l *lockerImpl) execute(ctx context.Context, fn func(ctx context.Context) (bool, error), attempts int) (bool, error) {
	for i := 0; i < attempts; i++ {
		if ok, err := fn(ctx); err != nil {
			return false, err
		} else if ok {
			return true, nil
		} else if err := l.wait(l.options.RetryInterval); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (l *lockerImpl) wait(d time.Duration) error {
	timer := time.NewTimer(d)
	select {
	case <-l.options.Context.Done():
		return ErrInterrupted
	case <-timer.C:
		return nil
	}
}

func (l *lockerImpl) keepRefreshing(ctx context.Context, refresh func(ctx context.Context) (bool, error), stop chan struct{}) {
	timeout := l.options.LockTTL / 2
	timer := time.NewTicker(timeout)
	defer timer.Stop()

	for {
		select {
		case <-stop:
			return
		case <-l.options.Context.Done():
			return
		case <-timer.C:
			refresh(ctx)
		}
	}
}

func (l *lockerImpl) acquireReader(ctx context.Context) (bool, error) {
	var preferWriter = 0
	switch l.options.Mode {
	case ModePreferWriter:
		preferWriter = 1
	case ModePreferReader:
		preferWriter = 0
	default:
		return false, ErrUnknownMode
	}
	return l.execScript(ctx, acquireReadLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
		l.keyWriterIntent,
	}, l.options.ReaderLockToken, l.lockTTL, preferWriter)
}

func (l *lockerImpl) releaseReader(ctx context.Context) (bool, error) {
	return l.execScript(ctx, releaseReadLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
	}, l.options.ReaderLockToken)
}

func (l *lockerImpl) refreshReader(ctx context.Context) (bool, error) {
	return l.execScript(ctx, refreshLock, []string{
		l.keyGlobalLock,
	}, l.options.ReaderLockToken, l.lockTTL)
}

func (l *lockerImpl) acquireWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, acquireWriteLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
		l.keyWriterIntent,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) releaseWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, releaseWriteLock, []string{
		l.keyGlobalLock,
	}, l.writerToken)
}

func (l *lockerImpl) refreshWriter(ctx context.Context) (bool, error) {
	return l.execScript(ctx, refreshLock, []string{
		l.keyGlobalLock,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) execScript(ctx context.Context, script *redis.Script, keys []string, args ...interface{}) (bool, error) {
	status, err := script.Run(ctx, l.redisClient, keys, args...).Result()
	if err != nil {
		return false, err
	}
	if status == int64(1) {
		return true, nil
	}
	return false, nil
}
