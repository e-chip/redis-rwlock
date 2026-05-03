module github.com/e-chip/redis-rwlock/adapters/goredis9

go 1.25.0

require (
	github.com/e-chip/redis-rwlock v0.0.0
	github.com/redis/go-redis/v9 v9.7.3
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
)

replace github.com/e-chip/redis-rwlock => ../..
