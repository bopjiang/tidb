package pgserver

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pingcap/tidb/mysql"
	"github.com/pingcap/tidb/util/arena"
)

// TODO: baseConnID,connStatus: share with mysql server?
var (
	baseConnID uint32
)

const (
	connStatusDispatching int32 = iota
	connStatusReading
	connStatusShutdown     // Closed by server.
	connStatusWaitShutdown // Notified by server to close.
)

type clientConn struct {
	bufReadConn  *bufferedReadConn // a buffered-read net.Conn or buffered-read tls.Conn.
	server       *Server           // a reference of pg server instance.
	connectionID uint32            // atomically allocated by a global variable, unique in process scope.
	collation    uint8             // collation used by client, may be different from the collation used by database.
	user         string            // user of the client.
	dbname       string            // default database name.
	alloc        arena.Allocator   // an memory allocator for reducing memory allocation.
	status       int32             // dispatching/reading/shutdown/waitshutdown

	// mu is used for cancelling the execution of current transaction.
	mu struct {
		sync.RWMutex
		cancelFunc context.CancelFunc
	}
}

// newClientConn creates a *clientConn object.
func newClientConn(s *Server) *clientConn {
	return &clientConn{
		server:       s,
		connectionID: atomic.AddUint32(&baseConnID, 1),
		collation:    mysql.DefaultCollationID,
		alloc:        arena.NewAllocator(32 * 1024),
		status:       connStatusDispatching,
	}
}

func (cc *clientConn) setConn(conn net.Conn) {
	cc.bufReadConn = newBufferedReadConn(conn)
}

func (cc *clientConn) handshake() error {
	return nil
}

func (cc *clientConn) Run() {

}
