-- Writer lock acquire.
--
-- Mirrors sync.RWMutex: subtracts BIAS from the counter to signal intent and
-- block new readers. Tries to acquire the global lock via SET NX; readers
-- release it when the last one departs and the counter reaches -BIAS.
-- PEXPIRE is set on every failed attempt so the counter self-heals if the
-- writer crashes or times out.
--
-- Deterministic: no random or time-based operations.
-- Script-effects replication is enabled so PEXPIRE and SET PX are logged as
-- PEXPIREAT / SET PXAT (absolute timestamps) to the AOF and replicas.
-- This preserves correct TTL values across restarts and replica failovers.
--
-- KEYS[1] = lock key
-- KEYS[2] = counter key
-- ARGV[1] = writer token
-- ARGV[2] = lock TTL in ms

-- Opt into script-effects replication (Redis 3.2-6.x). No-op in Redis 7.0+.
if redis.replicate_commands then redis.replicate_commands() end

local BIAS = 1073741824

-- Signal intent if not already done (counter non-negative means no writer yet).
local c = tonumber(redis.call("GET", KEYS[2]) or "0")
if c >= 0 then
    redis.call("DECRBY", KEYS[2], BIAS)
end

-- Acquire the lock. Readers release it when the last one decrements the
-- counter to -BIAS and deletes both keys.
if redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2], "NX") then
    redis.call("DEL", KEYS[2])
    return 1
end

-- Not acquired yet; refresh counter TTL so it expires if we stop retrying.
redis.call("PEXPIRE", KEYS[2], ARGV[2])
return 0
