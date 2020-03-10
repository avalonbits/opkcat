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

	"github.com/avalonbits/opkcat/record"
	"github.com/dgraph-io/badger/v2"
)

// Handle is a database handle. It can be used to read and write data concurrently.
type Handle struct {
	db *badger.DB
}

// Prod returns a production version of the database in location.
func Prod(location string) (*Handle, error) {
	db, err := badger.Open(badger.DefaultOptions(location))
	if err != nil {
		return nil, err
	}

	return &Handle{
		db: db,
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

// PutRecord will inserr a record into the database.
// If the record already exist, it will be updated.
func (h *Handle) UpdateRecord(rec *record.Record) error {
	if len(rec.Hash) == 0 {
		return fmt.Errorf("no valid hash for the record")
	}

	// We assume that if the hash exists then the record is valid.
	return h.db.Update(func(txn *badger.Txn) error {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(rec); err != nil {
			return err
		}
		return txn.Set(rec.Hash, buf.Bytes())
	})
}

func (h *Handle) RecordExists(rec *record.Record) (bool, error) {
	if len(rec.Hash) == 0 {
		return false, fmt.Errorf("no valid hash for the record")
	}

	exists := false
	err := h.db.View(func(txn *badger.Txn) error {
		// Record exists if key exists, no need to read the value.
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		itr := txn.NewIterator(opts)
		defer itr.Close()

		// Go straight to our expected key.
		itr.Seek(rec.Hash)
		exists = itr.ValidForPrefix(rec.Hash)
		return nil
	})
	return exists, err
}
