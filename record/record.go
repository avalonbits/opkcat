// Package record implements a database record that can describe any valid OPK.
package record

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

// Record is the record that can be stored in the database.
type Record struct {
	Name        string
	Description string
	Type        string
	Category    []string
	URL         string
	Hash        []byte
	Icon        []byte
}

func FromOPKURL(opkurl string) (*Record, error) {
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

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
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Unsquash the opk file so we can read its contents.
	finalDir := fmt.Sprintf("%s/%x", dir, record.Hash)
	cmd := exec.Command("unsquashfs", "-d", finalDir, file)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Read and parse the default desktop entry file.
	entry := filepath.Join(finalDir, "default.gcw0.desktop")
	fEntry, err := os.Open(entry)
	if err != nil {
		return err
	}
	defer fEntry.Close()

	content, err := ioutil.ReadAll(fEntry)
	if err != nil {
		return err
	}
	return parseDesktopEntry(content, finalDir, record)
}

// parseDesktopEntry parses the opk desktop entry file.
// It uses the ini file format.
func parseDesktopEntry(content []byte, dir string, record *Record) error {
	cfg, err := ini.Load(content)
	if err != nil {
		return err
	}

	// Read the string values from the desktop entry.
	sec, err := cfg.GetSection("Desktop Entry")
	if err != nil {
		return nil
	}
	record.Name = sec.Key("Name").String()
	record.Type = sec.Key("Type").String()
	record.Description = sec.Key("Comment").String()
	record.Category = strings.Split(sec.Key("Categories").String(), ";")

	// Read the icon content. It is always a png file.
	icon := sec.Key("Icon").String() + ".png"
	fIcon, err := os.Open(filepath.Join(dir, icon))
	if err != nil {
		return err
	}
	defer fIcon.Close()

	iconData, err := ioutil.ReadAll(fIcon)
	if err != nil {
		return err
	}
	record.Icon = iconData
	return nil
}
