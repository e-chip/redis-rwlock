# redis-rwlock

Distributed RW-lock for Go backed by Redis. Readers share the lock; a writer acquires it exclusively.

All lock operations are single atomic Lua scripts. Automatic TTL refresh keeps the lock alive for the duration of the protected function.

## Requirements

- **Redis 3.2+** — required for `redis.replicate_commands()`, which enables script-effects replication. Without it, Redis replays the entire `EVAL` call on AOF recovery and re-executes scripts with relative TTLs reset to current time.
- **Redis 7.0+** — script-effects replication is automatic; no extra configuration needed.
- **Redis Cluster** — both keys derived from the prefix (`prefix:lock`, `prefix:counter`) must land in the same hash slot. Use a hash tag in the prefix: `"{myapp:resource}"`.

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
    "myapp:rwlock", // unique key prefix — two Redis keys are derived from it
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

| Option            | Default                                  | Minimum | Description                                                                                                                  |
| ----------------- | ---------------------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `LockTTL`         | 1 s                                      | 100 ms  | Lock expiry. Refreshed every `LockTTL/2` while held. Keep below `RetryCount × RetryInterval` to avoid spurious `ErrTimeout`. |
| `RetryCount`      | 200                                      | 1       | Acquisition attempts before returning `ErrTimeout`.                                                                          |
| `RetryInterval`   | 10 ms                                    | 1 ms    | Pause between acquisition attempts.                                                                                          |
| `AppID`           | `""`                                     | —       | Prefix added to the writer token. Useful for identifying which process holds the lock in Redis.                              |
| `ReaderLockToken` | `"read_c2d-75a1-4b5b-a6fb-b0754224c666"` | —       | Shared token for all readers in a group. Override to create independent reader groups on the same key prefix.                |

## Errors

| Error            | When                                                                        |
| ---------------- | --------------------------------------------------------------------------- |
| `ErrTimeout`     | Lock not acquired within `RetryCount` attempts.                             |
| `ErrInterrupted` | Context was cancelled while waiting for the lock.                           |
| `ErrNotReleased` | Lock was held but could not be released (e.g. lock expired before release). |

## Lua scripts

All five operations (read-lock, read-unlock, write-lock, write-unlock, lock-refresh) are Lua scripts embedded at compile time from `lua/`. Each script is a single atomic Redis call.

**Determinism** — scripts contain no random or time-based operations. Given the same database state and arguments they always produce the same result, satisfying Redis replication requirements.

**AOF / replication safety** — every script calls `redis.replicate_commands()` (guarded for Redis < 3.2 compatibility). This enables script-effects replication: Redis logs individual write commands with absolute timestamps (`PEXPIREAT`, `SET PXAT`) instead of the `EVAL` call. On AOF recovery or replica replay, lock TTLs are preserved exactly rather than being reset relative to the replay time.

**Algorithm** — mirrors `sync.RWMutex`: a shared integer counter tracks active readers. When a writer is waiting it subtracts a large bias (`1<<30`) from the counter, making it negative and blocking new readers. The last departing reader detects the negative counter, releases the lock, and the writer acquires it on its next attempt.
