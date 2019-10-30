-- KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT]
-- ARGV = [TOKEN]

-- decrement ref counter. if last, release global lock.
if redis.call("DECR", KEYS[2]) == 0 then
    -- check that global lock is ours
    if redis.call("GET", KEYS[1]) == ARGV[1] then
        -- release global lock
        redis.call("DEL", KEYS[1], KEYS[2])
        -- success
        return 1
    else
        -- failed to release global lock
        return 0
    end
else
    -- success
    return 1
end
