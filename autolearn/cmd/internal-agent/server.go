package main

import (
	"context"
	"sync"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/internal-agent/handler"
)

const DefaultSeverPort = "8088"

type Server struct {
	workers   []handler.Worker
	workersWg sync.WaitGroup
}

// NewServer is agent http server
func NewServer() (*Server, error) {
	return &Server{}, nil
}

func (s *Server) RegisterWorker(worker handler.Worker) {
	s.workers = append(s.workers, worker)
}

func (s *Server) StartUp() {
	for _, w := range s.workers {
		w.Start(&s.workersWg)
	}
}

func (s *Server) Shutdown(ctx context.Context) {
	for _, m := range s.workers {
		m.Stop()
	}

	s.workersWg.Wait()
}
