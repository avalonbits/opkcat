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

// Package db implements the database api for opkcat.
package db

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/url"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/dgraph-io/badger/v2"
)

// Handle is a database handle. It can be used to read and write data concurrently.
type Handle struct {
	db    *badger.DB
	index bleve.Index
}

// Record is the record that can be stored in the database.
type Record struct {
	URL     string
	Hash    []byte
	Date    time.Time
	Etag    string
	Entries []*Entry
}

type Entry struct {
	Name        string
	Description string
	Type        string
	Categories  []string
	Icon        []byte
}

type URLFreshness struct {
	URL        string
	LastUpdate time.Time
	Etag       string
}

// Prod returns a production version of the database in location.
func Prod(dbLocation, idxLocation string) (*Handle, error) {
	db, err := badger.Open(badger.DefaultOptions(dbLocation))
	if err != nil {
		return nil, err
	}
	index, err := bleve.Open(idxLocation)
	if err != nil {
		// Path might not exist. Let's try creating it.
		if index, err = bleve.New(idxLocation, bleve.NewIndexMapping()); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &Handle{
		db:    db,
		index: index,
	}, nil
}

// Test returns a test (in-memory) version of the database.
func Test() (*Handle, error) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		return nil, err
	}

	return &Handle{
		db: db,
	}, nil
}

func (h *Handle) Close() error {
	if err := h.db.Close(); err != nil {
		return err
	}
	if err := h.index.Close(); err != nil {
		return err
	}
	return nil
}

func (h *Handle) IndexURL(opkurl string) error {
	return h.db.Update(func(txn *badger.Txn) error {
		fresh, err := h.lastUpdated(opkurl, txn)
		if err != nil {
			return err
		}

		// If we have already indexed the url, we are done.
		if fresh != nil {
			return nil
		}

		var fBuf bytes.Buffer
		fEnc := gob.NewEncoder(&fBuf)
		if err := fEnc.Encode(&freshness{Date: time.Time{}, Etag: ""}); err != nil {
			return err
		}
		return txn.Set([]byte("_url:"+url.PathEscape(opkurl)), fBuf.Bytes())
	})
}

type freshness struct {
	Date time.Time
	Etag string
}

func (h *Handle) Query(qry string) ([]*Record, error) {
	if qry == "" {
		return nil, fmt.Errorf("empty query string")
	}
	query := bleve.NewMatchQuery(qry)
	search := bleve.NewSearchRequestOptions(query, 100, 0, false)
	search.SortBy([]string{"Entries.Name"})
	results, err := h.index.Search(search)
	if err != nil {
		return nil, err
	}

	recIds := make([][]byte, len(results.Hits))
	for i, hit := range results.Hits {
		recIds[i] = []byte(hit.ID)
	}

	records := make([]*Record, 0, len(results.Hits))
	err = h.db.View(func(txn *badger.Txn) error {
		for _, id := range recIds {
			item, err := txn.Get(id)
			if err != nil {
				if err == badger.ErrKeyNotFound {
					fmt.Printf("Did not find record for key %x. Should not happen.\n", id)
					continue
				}
				return err
			}
			err = item.Value(func(data []byte) error {
				record := &Record{}
				buf := bytes.NewBuffer(data)
				dec := gob.NewDecoder(buf)
				if err := dec.Decode(record); err != nil {
					fmt.Println("ERROR DECODING")
					return err
				}
				records = append(records, record)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, err
}

func (h *Handle) KnownURLs() ([]*URLFreshness, error) {
	var urls []*URLFreshness
	err := h.db.View(func(txn *badger.Txn) error {
		prefix := []byte("_url:")
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := bytes.TrimPrefix(item.Key(), prefix)
			opkurl, err := url.PathUnescape(string(key))
			if err != nil {
				return err
			}
			err = item.Value(func(data []byte) error {
				fresh := &freshness{}
				buf := bytes.NewBuffer(data)
				dec := gob.NewDecoder(buf)
				if err := dec.Decode(fresh); err != nil {
					return err
				}
				urls = append(urls, &URLFreshness{
					URL:        opkurl,
					LastUpdate: fresh.Date,
					Etag:       fresh.Etag,
				})
				return nil
			})
			if err != nil {
				return nil
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return urls, nil
}

func (h *Handle) LastUpdated(opkurl string) (time.Time, string, error) {
	if len(opkurl) == 0 {
		return time.Time{}, "", fmt.Errorf("empty url")
	}

	var fresh *freshness
	err := h.db.View(func(txn *badger.Txn) error {
		var err error
		fresh, err = h.lastUpdated(opkurl, txn)
		return err
	})
	if err != nil {
		return time.Time{}, "", err
	}
	if fresh == nil {
		return time.Time{}, "", nil
	}
	return fresh.Date, fresh.Etag, nil
}

func (h *Handle) lastUpdated(opkurl string, txn *badger.Txn) (*freshness, error) {
	key := []byte("_url:" + url.PathEscape(opkurl))
	item, err := txn.Get(key)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}
	fresh := &freshness{}
	err = item.Value(func(data []byte) error {
		buf := bytes.NewBuffer(data)
		dec := gob.NewDecoder(buf)
		return dec.Decode(fresh)
	})

	if err != nil {
		return nil, err
	}
	return fresh, nil
}

// PutRecord will inserr a record into the database.
// If the record already exist, it will be updated.
func (h *Handle) UpdateRecord(rec *Record) error {
	if len(rec.Hash) == 0 {
		return fmt.Errorf("no valid hash for the record")
	}

	// We assume that if the hash exists then the record is valid.
	return h.db.Update(func(txn *badger.Txn) error {
		return h.updateRecord(rec, txn)
	})
}

func (h *Handle) MultiUpdateRecord(records []*Record) (int, error) {
	count := 0
	err := h.db.Update(func(txn *badger.Txn) error {
		for _, rec := range records {
			if len(rec.Hash) == 0 {
				return fmt.Errorf("No valid hash for %s", rec.URL)
			}
			if h.recordExists(rec.Hash, txn) {
				continue
			}

			if err := h.updateRecord(rec, txn); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (h *Handle) updateRecord(rec *Record, txn *badger.Txn) error {
	var eBuf bytes.Buffer
	enc := gob.NewEncoder(&eBuf)
	if err := enc.Encode(rec); err != nil {
		return err
	}
	if err := txn.Set(rec.Hash, eBuf.Bytes()); err != nil {
		return err
	}

	var fBuf bytes.Buffer
	fEnc := gob.NewEncoder(&fBuf)
	if err := fEnc.Encode(&freshness{Date: rec.Date, Etag: rec.Etag}); err != nil {
		return err
	}
	if err := txn.Set([]byte("_url:"+url.PathEscape(rec.URL)), fBuf.Bytes()); err != nil {
		return err
	}

	// Now index the record.
	return h.index.Index(string(rec.Hash), rec)
}

func (h *Handle) recordExists(hash []byte, txn *badger.Txn) bool {
	// Record exists if key exists, no need to read the value.
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false

	itr := txn.NewIterator(opts)
	defer itr.Close()

	// Go straight to our expected key.
	itr.Seek(hash)
	return itr.ValidForPrefix(hash)
}
