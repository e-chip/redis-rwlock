-- Reader lock acquire script.

-- If key WRITER_PREFERRING is not 0 this script retreats if writer has set it's intention to acquire lock.
-- The script increments number of shared locks and if it was the first to do that it tries to acquire lock.
-- If it fails to acquire lock then it decrements number of shared locks back to previous value.

-- KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT, WRITER_LOCK_INTENT]
-- ARGV = [TOKEN, EXPIRATION_TIMEOUT, WRITER_PREFERRING]

-- if writer preferring enabled then check writer intention to acquire lock
if ARGV[3] ~= 0 and redis.call("EXISTS", KEYS[3]) == 1 then
    -- failed
    return 0
else
    -- increment ref counter. if first, acquire global lock.
    if redis.call("INCR", KEYS[2]) == 1  then
        -- acquire global lock
        if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
            -- global lock acquired. success
            return 1
        else
            -- global lock not acquired. decrement ref counter
            redis.call("DECR", KEYS[2])
            -- failed
            return 0
        end
    else
        -- global lock must be acquired by some other reader. success
        return 1
    end
end
