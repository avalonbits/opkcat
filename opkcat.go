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
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/avalonbits/opkcat/db"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"gopkg.in/ini.v1"
)

type ModifiedGetter interface {
	GetIfModified(since time.Time, etag, url string) (*http.Response, error)
}

type Manager struct {
	getter  ModifiedGetter
	tmpdir  string
	storage *db.Handle

	mu      sync.Mutex
	records map[string]*db.Record
}

func NewManager(tmpdir string, getter ModifiedGetter, storage *db.Handle) *Manager {
	return &Manager{
		getter:  getter,
		tmpdir:  tmpdir,
		records: map[string]*db.Record{},
		storage: storage,
	}
}

func (m *Manager) LoadFromURL(opkurl string) error {
	record, err := m.fromOPKURL(opkurl)
	if err != nil {
		return err
	}

	// nil record means we already have the data.
	if record == nil {
		return nil
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

func (m *Manager) fromOPKURL(opkurl string) (*db.Record, error) {
	// First we retrieve the last update time for that url
	date, etag, err := m.storage.LastUpdated(opkurl)
	if err != nil {
		return nil, err
	}

	// Now we only retrieve tha opk if it is newer than the current version.
	resp, err := m.getter.GetIfModified(date, etag, opkurl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http fetch error: %v", resp.StatusCode)
	}

	var readEtag string
	if len(resp.Header["Etag"]) > 0 {
		readEtag = resp.Header["Etag"][0]
	}

	// As a last resort, we compare the etags here in case the server didn't respond with a 304.
	if readEtag == etag {
		return nil, nil
	}

	tmpFile, err := ioutil.TempFile(m.tmpdir, "Fopkcat-*-"+url.PathEscape(opkurl))
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return nil, err
	}

	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	return m.fromOPK(tmpFile.Name(), readEtag, opkurl)
}

// FromOPK creates a record by parsing an opkfile. opkurl as added to the the URL field.
func (m *Manager) fromOPK(opkfile, etag, opkurl string) (*db.Record, error) {
	hash, err := fileSHA256(opkfile)
	if err != nil {
		return nil, err
	}

	record := &db.Record{
		Hash: hash,
		URL:  opkurl,
		Date: time.Now().UTC(),
		Etag: etag,
	}

	if err := m.extractOPK(opkfile, record); err != nil {
		return nil, err
	}
	return record, nil
}

// fileSHA256 computes the SHA256 hash of a file.
func fileSHA256(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	to := sha256.New()
	if _, err := io.Copy(to, f); err != nil {
		return nil, err
	}
	return to.Sum(nil), nil
}

// extractOPK opens and pareses the contents of the opk file to create a valid
func (m *Manager) extractOPK(file string, record *db.Record) error {
	dir, err := ioutil.TempDir(m.tmpdir, "Dopkcat-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Unsquash the opk file so we can read its contents.
	finalDir := filepath.Join(dir, url.PathEscape(record.URL))
	cmd := exec.Command("unsquashfs", "-no-xattrs", "-d", finalDir, file)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", out, err)
	}

	// Read and parse the  desktop entries.
	entries, err := filepath.Glob(filepath.Join(finalDir, "*.gcw0.desktop"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fEntry, err := os.Open(entry)
		if err != nil {
			return err
		}
		defer fEntry.Close()

		content, err := ioutil.ReadAll(fEntry)
		if err != nil {
			return err
		}
		entry, err := parseDesktopEntry(content, finalDir)
		if err != nil {
			return err
		}
		record.Entries = append(record.Entries, entry)
	}
	return nil
}

// parseDesktopEntry parses the opk desktop entry file.
// It uses the ini file format.
func parseDesktopEntry(content []byte, dir string) (*db.Entry, error) {
	cfg, err := ini.Load(content)
	if err != nil {
		return nil, err
	}

	// Read the string values from the desktop entry.
	sec, err := cfg.GetSection("Desktop Entry")
	if err != nil {
		return nil, err
	}

	// Read the icon content. It is always a png file.
	icon := sec.Key("Icon").String() + ".png"
	fIcon, err := os.Open(filepath.Join(dir, icon))
	if err != nil {
		return nil, err
	}
	defer fIcon.Close()

	iconData, err := ioutil.ReadAll(fIcon)
	if err != nil {
		return nil, err
	}
	return &db.Entry{
		Name:        sec.Key("Name").String(),
		Type:        sec.Key("Type").String(),
		Description: sec.Key("Comment").String(),
		Categories:  strings.Split(sec.Key("Categories").String(), ";"),
		Icon:        iconData,
	}, nil
}

var opkEnd = []byte(".opk")

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
