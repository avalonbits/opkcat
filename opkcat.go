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
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

type StartStopper interface {
	Start() error
	Stop() error
}

type ServiceManager struct {
	services []StartStopper
	wg       sync.WaitGroup
}

func NewServiceManager(services []StartStopper) *ServiceManager {
	return &ServiceManager{
		services: services,
	}
}

func (sm *ServiceManager) Run() error {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, os.Kill)

	quit := make(chan bool)
	errs := make([]error, len(sm.services))
	for idx, s := range sm.services {
		sm.wg.Add(1)
		go func(idx int, ss StartStopper) {
			defer sm.wg.Done()
			go func() {
				if err := ss.Start(); err != nil {
					panic(err)
				}
			}()
			<-quit
			if err := ss.Stop(); err != nil {
				errs[idx] = err
			}
		}(idx, s)
	}
	<-sigC
	log.Println("Signalling quit.")
	close(quit)

	log.Println("Waiting for everyone to finish")
	sm.wg.Wait()

	log.Println("Checking for errors.")
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	log.Println("Service manager is done.")
	return nil
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
