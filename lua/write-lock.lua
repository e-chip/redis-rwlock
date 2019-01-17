-- KEYS = [GLOB_LOCK_KEY, READ_LOCK_REF_COUNT, WRITER_LOCK_INTENT]
-- ARGV = [TOKEN, EXPIRATION_TIMEOUT]

-- set writer intention to acquire global lock
redis.call("SET", KEYS[3], 1, "PX", ARGV[2])
-- acquire global lock
if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
    -- global lock acquired. reset intention. remove dangling readers refs.
    redis.call("DEL", KEYS[2], KEYS[3])
    -- success
    return 1
else
    -- failed to acquire global lock
    return 0
end
