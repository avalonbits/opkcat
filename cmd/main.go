/*
 * Copyright (C) 2020  Igor Cananea <icc@avalonbits.com>
 * Author: Igor Cananea <icc@avalonbits.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/db"
	"github.com/avalonbits/opkcat/record"
	"golang.org/x/sync/semaphore"
)

var (
	dbdir = flag.String("db_dir", "", "Location of the database. Should point to an existing directory.")
)

func main() {
	flag.Parse()

	_, _ = db.Prod(*dbdir)
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
