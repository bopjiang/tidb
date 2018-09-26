// Copyright 2013 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// The MIT License (MIT)
//
// Copyright (c) 2014 wandoulabs
// Copyright (c) 2014 siddontang
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package pgserver

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/pingcap/tidb/metrics"
	"github.com/pingcap/tidb/mysql"
	"github.com/pingcap/tidb/server"
	"github.com/pingcap/tidb/terror"
	"github.com/pingcap/tidb/util/arena"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
	pkt          *packetIO
	server       *Server         // a reference of pg server instance.
	connectionID uint32          // atomically allocated by a global variable, unique in process scope.
	collation    uint8           // collation used by client, may be different from the collation used by database.
	user         string          // user of the client.
	dbname       string          // default database name.
	alloc        arena.Allocator // an memory allocator for reducing memory allocation.
	status       int32           // dispatching/reading/shutdown/waitshutdown
	ctx          server.QueryCtx // an interface to execute sql statements.
	lastCmd      string          // latest sql query string, currently used for logging error.
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
	cc.pkt = newPacketIO(conn)
}

// Int32(80877103)
// The SSL request code.
// The value is chosen to contain 1234 in the most significant 16 bits, and 5679 in the least significant 16 bits. (To avoid confusion, this code must not be the same as any protocol version number.)
var SSLRequestCode = []byte{0x04, 0xd2, 0x16, 0x2f}

func (cc *clientConn) handshake() error {
	// 1. read SSLRequest
	data, err := cc.pkt.ReadStartupMessage()
	if err != nil {
		return err
	}

	if !bytes.Equal(data, SSLRequestCode) {
		return fmt.Errorf("read wrong SSLRequestCode,%X", data)
	}

	// 2. write SSLRequest Response
	// not support SSL yet
	if err := cc.pkt.Write([]byte{0x4e}); err != nil { // N
		return err
	}

	// 3. read StartupMessage
	data, err = cc.pkt.ReadStartupMessage()
	if err != nil {
		return err
	}

	version := data[:4]
	log.Debugf("conn %s, version=%X", cc, version)

	return nil
}

func readKeyValuePair(data []byte) map[string]string {
	return nil
}

func (cc *clientConn) Run() {
	const size = 4096
	closedOutside := false
	defer func() {
		r := recover()
		if r != nil {
			buf := make([]byte, size)
			stackSize := runtime.Stack(buf, false)
			buf = buf[:stackSize]
			log.Errorf("lastCmd %s, %v, %s", cc.lastCmd, r, buf)
			metrics.PanicCounter.WithLabelValues(metrics.LabelSession).Inc()
		}
		if !closedOutside {
			err := cc.Close()
			terror.Log(errors.Trace(err))
		}
	}()

}

func (cc *clientConn) Close() error {
	cc.server.rwlock.Lock()
	delete(cc.server.clients, cc.connectionID)
	connections := len(cc.server.clients)
	cc.server.rwlock.Unlock()
	metrics.ConnGauge.Set(float64(connections))
	err := cc.pkt.Close()
	terror.Log(errors.Trace(err))
	if cc.ctx != nil {
		return cc.ctx.Close()
	}
	return nil
}

func (cc *clientConn) String() string {
	return fmt.Sprintf("id:%d, addr:%s user:%s",
		cc.connectionID, cc.pkt.conn.RemoteAddr(), cc.user,
	)
}
