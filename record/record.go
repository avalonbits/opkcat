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

// Package record implements a database record that can describe any valid OPK.
package record

import (
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

	"gopkg.in/ini.v1"
)

// Record is the record that can be stored in the database.
type Record struct {
	URL     string
	Hash    []byte
	Entries []*Entry
}

type Entry struct {
	Name        string
	Description string
	Type        string
	Categories  []string
	Icon        []byte
}

func FromOPKURL(opkurl string) (*Record, error) {
	tmpFile, err := ioutil.TempFile("", "Fopkcat-*-"+url.PathEscape(opkurl))
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	resp, err := http.Get(opkurl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return nil, err
	}

	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	return FromOPK(tmpFile.Name(), opkurl)
}

// FromOPK creates a record by parsing an opkfile. opkurl as added to the the record.URL field.
func FromOPK(opkfile, opkurl string) (*Record, error) {
	hash, err := fileSHA256(opkfile)
	if err != nil {
		return nil, err
	}

	record := &Record{
		Hash: hash,
		URL:  opkurl,
	}

	if err := extractOPK(opkfile, record); err != nil {
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

// extractOPK opens and pareses the contents of the opk file to create a valid record.
func extractOPK(file string, record *Record) error {
	dir, err := ioutil.TempDir("", "Dopkcat-*")
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
func parseDesktopEntry(content []byte, dir string) (*Entry, error) {
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
	return &Entry{
		Name:        sec.Key("Name").String(),
		Type:        sec.Key("Type").String(),
		Description: sec.Key("Comment").String(),
		Categories:  strings.Split(sec.Key("Categories").String(), ";"),
		Icon:        iconData,
	}, nil
}
