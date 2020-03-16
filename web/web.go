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
