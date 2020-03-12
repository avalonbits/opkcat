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

package opkcat

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"sync"

	"github.com/avalonbits/opkcat/db"
	"github.com/avalonbits/opkcat/record"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

var opkEnd = []byte(".opk")

type HttpHeadGetter interface {
	Head(url string) (*http.Response, error)
	Get(url string) (*http.Response, error)
}

type Manager struct {
	hClient HttpHeadGetter
	tmpdir  string
	storage *db.Handle

	mu      sync.Mutex
	records map[string]*record.Record
}

func NewManager(tmpdir string, hClient HttpHeadGetter, storage *db.Handle) *Manager {
	return &Manager{
		hClient: hClient,
		tmpdir:  tmpdir,
		records: map[string]*record.Record{},
		storage: storage,
	}
}

func (m *Manager) LoadFromURL(opkurl string) error {
	record, err := record.FromOPKURL(m.tmpdir, opkurl)
	if err != nil {
		return err
	}
	key := string(record.Hash)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[key]; !ok {
		m.records[key] = record
	}
	return nil
}

func (m *Manager) PersistRecords() (int, error) {
	return m.storage.MultiUpdateRecord(m.records)
}

// SourceList returns a list of URLs of known opk files
func SourceList(markdown string) []string {
	f, err := os.Open(markdown)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	buf, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	mdParser := parser.New()
	node := mdParser.Parse(buf)

	opks := make([]string, 0, 32)
	ast.WalkFunc(node, ast.NodeVisitorFunc(func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		// We look for links in the page that end with .opk
		link, ok := node.(*ast.Link)
		if !ok {
			return ast.GoToNext
		}
		if !bytes.HasSuffix(link.Destination, opkEnd) {
			return ast.GoToNext
		}

		opks = append(opks, string(link.Destination))
		return ast.GoToNext
	}))
	return opks
}
