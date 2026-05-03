module github.com/e-chip/redis-rwlock/examples/prefer_writer

go 1.25.0

require (
	github.com/e-chip/redis-rwlock v0.0.0
	github.com/e-chip/redis-rwlock/adapters/goredisv6 v0.0.0
	github.com/go-redis/redis v6.15.9+incompatible
)

require github.com/gofrs/uuid v3.3.0+incompatible // indirect

replace (
	github.com/e-chip/redis-rwlock => ../..
	github.com/e-chip/redis-rwlock/adapters/goredisv6 => ../../adapters/goredisv6
)
