module github.com/e-chip/redis-rwlock/tests

go 1.25.0

require (
	github.com/alicebob/miniredis/v2 v2.33.0
	github.com/e-chip/redis-rwlock/v2 v2.0.0
	github.com/go-redis/redis v6.15.9+incompatible
)

replace github.com/e-chip/redis-rwlock/v2 => ..

require (
	github.com/alicebob/gopher-json v0.0.0-20200520072559-a9ecdc9d1d3a // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.40.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)
