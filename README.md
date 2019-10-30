# redis-rwlock
Golang implementation of distributed RW-Lock (Writer-preferring) using Redis.

This implementation uses [Redis distributed locking mechanism](https://redis.io/topics/distlock) and Redis LUA scripts.
It is writer preferring RW-lock implementation. It means that if writer is trying to acquire lock new readers will wait until writer finishes his job.

Every lock operation (rlock, runlock, lock, unlock, refresh) is a single LUA script. You can find scripts with comments in ```lua``` directory.
