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
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
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

	werr error // connection write error
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

	// 2. send SSLRequest Response
	// not support SSL yet
	cc.pkt.Write([]byte{'N'})
	if err := cc.pkt.Flush(); err != nil {
		return err
	}

	// 3. read StartupMessage
	data, err = cc.pkt.ReadStartupMessage()
	if err != nil {
		return err
	}

	version := data[:4]
	log.Debugf("conn %s, version=%X", cc, version)
	params := readKeyValuePairs(data[4:])
	log.Debugf("conn %s, paras=%+v", cc, params)
	cc.user, _ = params["user"]
	if cc.user == "" {
		return errors.New("user not set in StartupMessage")
	}

	cc.dbname, _ = params["database"]
	if cc.dbname == "" {
		cc.dbname = cc.user // The database to connect to. Defaults to the user name.
	}

	// 4. send AuthenticationRequest, only MD5Password supported now
	if err := cc.pkt.WriteMessage(MessageTypeAuthenticationRequest, buildMD5PasswordAuthRequest()); err != nil {
		return err
	}

	// 5. read password message
	tp, password, err := cc.pkt.ReadMessage()
	if err != nil {
		return err
	}

	if tp != MessageTypePasswordMessage {
		return fmt.Errorf("need PasswordMessage, got %d", tp)
	}

	// TODO: 5.1 check password
	// refer to: func (cc *clientConn) openSessionAndDoAuth(authData []byte) error {

	// 6. send auth OK, parameter status, backend key data
	cc.WriteAuthenticationOk()
	cc.WriteParameterStatus("application_name", "psql")
	cc.WriteParameterStatus("client_encoding", "UTF8")
	cc.WriteParameterStatus("DateStyle", "ISO, MDY")
	cc.WriteParameterStatus("integer_datetimes", "on")
	cc.WriteParameterStatus("IntervalStyle", "postgres")
	cc.WriteParameterStatus("is_superuser", "on")
	cc.WriteParameterStatus("server_encoding", "UTF8")
	cc.WriteParameterStatus("server_version", "10.5 (Debian 10.5-1.pgdg90+1)")
	cc.WriteParameterStatus("session_authorization", "postgres")
	cc.WriteParameterStatus("standard_conforming_strings", "on")
	cc.WriteParameterStatus("TimeZone", "GMT")
	cc.WriteBackendKeyData(int32(cc.connectionID), 88) // bye bye :-)
	if err := cc.WriteReadyForQuery(); err != nil {
		return err
	}
	log.Debugf("conn %s, password=%X", cc, password)
	return nil
}

const md5EncryptedPassword uint32 = 5 // Specifies that an MD5-encrypted password is required.

func buildMD5PasswordAuthRequest() []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[:4], md5EncryptedPassword)
	_, err := io.ReadFull(rand.Reader, out[4:]) // salt
	terror.Log(errors.Trace(err))
	return out
}

func readKeyValuePairs(data []byte) map[string]string {
	kvs := make(map[string]string)
	s := bytes.Split(data, []byte{0x00})
	for i := 0; i+1 < len(s); i += 2 {
		k := strings.TrimSpace(string(s[i]))
		if k == "" {
			continue
		}

		kvs[k] = string(s[i+1])
	}

	return kvs
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
	for {
		tp, data, err := cc.pkt.ReadMessage()
		if err != nil {
			log.Errorf("read message err %s, %s", cc, err)
			goto EXIT
		}

		switch tp {
		case MessageTypeSimpleQuery:
			log.Debugf("recv simple query, %s, %s", cc, string(data))
		default:
			log.Debugf("recv unsupported data type, %s, %d, %s", cc, tp, string(data))
		}
	}

EXIT:
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

var zeroInt32 = []byte{0x00, 0x00, 0x00, 0x00}

func (cc *clientConn) WriteAuthenticationOk() error {
	if cc.werr != nil {
		return cc.werr
	}
	cc.werr = cc.pkt.WriteMessage(MessageTypeAuthenticationRequest, zeroInt32)
	return cc.werr
}

func (cc *clientConn) WriteParameterStatus(k, v string) error {
	if cc.werr != nil {
		return cc.werr
	}
	data := make([]byte, 0, len(k)+len(v)+2)
	data = append(data, []byte(k)...)
	data = append(data, 0)
	data = append(data, []byte(v)...)
	data = append(data, 0)
	cc.werr = cc.pkt.WriteMessage(MessageTypeParameterStatus, data)
	return cc.werr
}

// WriteBackendKeyData write key data
// Identifies the message as cancellation key data. The frontend must save these values if it wishes to be able to issue CancelRequest messages later.
func (cc *clientConn) WriteBackendKeyData(processID, secretKey int32) error {
	if cc.werr != nil {
		return cc.werr
	}

	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], uint32(processID))
	binary.BigEndian.PutUint32(data[4:], uint32(secretKey))
	cc.werr = cc.pkt.WriteMessage(MessageTypeBackendKeyData, data)
	return cc.werr
}

func (cc *clientConn) WriteReadyForQuery() error {
	if cc.werr != nil {
		return cc.werr
	}
	cc.pkt.WriteMessage(MessageTypeReadyForQuery, []byte{'I'})
	cc.werr = cc.pkt.Flush()
	return cc.werr
}
