package pgserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	proxyprotocol "github.com/blacktear23/go-proxyprotocol"
	"github.com/pingcap/tidb/config"
	"github.com/pingcap/tidb/metrics"
	"github.com/pingcap/tidb/server"
	"github.com/pingcap/tidb/terror"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Server is the PostgreSQL protocol server
type Server struct {
	cfg       *config.Config
	tlsConfig *tls.Config
	driver    server.IDriver
	listener  net.Listener
	rwlock    *sync.RWMutex
	clients   map[uint32]*clientConn
}

// NewServer creates a PostgreSQL protocol server
func NewServer(cfg *config.Config, driver server.IDriver) (*Server, error) {
	s := &Server{
		cfg:    cfg,
		driver: driver,
	}

	var err error
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.PgPort)
	if s.listener, err = net.Listen("tcp", addr); err == nil {
		log.Infof("Server is running PostgreSQ Protocol at [%s]", addr)
	}

	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO: Proxy server
	log.Infof("Server run PostgreSQL Protocol Listen at [%s]", addr)

	return s, nil
}

// Run runs the server.
func (s *Server) Run() error {
	log.Info("pg server started...")
	metrics.ServerEventCounter.WithLabelValues(metrics.EventStart).Inc()

	// startStatusHTTP already started in MySQL server modula
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Err.Error() == "use of closed network connection" {
					return nil
				}
			}

			// If we got PROXY protocol error, we should continue accept.
			if proxyprotocol.IsProxyProtocolError(err) {
				log.Errorf("PROXY protocol error: %s", err.Error())
				continue
			}

			log.Errorf("accept error %s", err.Error())
			return errors.Trace(err)
		}

		go s.onConn(conn)
	}
	err := s.listener.Close()
	terror.Log(errors.Trace(err))
	s.listener = nil
	for {
		metrics.ServerEventCounter.WithLabelValues(metrics.EventHang).Inc()
		log.Errorf("pgserver listener stopped, waiting for manual kill.")
		time.Sleep(time.Minute)
	}
}

func (s *Server) onConn(c net.Conn) {
	log.Debugf("pg new connection, %s->%s", c.RemoteAddr(), c.LocalAddr())
	conn := s.newConn(c)
	if err := conn.handshake(); err != nil {
		// Some keep alive services will send request to TiDB and disconnect immediately.
		// So we only record metrics.
		metrics.HandShakeErrorCounter.Inc()
		err = c.Close()
		terror.Log(errors.Trace(err))
		return
	}
	log.Infof("con:%d new connection %s", conn.connectionID, c.RemoteAddr().String())
	defer func() {
		log.Infof("con:%d close connection", conn.connectionID)
	}()
	s.rwlock.Lock()
	s.clients[conn.connectionID] = conn
	connections := len(s.clients)
	s.rwlock.Unlock()
	metrics.ConnGauge.Set(float64(connections))

	conn.Run()
}

func (s *Server) newConn(conn net.Conn) *clientConn {
	cc := newClientConn(s)
	if s.cfg.Performance.TCPKeepAlive {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				log.Error("failed to set tcp keep alive option:", err)
			}
		}
	}
	cc.setConn(conn)
	return cc
}
