-- Reader lock acquire.
--
-- Increments the counter. If the result is non-positive a writer has signalled
-- intent (BIAS was subtracted); undo and fail. The first reader (counter == 1)
-- acquires the global lock via SET NX.
--
-- Deterministic: no random or time-based operations.
-- Script-effects replication is enabled so TTLs are propagated as absolute
-- timestamps (PEXPIREAT) to replicas and the AOF, making recovery safe.
--
-- KEYS[1] = lock key
-- KEYS[2] = counter key
-- ARGV[1] = reader token
-- ARGV[2] = lock TTL in ms

-- Opt into script-effects replication (Redis 3.2-6.x). No-op in Redis 7.0+.
if redis.replicate_commands then redis.replicate_commands() end

local c = redis.call("INCR", KEYS[2])
if c <= 0 then
    -- Writer intent is active; undo and fail.
    redis.call("DECR", KEYS[2])
    return 0
end

if c == 1 then
    -- First reader: acquire the global lock.
    if not redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
        redis.call("DECR", KEYS[2])
        return 0
    end
end

return 1
