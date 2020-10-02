package rwlock

import (
	"github.com/go-redis/redis"
)

var (
	acquireReadLock = redis.NewScript(readLockScript)
	releaseReadLock = redis.NewScript(readUnlockScript)

	refreshLock = redis.NewScript(lockRefreshScript)

	acquireWriteLock = redis.NewScript(writeLockScript)
	releaseWriteLock = redis.NewScript(writeUnlockScript)
)

// KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT, WRITER_LOCK_INTENT]
// ARGV = [TOKEN, EXPIRATION_TIMEOUT, WRITER_PREFERRING]
const readLockScript = `
if ARGV[3] ~= 0 and redis.call("EXISTS", KEYS[3]) == 1 then
	return 0
else
	if redis.call("INCR", KEYS[2]) == 1  then
		if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
            return 1
        else
            redis.call("DECR", KEYS[2])
            return 0
        end
    else
        return 1
    end
end`

// KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT]
// ARGV = [TOKEN]
const readUnlockScript = `
if redis.call("DECR", KEYS[2]) == 0 then
    if redis.call("GET", KEYS[1]) == ARGV[1] then
        redis.call("DEL", KEYS[1], KEYS[2])
        return 1
    else
        return 0
    end
else
    return 1
end`

// KEYS = [GLOB_LOCK_KEY]
// ARGV = [TOKEN, EXPIRATION_TIMEOUT]
const lockRefreshScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    return 0
end`

// KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT, WRITER_LOCK_INTENT]
// ARGV = [TOKEN, EXPIRATION_TIMEOUT]
const writeLockScript = `
redis.call("SET", KEYS[3], 1, "PX", ARGV[2])
if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
    redis.call("DEL", KEYS[2], KEYS[3])
    return 1
else
    return 0
end`

// KEYS = [GLOB_LOCK_KEY]
// ARGV = [TOKEN]
const writeUnlockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("DEL", KEYS[1])
    return 1
else
    return 0
end`
