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
