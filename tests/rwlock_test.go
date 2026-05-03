package rwlock_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis"

	rwlock "github.com/e-chip/redis-rwlock/v2"
)

// testClient wraps a go-redis v6 client to implement rwlock.RedisClient for tests.
type testClient struct {
	c *goredis.Client
}

func (tc *testClient) Eval(_ context.Context, script string, keys []string, args ...any) (int64, error) {
	res, err := tc.c.Eval(script, keys, args...).Result()
	if err != nil {
		return 0, err
	}
	v, ok := res.(int64)
	if !ok {
		return 0, nil
	}
	return v, nil
}

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *testClient) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	c := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })

	return mr, &testClient{c: c}
}

func newTestLocker(t *testing.T, client rwlock.RedisClient, opts rwlock.Options) rwlock.Locker {
	t.Helper()
	l, err := rwlock.New(client, "testlock", opts)
	if err != nil {
		t.Fatalf("rwlock.New: %v", err)
	}
	return l
}

// --- Basic correctness ---

// TestReadLock verifies that Read executes the function and returns no error.
func TestReadLock(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	called := false
	err := l.Read(context.Background(), func(_ context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
}

// TestWriteLock verifies that Write executes the function and returns no error.
func TestWriteLock(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	called := false
	err := l.Write(context.Background(), func(_ context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
}

// TestReadLockReleasesAfterFn verifies the lock is properly released so subsequent calls succeed.
func TestReadLockReleasesAfterFn(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	for i := range 3 {
		if err := l.Read(context.Background(), func(_ context.Context) error { return nil }); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
}

// TestWriteLockReleasesAfterFn verifies the lock is properly released so subsequent calls succeed.
func TestWriteLockReleasesAfterFn(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	for i := range 3 {
		if err := l.Write(context.Background(), func(_ context.Context) error { return nil }); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
}

// TestFnError verifies that an error returned by fn is propagated to the caller.
func TestFnError(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	sentinel := errors.New("fn error")
	err := l.Read(context.Background(), func(_ context.Context) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// --- Concurrency ---

// TestConcurrentReaders verifies that multiple readers can hold the lock simultaneously.
func TestConcurrentReaders(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	const n = 5
	var (
		wg            sync.WaitGroup
		mu            sync.Mutex
		concurrent    int
		maxConcurrent int
	)

	for range n {
		wg.Go(func() {
			if err := l.Read(context.Background(), func(_ context.Context) error {
				mu.Lock()
				concurrent++
				if concurrent > maxConcurrent {
					maxConcurrent = concurrent
				}
				mu.Unlock()

				time.Sleep(20 * time.Millisecond)

				mu.Lock()
				concurrent--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	wg.Wait()

	if maxConcurrent < 2 {
		t.Errorf("expected concurrent readers, max concurrent was %d", maxConcurrent)
	}
}

// TestWriterExcludesOtherWriters verifies that only one writer holds the lock at a time.
func TestWriterExcludesOtherWriters(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{
		RetryCount:    100,
		RetryInterval: 5 * time.Millisecond,
	})

	const n = 3
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		concurrent int
		violation  bool
	)

	for range n {
		wg.Go(func() {
			if err := l.Write(context.Background(), func(_ context.Context) error {
				mu.Lock()
				concurrent++
				if concurrent > 1 {
					violation = true
				}
				mu.Unlock()

				time.Sleep(20 * time.Millisecond)

				mu.Lock()
				concurrent--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	wg.Wait()

	if violation {
		t.Error("multiple writers held the lock simultaneously")
	}
}

// TestWriterExcludesReaders verifies that a writer and reader(s) never overlap.
// Readers may run concurrently with each other, but a writer must be fully exclusive.
func TestWriterExcludesReaders(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{
		RetryCount:    200,
		RetryInterval: 5 * time.Millisecond,
	})

	var (
		mu             sync.Mutex
		readersHolding int
		writerHolding  bool
		violation      bool
		wg             sync.WaitGroup
	)

	enterWriter := func() {
		mu.Lock()
		defer mu.Unlock()
		if readersHolding > 0 {
			violation = true
		}
		writerHolding = true
	}
	exitWriter := func() {
		mu.Lock()
		writerHolding = false
		mu.Unlock()
	}
	enterReader := func() {
		mu.Lock()
		defer mu.Unlock()
		if writerHolding {
			violation = true
		}
		readersHolding++
	}
	exitReader := func() {
		mu.Lock()
		readersHolding--
		mu.Unlock()
	}

	wg.Go(func() {
		if err := l.Write(context.Background(), func(_ context.Context) error {
			enterWriter()
			time.Sleep(20 * time.Millisecond)
			exitWriter()
			return nil
		}); err != nil {
			t.Errorf("writer error: %v", err)
		}
	})

	for range 3 {
		wg.Go(func() {
			if err := l.Read(context.Background(), func(_ context.Context) error {
				enterReader()
				time.Sleep(10 * time.Millisecond)
				exitReader()
				return nil
			}); err != nil {
				t.Errorf("reader error: %v", err)
			}
		})
	}

	wg.Wait()

	if violation {
		t.Error("writer and reader(s) held the lock simultaneously")
	}
}

// --- Error cases ---

// TestErrTimeout verifies that ErrTimeout is returned when the lock cannot be acquired in time.
func TestErrTimeout(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{
		RetryCount:    3,
		RetryInterval: time.Millisecond,
	})

	acquired := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = l.Write(context.Background(), func(_ context.Context) error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired

	err := l.Write(context.Background(), func(_ context.Context) error { return nil })
	close(release)

	if !errors.Is(err, rwlock.ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

// TestErrInterrupted verifies that ErrInterrupted is returned when the context is cancelled.
func TestErrInterrupted(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{
		RetryInterval: 10 * time.Millisecond,
	})

	acquired := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = l.Write(context.Background(), func(_ context.Context) error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	err := l.Write(ctx, func(_ context.Context) error { return nil })
	close(release)

	if !errors.Is(err, rwlock.ErrInterrupted) {
		t.Errorf("expected ErrInterrupted, got %v", err)
	}
}

// TestRedisUnavailable verifies that a non-nil, non-timeout error is returned when Redis is down.
func TestRedisUnavailable(t *testing.T) {
	mr, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})
	mr.Close()

	err := l.Read(context.Background(), func(_ context.Context) error { return nil })
	if err == nil {
		t.Fatal("expected error when Redis is unavailable, got nil")
	}
	if errors.Is(err, rwlock.ErrTimeout) {
		t.Fatal("expected connection error, got ErrTimeout")
	}
}

// --- Panic recovery ---

// TestPanicRecovery verifies that a panic inside fn is caught and returned as an error.
func TestPanicRecovery(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	err := l.Read(context.Background(), func(_ context.Context) error {
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected an error from panic recovery, got nil")
	}
	if err.Error() != "test panic" {
		t.Errorf("expected 'test panic', got %q", err.Error())
	}
}

// TestPanicRecoveryErrorType verifies that an error-typed panic is preserved.
func TestPanicRecoveryErrorType(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	sentinel := context.DeadlineExceeded
	err := l.Write(context.Background(), func(_ context.Context) error {
		panic(sentinel)
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// --- Mutex modes ---

// TestModePreferWriterBlocksNewReaders verifies that in ModePreferWriter, a late reader is blocked
// while a writer has declared its intent to acquire the lock.
func TestModePreferWriterBlocksNewReaders(t *testing.T) {
	_, tc := setupMiniredis(t)

	opts := rwlock.Options{
		Mode:          rwlock.ModePreferWriter,
		RetryCount:    3,
		RetryInterval: 5 * time.Millisecond,
	}
	readerLocker, _ := rwlock.New(tc, "modetest", opts)
	writerLocker, _ := rwlock.New(tc, "modetest", opts)

	readerAcquired := make(chan struct{})
	readerRelease := make(chan struct{})

	go func() {
		_ = readerLocker.Read(context.Background(), func(_ context.Context) error {
			close(readerAcquired)
			<-readerRelease
			return nil
		})
	}()
	<-readerAcquired

	writerDone := make(chan error, 1)
	go func() {
		writerDone <- writerLocker.Write(context.Background(), func(_ context.Context) error { return nil })
	}()

	// Give the writer time to set its intent key.
	time.Sleep(20 * time.Millisecond)

	lateReaderLocker, _ := rwlock.New(tc, "modetest", opts)
	err := lateReaderLocker.Read(context.Background(), func(_ context.Context) error { return nil })

	close(readerRelease)
	<-writerDone

	if !errors.Is(err, rwlock.ErrTimeout) {
		t.Errorf("expected late reader to be blocked by writer intent (ErrTimeout), got %v", err)
	}
}

// TestModePreferReaderAllowsNewReaders verifies that in ModePreferReader, a late reader is NOT blocked
// by writer intent and can join the existing reader group.
func TestModePreferReaderAllowsNewReaders(t *testing.T) {
	_, tc := setupMiniredis(t)

	opts := rwlock.Options{
		Mode:          rwlock.ModePreferReader,
		RetryCount:    3,
		RetryInterval: 5 * time.Millisecond,
	}
	readerLocker, _ := rwlock.New(tc, "modetest", opts)
	writerLocker, _ := rwlock.New(tc, "modetest", opts)

	readerAcquired := make(chan struct{})
	readerRelease := make(chan struct{})

	go func() {
		_ = readerLocker.Read(context.Background(), func(_ context.Context) error {
			close(readerAcquired)
			<-readerRelease
			return nil
		})
	}()
	<-readerAcquired

	writerDone := make(chan error, 1)
	go func() {
		writerDone <- writerLocker.Write(context.Background(), func(_ context.Context) error { return nil })
	}()

	time.Sleep(20 * time.Millisecond)

	lateReaderLocker, _ := rwlock.New(tc, "modetest", opts)
	err := lateReaderLocker.Read(context.Background(), func(_ context.Context) error { return nil })

	close(readerRelease)
	<-writerDone

	if err != nil {
		t.Errorf("expected late reader to succeed in ModePreferReader, got %v", err)
	}
}

// --- Options and constructor ---

// TestCustomOptions verifies that non-default options are accepted without error.
func TestCustomOptions(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{
		LockTTL:       500 * time.Millisecond,
		RetryCount:    10,
		RetryInterval: 5 * time.Millisecond,
		AppID:         "test-app",
	})

	if err := l.Write(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("unexpected error with custom options: %v", err)
	}
}

// TestNewEmptyPrefix verifies that New rejects an empty keyPrefix.
func TestNewEmptyPrefix(t *testing.T) {
	_, tc := setupMiniredis(t)
	_, err := rwlock.New(tc, "", rwlock.Options{})
	if err == nil {
		t.Fatal("expected error for empty keyPrefix, got nil")
	}
}

// TestNewUnknownMode verifies that New rejects an unrecognised Mode value.
func TestNewUnknownMode(t *testing.T) {
	_, tc := setupMiniredis(t)
	_, err := rwlock.New(tc, "testlock", rwlock.Options{Mode: rwlock.Mode(99)})
	if err == nil {
		t.Fatal("expected error for unknown mode, got nil")
	}
}

// TestReadAfterWrite verifies that after a write lock is released, a read lock can be acquired.
func TestReadAfterWrite(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	if err := l.Write(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := l.Read(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("read after write error: %v", err)
	}
}

// TestWriteAfterRead verifies that after a read lock is released, a write lock can be acquired.
func TestWriteAfterRead(t *testing.T) {
	_, tc := setupMiniredis(t)
	l := newTestLocker(t, tc, rwlock.Options{})

	if err := l.Read(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if err := l.Write(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Fatalf("write after read error: %v", err)
	}
}
