# redis-rwlock

Distributed RW-lock for Go backed by Redis. Readers acquire the lock shared; a writer acquires it exclusively.

Inspired by the [Redis distributed locking pattern](https://redis.io/topics/distlock). All lock operations are single atomic Lua scripts.

## Installation

```redis-rwlock/go.mod#L1-2
module github.com/e-chip/redis-rwlock

go 1.24.0
```

Core package (no Redis dependency):

```/dev/null/sh.txt#L1-1
go get github.com/e-chip/redis-rwlock/pkg/rwlock
```

Pick an adapter for your Redis client:

```/dev/null/sh.txt#L1-3
# go-redis v9 (current)
go get github.com/e-chip/redis-rwlock/adapters/goredis9
# go-redis v6 (legacy)
go get github.com/e-chip/redis-rwlock/adapters/goredisv6
```

## Usage

```/dev/null/example.go#L1-30
import (
    "github.com/e-chip/redis-rwlock/pkg/rwlock"
    goredis9 "github.com/e-chip/redis-rwlock/adapters/goredis9"
    goredis "github.com/redis/go-redis/v9"
)

client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})

locker := rwlock.New(
    goredis9.New(client),
    "myapp:lock",          // global lock key
    "myapp:readers",       // reader reference counter key
    "myapp:writer_intent", // writer intent key
    rwlock.Options{},
)

// Shared read access — multiple readers run concurrently.
err := locker.Read(func() {
    // critical section
})

// Exclusive write access — no readers or other writers run concurrently.
err = locker.Write(func() {
    // critical section
})
```

## Custom Redis client

Implement the two-method `RedisClient` interface to use any Redis client:

```/dev/null/custom.go#L1-14
type RedisClient interface {
    Ping(ctx context.Context) error
    Eval(ctx context.Context, script string, keys []string, args ...interface{}) (int64, error)
}
```

## Options

| Option            | Default                                    | Minimum  | Description |
|-------------------|--------------------------------------------|----------|-------------|
| `LockTTL`         | 1 s                                        | 100 ms   | Lock expiry. Automatically refreshed every `LockTTL/2` while held. Keep lower than `RetryCount × RetryInterval` to avoid spurious `ErrTimeout`. |
| `RetryCount`      | 200                                        | 1        | Number of acquisition attempts before returning `ErrTimeout`. |
| `RetryInterval`   | 10 ms                                      | 1 ms     | Pause between acquisition attempts. |
| `Context`         | `context.Background()`                     | —        | Cancelling the context returns `ErrInterrupted`. |
| `AppID`           | `""`                                       | —        | Prefix added to the writer token. Useful for debugging which instance holds the lock. |
| `ReaderLockToken` | `"read_c2d-75a1-4b5b-a6fb-b0754224c666"` | —        | Shared token for all readers in a group. Override to create independent reader groups on the same keys. |
| `Mode`            | `ModePreferWriter`                         | —        | See [Mutex modes](#mutex-modes). |

## Mutex modes

**`ModePreferWriter`** — writer-preferring (default). When a writer is waiting, it sets an *intent* key. New readers detect the intent and back off, letting the writer proceed as soon as existing readers finish. Prevents writer starvation.

**`ModePreferReader`** — reader-preferring. The intent key is still set by the writer but readers ignore it. A writer must wait until all readers (present and future) release the lock naturally. May starve writers under continuous read load.

## Error reference

| Error | Meaning |
|---|---|
| `ErrConnection` | Redis is unreachable (checked via `Ping` before each operation). |
| `ErrTimeout` | Lock was not acquired within `RetryCount × RetryInterval`. |
| `ErrInterrupted` | The provided `Context` was cancelled while waiting. |
| `ErrNotReleased` | Lock was acquired but could not be released. |
| `ErrUnknownMode` | `Options.Mode` was set to an unrecognised value. |

## Lua scripts

All lock operations are single atomic Lua scripts embedded in the binary (`pkg/rwlock/lua/`). The reference sources with comments live in `lua/`.
