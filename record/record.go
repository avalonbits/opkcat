// Package record implements a database record that can describe any valid OPK.
package record

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

// Record is the record that can be stored in the database.
type Record struct {
	Name        string
	Description string
	Cateogry    string
	URL         string
	Hash        []byte
	Icon        []byte
}

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

func extractOPK(file string, record *Record) error {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	finalDir := fmt.Sprintf("%s/%x", dir, record.Hash)
	cmd := exec.Command("unsquashfs", "-d", finalDir, file)
	if err := cmd.Run(); err != nil {
		return err
	}

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
	fmt.Println(string(content))
	return nil
}
