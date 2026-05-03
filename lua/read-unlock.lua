-- Reader lock release.
--
-- Decrements the counter. Releases the global lock when the counter reaches 0
-- (no writer waiting) or -BIAS (writer was waiting and is now clear to acquire).
--
-- Deterministic: no random or time-based operations.
-- Script-effects replication is enabled so individual commands are logged to
-- the AOF rather than the EVAL call, making recovery safe.
--
-- KEYS[1] = lock key
-- KEYS[2] = counter key
-- ARGV[1] = reader token

-- Opt into script-effects replication (Redis 3.2-6.x). No-op in Redis 7.0+.
if redis.replicate_commands then redis.replicate_commands() end

local BIAS = 1073741824

local c = redis.call("DECR", KEYS[2])
if c == 0 or c == -BIAS then
    if redis.call("GET", KEYS[1]) == ARGV[1] then
        redis.call("DEL", KEYS[1], KEYS[2])
    end
end

return 1
