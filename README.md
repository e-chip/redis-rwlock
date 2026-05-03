# redis-rwlock

Distributed RW-lock for Go backed by Redis. Readers share the lock; a writer acquires it exclusively.

All lock operations are single atomic Lua scripts. Automatic TTL refresh keeps the lock alive for the duration of the protected function.

## Installation

```/dev/null/sh.txt#L1-1
go get github.com/e-chip/redis-rwlock/v2
```

Pick an adapter for your Redis client:

```/dev/null/sh.txt#L1-4
# go-redis v9 (current)
go get github.com/e-chip/redis-rwlock/adapters/goredisv9

# go-redis v6 (legacy)
go get github.com/e-chip/redis-rwlock/adapters/goredisv6
```

## Usage

```/dev/null/example.go#L1-32
import (
    "context"

    rwlock  "github.com/e-chip/redis-rwlock/v2"
    goredisv9 "github.com/e-chip/redis-rwlock/adapters/goredisv9"
    goredis  "github.com/redis/go-redis/v9"
)

client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})

locker, err := rwlock.New(
    goredisv9.New(client),
    "myapp:rwlock", // unique key prefix — three Redis keys are derived from it
    rwlock.Options{},
)
if err != nil { ... }

// Shared read access — multiple readers run concurrently.
err = locker.Read(ctx, func(ctx context.Context) error {
    // critical section
    return nil
})

// Exclusive write access — fully serialised.
err = locker.Write(ctx, func(ctx context.Context) error {
    // critical section
    return nil
})
```

## Custom Redis client

Implement the single-method `RedisClient` interface to integrate any Redis client:

```/dev/null/iface.go#L1-4
type RedisClient interface {
    Eval(ctx context.Context, script string, keys []string, args ...any) (int64, error)
}
```

## Options

| Option            | Default                                    | Minimum | Description |
|-------------------|--------------------------------------------|---------|-------------|
| `LockTTL`         | 1 s                                        | 100 ms  | Lock expiry. Refreshed every `LockTTL/2` while held. Keep below `RetryCount × RetryInterval` to avoid spurious `ErrTimeout`. |
| `RetryCount`      | 200                                        | 1       | Acquisition attempts before returning `ErrTimeout`. |
| `RetryInterval`   | 10 ms                                      | 1 ms    | Pause between acquisition attempts. |
| `AppID`           | `""`                                       | —       | Prefix added to the writer token. Useful for identifying which process holds the lock in Redis. |
| `ReaderLockToken` | `"read_c2d-75a1-4b5b-a6fb-b0754224c666"` | —       | Shared token for all readers in a group. Override to create independent reader groups on the same key prefix. |
| `Mode`            | `ModePreferWriter`                         | —       | See [Mutex modes](#mutex-modes). |

## Mutex modes

**`ModePreferWriter`** (default) — when a writer is waiting, it sets an *intent* key. New readers back off until the writer acquires the lock. Prevents writer starvation.

**`ModePreferReader`** — readers ignore the writer intent key. Writers must wait for all readers (present and future) to finish. May starve writers under sustained read load.

## Errors

| Error | When |
|---|---|
| `ErrTimeout` | Lock not acquired within `RetryCount` attempts. |
| `ErrInterrupted` | Context was cancelled while waiting for the lock. |
| `ErrNotReleased` | Lock was held but could not be released (e.g. lock expired before release). |

## Lua scripts

All five operations (read-lock, read-unlock, write-lock, write-unlock, lock-refresh) are Lua scripts embedded at compile time from `lua/`. Each script is a single atomic Redis call and includes inline comments.
