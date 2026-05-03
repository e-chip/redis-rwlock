-- Writer lock release.
--
-- Verifies ownership by token before deleting the lock key.
--
-- Deterministic: no random or time-based operations.
-- Script-effects replication is enabled so individual commands are logged to
-- the AOF rather than the EVAL call, making recovery safe.
--
-- KEYS[1] = lock key
-- ARGV[1] = writer token

-- Opt into script-effects replication (Redis 3.2-6.x). No-op in Redis 7.0+.
if redis.replicate_commands then redis.replicate_commands() end

if redis.call("GET", KEYS[1]) == ARGV[1] then
    redis.call("DEL", KEYS[1])
    return 1
end
return 0
