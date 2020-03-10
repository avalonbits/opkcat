package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/record"
	"golang.org/x/sync/semaphore"
)

func main() {
	var count int32
	ctx := context.Background()
	sources := opkcat.SourceList(os.Args[1])
	records := make([]*record.Record, len(sources))
	sem := semaphore.NewWeighted(10)
	var wg sync.WaitGroup
	for i, url := range sources {
		wg.Add(1)
		atomic.AddInt32(&count, 1)
		go func(idx int, url string) {
			defer wg.Done()

			if err := sem.Acquire(ctx, 1); err != nil {
				fmt.Printf("Error acquiring semaphore: %v", err)
				return
			}
			defer sem.Release(1)

			rec, err := record.FromOPKURL(url)
			if err != nil {
				fmt.Printf("Error processing %s: %v\n", url, err)
				return
			}
			records[idx] = rec
			fmt.Println("Need to process", atomic.AddInt32(&count, -1))
		}(i, url)
	}
	fmt.Println("Waiting...")
	wg.Wait()
	fmt.Println("Done")
}
