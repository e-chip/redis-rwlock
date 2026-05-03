-- Lock timer refresh script.

-- This script checks if the lock was acquired by the same owner and then refreshes the expiration timer.

-- KEYS = [GLOB_LOCK_KEY]
-- ARGV = [TOKEN, EXPIRATION_TIMEOUT]

-- check that global lock is ours
if redis.call("GET", KEYS[1]) == ARGV[1] then
    -- update global lock timeout
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    -- failed to update global lock timeout
    return 0
end
