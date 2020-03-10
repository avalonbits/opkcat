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
	"sync"
	"sync/atomic"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/db"
	"github.com/avalonbits/opkcat/record"
	"golang.org/x/sync/semaphore"
)

var (
	dbDir = flag.String("db_dir", "",
		"Location of the database. Should point to an existing directory.")
	tmpDir = flag.String("tmp_dir", "",
		"Location use for temporary data. If empty, will use the system default.")
)

func main() {
	flag.Parse()

	storage, err := db.Prod(*dbDir)
	if err != nil {
		panic(err)
	}

	var count int32
	ctx := context.Background()
	sources := opkcat.SourceList(flag.Args()[0])
	sem := semaphore.NewWeighted(10)
	var wg sync.WaitGroup
	for i, url := range sources {
		wg.Add(1)
		atomic.AddInt32(&count, 1)
		go func(idx int, url string) {
			defer wg.Done()
			defer func() {
				fmt.Println("Need to process", atomic.AddInt32(&count, -1))
			}()

			if err := sem.Acquire(ctx, 1); err != nil {
				fmt.Printf("Error acquiring semaphore: %v", err)
				return
			}
			defer sem.Release(1)

			rec, err := record.FromOPKURL(*tmpDir, url)
			if err != nil {
				fmt.Printf("Error processing %s: %v\n", url, err)
				return
			}
			exists, err := storage.RecordExists(rec)
			if err != nil {
				fmt.Printf("Error reading record: %v\n", err)
				return
			}

			if exists {
				fmt.Printf("Record for %s already inserted.\n", rec.URL)
				return
			}
			if err := storage.UpdateRecord(rec); err != nil {
				fmt.Printf("Unable to write record: %v", err)
				return
			}
		}(i, url)
	}
	fmt.Println("Waiting...")
	wg.Wait()
	fmt.Println("Done")
}
