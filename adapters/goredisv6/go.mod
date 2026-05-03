module github.com/e-chip/redis-rwlock/adapters/goredisv6

go 1.25.0

require (
	github.com/e-chip/redis-rwlock v0.0.0
	github.com/go-redis/redis v6.15.9+incompatible
)

require (
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/text v0.36.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)

replace github.com/e-chip/redis-rwlock => ../..
