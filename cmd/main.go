package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/record"
)

func main() {
	sources := opkcat.SourceList(os.Args[1])
	records := make([]*record.Record, len(sources))
	var wg sync.WaitGroup
	for i, url := range sources {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			rec, err := record.FromOPKURL(url)
			if err != nil {

				panic(fmt.Sprintf("Error processing %s: %v", url, err))
			}
			records[idx] = rec
			fmt.Println(*rec)
		}(i, url)
	}
	wg.Wait()
}
