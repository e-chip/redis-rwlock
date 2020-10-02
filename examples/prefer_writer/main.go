package main

import (
	"fmt"
	"sync"
	"time"

	rwlock "github.com/e-chip/redis-rwlock"
	"github.com/go-redis/redis"
)

const (
	readersCount    = 10
	writeIterations = 5
	writeDuration   = 500 * time.Millisecond
	writeInterval   = 2 * time.Second
)

type example struct {
	locker rwlock.Locker
	wg     sync.WaitGroup
	doneC  chan struct{}
}

func (e *example) WriteSharedData(sharedData *int) {
	e.wg.Add(1)
	go func() {
		for i := 0; i < writeIterations; i++ {
			err := e.locker.Write(func() {
				fmt.Printf("Writing...\n")
				time.Sleep(writeDuration)
				(*sharedData)++
				fmt.Printf("Write: %d\n", *sharedData)
			})
			if err != nil {
				fmt.Printf("Writing error: %v\n", err)
			}
			time.Sleep(writeInterval)
		}
		close(e.doneC)
		e.wg.Done()
	}()
}

func (e *example) ReadSharedData(sharedData *int) {
	e.wg.Add(1)
	go func() {
		for {
			select {
			case <-e.doneC:
				e.wg.Done()
				return
			default:
				err := e.locker.Read(func() {
					fmt.Printf("Read: %d\n", *sharedData)
				})
				if err != nil {
					fmt.Printf("Read error: %v\n", err)
				}
			}
		}
	}()
}

func (e *example) Wait() {
	e.wg.Wait()
}

func main() {
	var (
		sharedData  = 0
		redisClient = redis.NewClient(&redis.Options{
			Network: "tcp",
			Addr:    "localhost:6379",
			DB:      9,
		})
		example = example{
			locker: rwlock.New(redisClient, "GLOBAL_LOCK", "READER_COUNT", "WRITER_INTENT", &rwlock.Options{}),
			doneC:  make(chan struct{}),
		}
	)
	defer redisClient.Close()
	for i := 0; i < readersCount; i++ {
		example.ReadSharedData(&sharedData)
	}
	example.WriteSharedData(&sharedData)
	example.Wait()
}
