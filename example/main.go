package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/e-chip/redis-rwlock"
	"github.com/go-redis/redis"
)

func writeSharedData(locker rwlock.Locker, wg *sync.WaitGroup, sharedData *int) {
	for {
		err := locker.Write(func() {
			fmt.Printf("Writing...\n")
			time.Sleep(500 * time.Millisecond)
			(*sharedData)++
			fmt.Printf("Write: %d\n", *sharedData)
		})
		if err != nil {
			fmt.Printf("Writing error: %v\n", err)
			if err != rwlock.ErrTimeout {
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	wg.Done()
}

func readSharedData(locker rwlock.Locker, wg *sync.WaitGroup, sharedData *int) {
	for {
		err := locker.Read(func() {
			fmt.Printf("Read: %d\n", *sharedData)
		})
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
			if err != rwlock.ErrTimeout {
				break
			}
		}
	}
	wg.Done()
}

func main() {
	var (
		wg          sync.WaitGroup
		sharedData  = 0
		redisClient = redis.NewClient(&redis.Options{
			Network: "tcp",
			Addr:    "localhost:6379",
			DB:      9,
		})
		locker = rwlock.Make(redisClient, "GLOBAL_LOCK", "READER_COUNT", "WRITER_INTENT", &rwlock.Options{})
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go readSharedData(locker, &wg, &sharedData)
	}
	wg.Add(1)
	go writeSharedData(locker, &wg, &sharedData)
	wg.Wait()
}
