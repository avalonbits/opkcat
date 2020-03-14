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
	"flag"
	"net/http"
	"time"

	"github.com/avalonbits/opkcat"
	"github.com/avalonbits/opkcat/db"
	"github.com/avalonbits/opkcat/fetcher"
)

var (
	dbDir = flag.String("db_dir", "",
		"Location of the database. Should point to an existing directory.")
	idxFile = flag.String("idx_file", "", "Location of the full-text index file.")
	tmpDir  = flag.String("tmp_dir", "",
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

	storage, err := db.Prod(*dbDir, *idxFile)
	if err != nil {
		panic(err)
	}
	defer storage.Close()

	fetchServ := fetcher.New(*tmpDir, storage, &Getter{client: &http.Client{}}, 10)
	for _, source := range opkcat.SourceList(flag.Args()[0]) {
		fetchServ.Add(source)
	}
	if err := fetchServ.Fetch(); err != nil {
		panic(err)
	}
}
