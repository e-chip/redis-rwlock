//go:build integration

package rwlock_test

// Integration tests run against a real Redis instance.
// Start Redis locally and run:
//
//	go test -tags integration ./...
//
// Override the address with REDIS_ADDR (default: localhost:6379).
//
// These tests cover behaviour that miniredis cannot replicate faithfully:
//   - redis.replicate_commands() compatibility on real Redis 3.2–7.x
//   - real TTL expiry (miniredis requires manual FastForward)
//   - PEXPIRE counter self-heal after a writer crash / timeout
//   - lock refresh keeping the lock alive past the initial TTL

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	goredis "github.com/go-redis/redis"

	rwlock "github.com/e-chip/redis-rwlock/v2"
)

// bias must match the constant in write-lock.lua.
const bias = 1073741824

// integrationRedis returns a real Redis client, skipping the test if Redis is
// unreachable. The address is taken from REDIS_ADDR (default: localhost:6379).
func integrationRedis(t *testing.T) *goredis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	c := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := c.Ping().Err(); err != nil {
		t.Skipf("Redis unavailable at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// integrationLocker creates a Locker backed by real Redis and a cleanup hook.
// The prefix uses a hash tag so both keys land in the same Redis Cluster slot.
func integrationLocker(t *testing.T, rc *goredis.Client, opts rwlock.Options) (rwlock.Locker, string) {
	t.Helper()
	prefix := fmt.Sprintf("{rwlock_int_%s}", t.Name())
	l, err := rwlock.New(&testClient{c: rc}, prefix, opts)
	if err != nil {
		t.Fatalf("rwlock.New: %v", err)
	}
	t.Cleanup(func() {
		rc.Del(prefix+":lock", prefix+":counter")
	})
	return l, prefix
}

// TestIntegrationAllScripts exercises all five Lua scripts on a real Redis
// instance and verifies that redis.replicate_commands() causes no errors.
// A short LockTTL ensures at least one lock-refresh cycle runs.
func TestIntegrationAllScripts(t *testing.T) {
	rc := integrationRedis(t)
	opts := rwlock.Options{
		LockTTL:       150 * time.Millisecond,
		RetryCount:    10,
		RetryInterval: 5 * time.Millisecond,
	}

	l, _ := integrationLocker(t, rc, opts)

	// read-lock → lock-refresh → read-unlock
	if err := l.Read(context.Background(), func(_ context.Context) error {
		time.Sleep(80 * time.Millisecond) // triggers one refresh at TTL/2 = 75ms
		return nil
	}); err != nil {
		t.Fatalf("Read: %v", err)
	}

	// write-lock → lock-refresh → write-unlock
	if err := l.Write(context.Background(), func(_ context.Context) error {
		time.Sleep(80 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// TestIntegrationRefreshKeepsLockAlive verifies that the TTL refresh goroutine
// prevents the lock from expiring while the protected function is running.
func TestIntegrationRefreshKeepsLockAlive(t *testing.T) {
	rc := integrationRedis(t)
	opts := rwlock.Options{
		LockTTL:       200 * time.Millisecond, // refresh every 100ms
		RetryCount:    5,
		RetryInterval: 10 * time.Millisecond,
	}
	l, prefix := integrationLocker(t, rc, opts)

	err := l.Write(context.Background(), func(_ context.Context) error {
		// Hold the lock for 3× the TTL; without refresh it would expire.
		time.Sleep(600 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("Write failed (lock may have expired): %v", err)
	}

	// Lock key must be gone after release.
	if rc.Exists(prefix+":lock").Val() != 0 {
		t.Error("lock key still present after release")
	}
}

// TestIntegrationLockExpiryRecovery verifies that when a lock holder crashes
// (simulated by setting the lock key directly with a short TTL and no refresh),
// the lock expires and a new holder can acquire it.
func TestIntegrationLockExpiryRecovery(t *testing.T) {
	rc := integrationRedis(t)
	opts := rwlock.Options{
		LockTTL:       200 * time.Millisecond,
		RetryCount:    3,
		RetryInterval: 10 * time.Millisecond,
	}
	l, prefix := integrationLocker(t, rc, opts)

	// Simulate a crashed writer: plant the lock key with a short TTL directly.
	lockKey := prefix + ":lock"
	rc.Set(lockKey, "crashed-holder-token", 200*time.Millisecond)

	// A new writer should be blocked while the simulated lock is alive.
	err := l.Write(context.Background(), func(_ context.Context) error { return nil })
	if !errors.Is(err, rwlock.ErrTimeout) {
		t.Fatalf("expected ErrTimeout while simulated lock is held, got %v", err)
	}

	// Wait for the simulated lock to expire.
	time.Sleep(250 * time.Millisecond)

	// Now the new writer must succeed.
	if err := l.Write(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Errorf("expected writer to acquire after lock expiry, got %v", err)
	}
}

// TestIntegrationCounterSelfHeal verifies that when writer intent is set
// (counter < 0) but the writer stops retrying, the PEXPIRE on the counter
// key eventually expires and new readers can acquire the lock.
func TestIntegrationCounterSelfHeal(t *testing.T) {
	rc := integrationRedis(t)
	opts := rwlock.Options{
		LockTTL:       200 * time.Millisecond,
		RetryCount:    3,
		RetryInterval: 10 * time.Millisecond,
	}
	l, prefix := integrationLocker(t, rc, opts)

	// Plant a stuck counter (simulates a writer that set intent and crashed).
	// There is no lock key, so the writer would have acquired immediately in a
	// normal run — we must set the counter directly to simulate the stuck state.
	counterKey := prefix + ":counter"
	rc.Set(counterKey, int64(-bias), 200*time.Millisecond)

	// Readers must fail: counter is negative (writer intent active).
	err := l.Read(context.Background(), func(_ context.Context) error { return nil })
	if !errors.Is(err, rwlock.ErrTimeout) {
		t.Fatalf("expected ErrTimeout while counter signals writer intent, got %v", err)
	}

	// Wait for PEXPIRE to expire.
	time.Sleep(250 * time.Millisecond)

	// Counter is gone → readers can acquire again.
	if err := l.Read(context.Background(), func(_ context.Context) error { return nil }); err != nil {
		t.Errorf("expected reader to succeed after counter self-heal, got %v", err)
	}
}

// TestIntegrationHighConcurrency hammers the lock with many concurrent readers
// and writers and checks that exclusion invariants are never violated.
func TestIntegrationHighConcurrency(t *testing.T) {
	rc := integrationRedis(t)
	opts := rwlock.Options{
		LockTTL:       500 * time.Millisecond,
		RetryCount:    500,
		RetryInterval: 2 * time.Millisecond,
	}

	const (
		readers = 20
		writers = 5
	)

	var (
		mu             sync.Mutex
		readersHolding int
		writerHolding  bool
		violation      bool
		wg             sync.WaitGroup
	)

	check := func(isWriter bool) {
		mu.Lock()
		defer mu.Unlock()
		if isWriter && readersHolding > 0 {
			violation = true
		}
		if !isWriter && writerHolding {
			violation = true
		}
	}

	for range readers {
		wg.Go(func() {
			l, _ := integrationLocker(t, rc, opts)
			if err := l.Read(context.Background(), func(_ context.Context) error {
				mu.Lock()
				readersHolding++
				mu.Unlock()

				check(false)
				time.Sleep(5 * time.Millisecond)
				check(false)

				mu.Lock()
				readersHolding--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("reader error: %v", err)
			}
		})
	}

	for range writers {
		wg.Go(func() {
			l, _ := integrationLocker(t, rc, opts)
			if err := l.Write(context.Background(), func(_ context.Context) error {
				mu.Lock()
				writerHolding = true
				mu.Unlock()

				check(true)
				time.Sleep(10 * time.Millisecond)
				check(true)

				mu.Lock()
				writerHolding = false
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("writer error: %v", err)
			}
		})
	}

	wg.Wait()

	if violation {
		t.Error("exclusion invariant violated: reader and writer held the lock simultaneously")
	}
}
