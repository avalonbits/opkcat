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
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/db"
	"golang.org/x/sync/semaphore"
)

var (
	dbDir = flag.String("db_dir", "",
		"Location of the database. Should point to an existing directory.")
	tmpDir = flag.String("tmp_dir", "",
		"Location use for temporary data. If empty, will use the system default.")
)

type Getter struct {
	client *http.Client
}

func (g *Getter) GetIfModified(since time.Time, etag, url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// We either send the if-none-match or the if-modified-since header, never both or etag matching
	// won't work.
	if etag != "" {
		req.Header["If-None-Match"] = []string{etag}
	} else if !since.IsZero() {
		req.Header["If-Modified-Since"] = []string{since.Format("Mon, 2 Jan 2006 15:04:05 MST")}
	}

	return g.client.Do(req)
}

func main() {
	flag.Parse()

	storage, err := db.Prod(*dbDir)
	if err != nil {
		panic(err)
	}
	defer storage.Close()

	var count int32
	ctx := context.Background()
	sources := opkcat.SourceList(flag.Args()[0])
	sem := semaphore.NewWeighted(10)
	manager := opkcat.NewManager(*tmpDir, &Getter{client: &http.Client{}}, storage)
	var wg sync.WaitGroup
	for _, url := range sources {
		wg.Add(1)
		atomic.AddInt32(&count, 1)
		go func(url string) {
			defer wg.Done()
			defer func() {
				fmt.Println("Need to process", atomic.AddInt32(&count, -1))
			}()

			if err := sem.Acquire(ctx, 1); err != nil {
				fmt.Printf("Error acquiring semaphore: %v", err)
				return
			}
			defer sem.Release(1)
			if err := manager.LoadFromURL(url); err != nil {
				fmt.Printf("Error loading from url: %v\n", err)
				return
			}
		}(url)
	}
	fmt.Println("Waiting...")
	wg.Wait()
	updated, err := manager.PersistRecords()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Done. Wrote %d records\n", updated)
}
