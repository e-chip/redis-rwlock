package rwlock_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis"

	"github.com/e-chip/redis-rwlock/pkg/rwlock"
)

// testClient wraps a go-redis v6 client to implement rwlock.RedisClient for tests.
type testClient struct {
	c *goredis.Client
}

func (tc *testClient) Ping(_ context.Context) error {
	return tc.c.Ping().Err()
}

func (tc *testClient) Eval(_ context.Context, script string, keys []string, args ...any) (int64, error) {
	res, err := tc.c.Eval(script, keys, args...).Result()
	if err != nil {
		return 0, err
	}
	v, ok := res.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected result type %T from Eval", res)
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
	return rwlock.New(client, "lock", "readers", "intent", opts)
}

// TestReadLock verifies that Read executes the function and returns no error.
func TestReadLock(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	called := false
	err := locker.Read(func() { called = true })

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("function was not called")
	}
}

// TestWriteLock verifies that Write executes the function and returns no error.
func TestWriteLock(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	called := false
	err := locker.Write(func() { called = true })

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("function was not called")
	}
}

// TestReadLockReleasesAfterFn verifies that subsequent Read calls succeed (lock is properly released).
func TestReadLockReleasesAfterFn(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	for i := range 3 {
		if err := locker.Read(func() {}); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
}

// TestWriteLockReleasesAfterFn verifies that subsequent Write calls succeed (lock is properly released).
func TestWriteLockReleasesAfterFn(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	for i := range 3 {
		if err := locker.Write(func() {}); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
}

// TestConcurrentReaders verifies that multiple readers can hold the lock simultaneously.
func TestConcurrentReaders(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	const n = 5
	var (
		wg            sync.WaitGroup
		mu            sync.Mutex
		concurrent    int
		maxConcurrent int
	)

	for range n {
		wg.Go(func() {
			if err := locker.Read(func() {
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
	locker := newTestLocker(t, tc, rwlock.Options{
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
			if err := locker.Write(func() {
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

// TestWriterExcludesReaders verifies that a writer blocks until all readers release and vice versa.
// Readers may run concurrently with each other, but a writer must be fully exclusive.
func TestWriterExcludesReaders(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{
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

	// Start a writer.
	wg.Go(func() {
		if err := locker.Write(func() {
			enterWriter()
			time.Sleep(20 * time.Millisecond)
			exitWriter()
		}); err != nil {
			t.Errorf("writer error: %v", err)
		}
	})

	// Start readers that will contend with the writer.
	for range 3 {
		wg.Go(func() {
			if err := locker.Read(func() {
				enterReader()
				time.Sleep(10 * time.Millisecond)
				exitReader()
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

// TestErrTimeout verifies that ErrTimeout is returned when the lock cannot be acquired in time.
func TestErrTimeout(t *testing.T) {
	_, tc := setupMiniredis(t)

	locker := newTestLocker(t, tc, rwlock.Options{
		RetryCount:    3,
		RetryInterval: time.Millisecond,
	})

	acquired := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = locker.Write(func() {
			close(acquired)
			<-release
		})
	}()

	<-acquired

	err := locker.Write(func() {})
	close(release)

	if err != rwlock.ErrTimeout {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

// TestErrInterrupted verifies that ErrInterrupted is returned when the context is cancelled.
func TestErrInterrupted(t *testing.T) {
	_, tc := setupMiniredis(t)

	ctx, cancel := context.WithCancel(context.Background())
	locker := newTestLocker(t, tc, rwlock.Options{
		Context:       ctx,
		RetryInterval: 10 * time.Millisecond,
	})

	acquired := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = locker.Write(func() {
			close(acquired)
			<-release
		})
	}()

	<-acquired

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	err := locker.Write(func() {})
	close(release)

	if err != rwlock.ErrInterrupted {
		t.Errorf("expected ErrInterrupted, got %v", err)
	}
}

// TestPanicRecovery verifies that a panic inside fn is caught and returned as an error.
func TestPanicRecovery(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	err := locker.Read(func() {
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected an error from panic recovery, got nil")
	}
	if err.Error() != "test panic" {
		t.Errorf("expected panic message 'test panic', got %q", err.Error())
	}
}

// TestPanicRecoveryErrorType verifies that an error-typed panic is preserved.
func TestPanicRecoveryErrorType(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	sentinel := context.DeadlineExceeded
	err := locker.Write(func() {
		panic(sentinel)
	})

	if err != sentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// TestErrConnection verifies that ErrConnection is returned when Redis is unavailable.
func TestErrConnection(t *testing.T) {
	mr, tc := setupMiniredis(t)

	locker := newTestLocker(t, tc, rwlock.Options{})
	mr.Close()

	err := locker.Read(func() {})
	if err != rwlock.ErrConnection {
		t.Errorf("expected ErrConnection, got %v", err)
	}
}

// TestModePreferWriterBlocksNewReaders verifies that in ModePreferWriter, a late reader is blocked
// while a writer has declared its intent to acquire the lock.
func TestModePreferWriterBlocksNewReaders(t *testing.T) {
	_, tc := setupMiniredis(t)

	opts := rwlock.Options{
		Mode:          rwlock.ModePreferWriter,
		RetryCount:    3,
		RetryInterval: 5 * time.Millisecond,
	}
	// Use two separate lockers sharing the same keys: one reader, one writer.
	readerLocker := rwlock.New(tc, "lock", "readers", "intent", opts)
	writerLocker := rwlock.New(tc, "lock", "readers", "intent", opts)

	readerAcquired := make(chan struct{})
	readerRelease := make(chan struct{})

	// Reader 1: holds the read lock.
	go func() {
		_ = readerLocker.Read(func() {
			close(readerAcquired)
			<-readerRelease
		})
	}()
	<-readerAcquired

	// Writer: sets writer intent (will block until reader 1 releases).
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- writerLocker.Write(func() {})
	}()

	// Give the writer goroutine time to set its intent key.
	time.Sleep(20 * time.Millisecond)

	// Reader 2: should be blocked because writer intent is set.
	lateReaderLocker := rwlock.New(tc, "lock", "readers", "intent", opts)
	err := lateReaderLocker.Read(func() {})

	close(readerRelease)
	<-writerDone

	if err != rwlock.ErrTimeout {
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
	readerLocker := rwlock.New(tc, "lock", "readers", "intent", opts)
	writerLocker := rwlock.New(tc, "lock", "readers", "intent", opts)

	readerAcquired := make(chan struct{})
	readerRelease := make(chan struct{})

	// Reader 1: holds the read lock.
	go func() {
		_ = readerLocker.Read(func() {
			close(readerAcquired)
			<-readerRelease
		})
	}()
	<-readerAcquired

	// Writer: sets writer intent (will block until reader 1 releases).
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- writerLocker.Write(func() {})
	}()

	// Give the writer goroutine time to set its intent key.
	time.Sleep(20 * time.Millisecond)

	// Reader 2: must succeed in ModePreferReader despite writer intent.
	lateReaderLocker := rwlock.New(tc, "lock", "readers", "intent", opts)
	err := lateReaderLocker.Read(func() {})

	close(readerRelease)
	<-writerDone

	if err != nil {
		t.Errorf("expected late reader to succeed in ModePreferReader, got %v", err)
	}
}

// TestLockTTLOption verifies that custom LockTTL is accepted without error.
func TestCustomOptions(t *testing.T) {
	_, tc := setupMiniredis(t)

	locker := newTestLocker(t, tc, rwlock.Options{
		LockTTL:       500 * time.Millisecond,
		RetryCount:    10,
		RetryInterval: 5 * time.Millisecond,
		AppID:         "test-app",
	})

	if err := locker.Write(func() {}); err != nil {
		t.Fatalf("unexpected error with custom options: %v", err)
	}
}

// TestReadAfterWrite verifies that after a write lock is released, a read lock can be acquired.
func TestReadAfterWrite(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	if err := locker.Write(func() {}); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := locker.Read(func() {}); err != nil {
		t.Fatalf("read after write error: %v", err)
	}
}

// TestWriteAfterRead verifies that after a read lock is released, a write lock can be acquired.
func TestWriteAfterRead(t *testing.T) {
	_, tc := setupMiniredis(t)
	locker := newTestLocker(t, tc, rwlock.Options{})

	if err := locker.Read(func() {}); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if err := locker.Write(func() {}); err != nil {
		t.Fatalf("write after read error: %v", err)
	}
}
