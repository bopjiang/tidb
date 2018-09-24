package pgserver

import (
	"github.com/pingcap/tidb/config"
	"github.com/pingcap/tidb/server"
	log "github.com/sirupsen/logrus"
)

// Server is the PostgreSQL protocol server
type Server struct {
}

func NewServer(cfg *config.Config, driver server.IDriver) (*Server, error) {
	s := &Server{}
	return s, nil
}

// Run runs the server.
func (s *Server) Run() error {
	log.Info("pg server started...")
	return nil
}
