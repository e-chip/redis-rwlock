package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	goredis "github.com/go-redis/redis"

	goredisv6 "github.com/e-chip/redis-rwlock/adapters/goredisv6"
	rwlock "github.com/e-chip/redis-rwlock/v2"
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
		defer e.wg.Done()
		for i := 0; i < writeIterations; i++ {
			err := e.locker.Write(context.Background(), func(_ context.Context) error {
				fmt.Printf("Writing...\n")
				time.Sleep(writeDuration)
				(*sharedData)++
				fmt.Printf("Write: %d\n", *sharedData)
				return nil
			})
			if err != nil {
				fmt.Printf("Writing error: %v\n", err)
			}
			time.Sleep(writeInterval)
		}
		close(e.doneC)
	}()
}

func (e *example) ReadSharedData(sharedData *int) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-e.doneC:
				return
			default:
				err := e.locker.Read(context.Background(), func(_ context.Context) error {
					fmt.Printf("Read: %d\n", *sharedData)
					return nil
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
	c := goredis.NewClient(&goredis.Options{
		Network: "tcp",
		Addr:    "localhost:6379",
		DB:      9,
	})
	defer c.Close()

	locker, err := rwlock.New(
		goredisv6.New(c),
		"myapp:rwlock",
		rwlock.Options{Mode: rwlock.ModePreferWriter},
	)
	if err != nil {
		panic(err)
	}

	ex := example{
		locker: locker,
		doneC:  make(chan struct{}),
	}

	sharedData := 0
	for i := 0; i < readersCount; i++ {
		ex.ReadSharedData(&sharedData)
	}
	ex.WriteSharedData(&sharedData)
	ex.Wait()
}
