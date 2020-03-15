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

// Package fetcher implements a service that fetches and stores opk records.
package fetcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/avalonbits/opkcat/db"
	"golang.org/x/sync/errgroup"
	"gopkg.in/ini.v1"
)

type ModifiedGetter interface {
	GetIfModified(since time.Time, etag, url string) (*http.Response, error)
}

type Service struct {
	tmpdir     string
	storage    *db.Handle
	getter     ModifiedGetter
	maxFetches int

	quit   chan struct{}
	ticker *time.Ticker
}

func New(tmpdir string, storage *db.Handle, getter ModifiedGetter, maxFetches int) *Service {
	return &Service{
		storage:    storage,
		getter:     getter,
		maxFetches: maxFetches,

		quit:   make(chan struct{}),
		ticker: time.NewTicker(12 * time.Hour),
	}
}

func (s *Service) Add(url string) error {
	return s.storage.IndexURL(url)
}

func (s *Service) Run() error {
	defer close(s.quit)

	// We always run the fetcher on startup.
	runFetch := make(chan bool, 1)
	runFetch <- true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
RUN:
	for {
		select {
		case <-runFetch:
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.Fetch(ctx); err != nil {
					log.Println(err)
				} else {
					log.Println("Done fetching.")
				}
			}()
		case <-s.ticker.C:
			runFetch <- true
		case <-s.quit:
			// Cancel the context
			cancel()

			// We stop the ticker so it won't write after we close the channel
			s.ticker.Stop()
			close(runFetch)

			// Drain the channel. Not sure it is required, but let's do it anyway.
			for _ = range runFetch {
			}

			log.Println("Waiting for fetches to finish.")
			wg.Wait()
			log.Println("Done fetching.")
			break RUN
		}
	}

	return nil
}

func (s *Service) Stop() error {
	log.Println("Stopping fetcher.")
	s.quit <- struct{}{}
	log.Println("Waiting for confirmation.")
	<-s.quit
	log.Println("Done.")
	return nil
}

// Fetch retrieves and stores metadata on each known opk.
func (s *Service) Fetch(ctx context.Context) error {
	var group errgroup.Group
	urlsCh := make(chan *db.URLFreshness, s.maxFetches)

	// To limit the amount of goroutines, we desing the fetcher in the following way:
	// - 1 goroutine reads the known urls and send them over a channel.
	group.Go(func() error {
		defer close(urlsCh)

		urls, err := s.storage.KnownURLs()
		if err != nil {
			return err
		}

	URL_LOOP:
		for _, opkurl := range urls {
			select {
			case <-ctx.Done():
				break URL_LOOP
			case urlsCh <- opkurl:
				break
			}
		}
		return nil
	})

	// - maxFetches goroutines read from the url channel and do the fethcing and record creating.
	var mu sync.Mutex
	records := []*db.Record{}
	for i := 0; i < s.maxFetches; i++ {
		group.Go(func() error {
			for opkurl := range urlsCh {
				log.Println("Processing", opkurl.URL)
				record, err := s.recordFromURL(opkurl)
				if err != nil {
					log.Println(err)
					continue
				}

				if record == nil {
					log.Println(opkurl, "is up-to-date.")
					// The current record is up-to-date, we are done with the url.
					continue
				}

				mu.Lock()
				records = append(records, record)
				mu.Unlock()
			}
			return nil
		})

	}

	if err := group.Wait(); err != nil {
		return err
	}

	log.Println("Will write", len(records), "records")
	// - Once everyone is done, we write the records in a single batch.
	_, err := s.storage.MultiUpdateRecord(records)
	return err
}

func (s *Service) recordFromURL(opkurl *db.URLFreshness) (*db.Record, error) {
	// We only retrieve tha opk if it is newer than the current version.
	resp, err := s.getter.GetIfModified(opkurl.LastUpdate, opkurl.Etag, opkurl.URL)
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
	if readEtag == opkurl.Etag {
		return nil, nil
	}

	tmpFile, err := ioutil.TempFile(s.tmpdir, "Fopkcat-*-"+url.PathEscape(opkurl.URL))
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

	return s.fromOPK(tmpFile.Name(), readEtag, opkurl.URL)
}

// FromOPK creates a record by parsing an opkfile. opkurl as added to the the URL field.
func (s *Service) fromOPK(opkfile, etag, opkurl string) (*db.Record, error) {
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

	if err := s.extractOPK(opkfile, record); err != nil {
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
func (s *Service) extractOPK(file string, record *db.Record) error {
	dir, err := ioutil.TempDir(s.tmpdir, "Dopkcat-*")
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
