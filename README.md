# redis-rwlock
Golang implementation of distributed RW-Lock (Writer-preferring) using Redis.

This implementation uses [Redis distributed locking mechanism](https://redis.io/topics/distlock) and Redis LUA scripts.
It is writer preferring RW-lock implementation. It means that if writer is trying to acquire lock new readers will wait until writer finishes his job.

Every lock operation (rlock, runlock, lock, unlock, refresh) is a single LUA script. You can find scripts with comments in ```lua``` directory.

# Example
```golang
package main

import (
    "fmt"
    "sync"
    "time"

    "github.com/e-chip/redis-rwlock"
    "github.com/go-redis/redis"
)

func writeSharedData(locker rwlock.Locker, wg *sync.WaitGroup, sharedData *int) {
    for {
        err := locker.Write(func() {
            fmt.Printf("Writing...\n")
            time.Sleep(500 * time.Millisecond)
            (*sharedData)++
            fmt.Printf("Write: %d\n", *sharedData)
        })
        if err != nil {
            fmt.Printf("Writing error: %v\n", err)
            if err != rwlock.ErrTimeout {
                break
            }
        }
        time.Sleep(2 * time.Second)
    }
    wg.Done()
}

func readSharedData(locker rwlock.Locker, wg *sync.WaitGroup, sharedData *int) {
    for {
        err := locker.Read(func() {
            fmt.Printf("Read: %d\n", *sharedData)
        })
        if err != nil {
            fmt.Printf("Read error: %v\n", err)
            if err != rwlock.ErrTimeout {
                break
            }
        }
    }
    wg.Done()
}

func main() {
    var (
        wg          sync.WaitGroup
        sharedData  = 0
        redisClient = redis.NewClient(&redis.Options{
            Network: "tcp",
            Addr:    "localhost:6379",
            DB:      9,
        })
        locker = rwlock.Make(redisClient, "GLOBAL_LOCK", "READER_COUNT", "WRITER_INTENT", &rwlock.Options{})
    )
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go readSharedData(locker, &wg, &sharedData)
    }
    wg.Add(1)
    go writeSharedData(locker, &wg, &sharedData)
    wg.Wait()
}

```
