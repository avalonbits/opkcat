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
