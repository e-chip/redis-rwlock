-- Lock TTL refresh.
--
-- Verifies ownership by token and extends the lock expiry.
--
-- Deterministic: no random or time-based operations.
-- Script-effects replication is enabled so PEXPIRE is logged as PEXPIREAT
-- (absolute timestamp) to the AOF and replicas, preserving the correct
-- remaining TTL across restarts and replica failovers.
--
-- KEYS[1] = lock key
-- ARGV[1] = token
-- ARGV[2] = lock TTL in ms

-- Opt into script-effects replication (Redis 3.2-6.x). No-op in Redis 7.0+.
if redis.replicate_commands then redis.replicate_commands() end

if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
    return 1
end
return 0
