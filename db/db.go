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

type Handle struct {
	db *badger.DB
}

func Prod(location string) (*Handle, error) {
	db, err := badger.Open(badger.DefaultOptions(location))
	if err != nil {
		return nil, err
	}

	return &Handle{
		db: db,
	}, nil
}

func Test() (*Handle, error) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		return nil, err
	}

	return &Handle{
		db: db,
	}, nil
}
