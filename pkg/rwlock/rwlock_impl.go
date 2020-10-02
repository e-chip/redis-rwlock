package rwlock

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis"
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

func (l *lockerImpl) Read(fn func()) error {
	return l.do(fn, l.acquireReader, l.refreshReader, l.releaseReader)
}

func (l *lockerImpl) Write(fn func()) error {
	return l.do(fn, l.acquireWriter, l.refreshWriter, l.releaseWriter)
}

func (l *lockerImpl) do(fn func(), acquire func() (bool, error), refresh func() (bool, error), release func() (bool, error)) error {
	if l.redisClient.Ping().Err() != nil {
		return ErrConnection
	}
	stopRefreshing := make(chan struct{})
	acquired, err := l.execute(acquire, l.options.RetryCount)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrTimeout
	}
	go l.keepRefreshing(refresh, stopRefreshing)
	fnErr := l.runFn(fn)
	stopRefreshing <- struct{}{}
	released, err := release()
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

func (l *lockerImpl) execute(fn func() (bool, error), attempts int) (bool, error) {
	for i := 0; i < attempts; i++ {
		if ok, err := fn(); err != nil {
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

func (l *lockerImpl) keepRefreshing(refresh func() (bool, error), stop chan struct{}) {
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
			refresh()
		}
	}
}

func (l *lockerImpl) acquireReader() (bool, error) {
	var preferWriter = 0
	switch l.options.Mode {
	case ModePreferWriter:
		preferWriter = 1
	case ModePreferReader:
		preferWriter = 0
	default:
		return false, ErrUnknownMode
	}
	return l.execScript(acquireReadLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
		l.keyWriterIntent,
	}, l.options.ReaderLockToken, l.lockTTL, preferWriter)
}

func (l *lockerImpl) releaseReader() (bool, error) {
	return l.execScript(releaseReadLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
	}, l.options.ReaderLockToken)
}

func (l *lockerImpl) refreshReader() (bool, error) {
	return l.execScript(refreshLock, []string{
		l.keyGlobalLock,
	}, l.options.ReaderLockToken, l.lockTTL)
}

func (l *lockerImpl) acquireWriter() (bool, error) {
	return l.execScript(acquireWriteLock, []string{
		l.keyGlobalLock,
		l.keyReadersCount,
		l.keyWriterIntent,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) releaseWriter() (bool, error) {
	return l.execScript(releaseWriteLock, []string{
		l.keyGlobalLock,
	}, l.writerToken)
}

func (l *lockerImpl) refreshWriter() (bool, error) {
	return l.execScript(refreshLock, []string{
		l.keyGlobalLock,
	}, l.writerToken, l.lockTTL)
}

func (l *lockerImpl) execScript(script *redis.Script, keys []string, args ...interface{}) (bool, error) {
	status, err := script.Run(l.redisClient, keys, args...).Result()
	if err != nil {
		return false, err
	}
	if status == int64(1) {
		return true, nil
	}
	return false, nil
}
