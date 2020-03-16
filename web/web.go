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

package web

import (
	"github.com/avalonbits/opkcat/db"
)

type Service struct {
	storage *db.Handle
	quit    chan bool
}

func New(storage *db.Handle) *Service {
	return &Service{
		storage: storage,
		quit:    make(chan bool),
	}
}

func (s *Service) Start() error {
	<-s.quit
	s.quit <- true
	return nil
}

func (s *Service) Stop() error {
	s.quit <- true
	<-s.quit
	return nil
}
