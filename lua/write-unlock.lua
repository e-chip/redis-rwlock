-- Writer lock release script.

-- This scripts checks if the lock was acquired by the same owner and then releases it.

-- KEYS = [GLOB_LOCK_KEY]
-- ARGV = [TOKEN]

-- check that global lock is ours
if redis.call("GET", KEYS[1]) == ARGV[1] then
    -- release global lock
    redis.call("DEL", KEYS[1])
    -- success
    return 1
else
    -- failed to release global lock
    return 0
end
