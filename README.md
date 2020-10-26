# redis-rwlock
Golang implementation of distributed RW-Lock using Redis.

This implementation was inspired by [Redis distributed locking mechanism](https://redis.io/topics/distlock) and uses Redis LUA scripts.
Configuration:

|Option|Default|Minumum|Description|
|---|---|---|---|
|`LockTTL`|1 sec||100 msec|Lock is set with a timeout to avoid lock leak in case owner dies/disconnects/hangs. Lock TTL is automatically refreshed by owner every `LockTTL/2` while it is acquired. Duration should be less then `RetryCount * RetryInterval` to avoid undesired timeout errors.|
|`RetryCount`|200|0|Lock acquisition internally is a non-blocking operation. To imitate normal blocking behavior we make `RetryCount` attempts to acquire the lock with `RetryInterval` intervals. If lock is not acquired after all attempts caller receives timeout error.|
|`RetryInterval`|10 msec|1 msec|Interval between lock acquisition attempts. See `RetryCount` description.|
|`Context`|`context.Background`|n/a|Execution context. All locking operation use this context. You can use cancelable context for graceful shutdown.|
|`AppID`|`""`|n/a|Any string which will allow you to debug redis calls and locks to detect interference of multiple apps using the same lock.|
|`ReaderLockToken`|`"read_c2d-75a1-4b5b-a6fb-b0754224c666"`|n/a|This token is used as a value for the global lock key when reader acquires the lock. It should be the same for all readers because when tle lock is released we check if it was acquired by us using this token and raise error if not. But if you have separate groups of readers which should not interfere you can use different token to distinguish them.|
|`Mode`|`ModePreferWriter`|n/a|RWMutex can work in two modes: either reader and writer are equal or readers respect writer and do not acquire lock if writers has declared it's intention to acquire the lock. By default `ModePreferWriter` is set to keep back compatibility. See [MutexMode](#Mutex Modes)|

### Mutex Modes
Generally RWMutex is a combination of regular mutex and a counter of readers.
Lock is either free or acquired exclusively by the writer or by a number of readers. First reader acquires lock and increments readers counter. Every new reader increments number of readers and does not do anything with the global lock. When reader releases the lock it first decrements readers counter and if reader was the last one it releases the global lock. That's it.

Reader-preferring RWMutex is simplest implementation. If lock is acquired by reader(s) writer will block but new reader will increment counter and keep global lock acquired. In this case readers may never release lock for the writer and continuously read outdated data.
    
Writer-preferring RWMutex introduce 'Writer Intention'. In this case if lock is acquired by reader(s) and writer tries to acquire the lock it will set an intention. Writer-respecting readers will not increment readers counter and writer will acquire the global lock as soon as all old readers will finish their job.

### Implementation

Every lock operation (rlock, runlock, lock, unlock, refresh) is a single LUA script. You can find scripts with comments in ```lua``` directory.
