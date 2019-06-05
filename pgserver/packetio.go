package pgserver

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
)

const (
	defaultReaderSize = 16 * 1024
	defaultWriterSize = 16 * 1024
)

// packetIO is a helper to read and write data in packet format.
type packetIO struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func newPacketIO(conn net.Conn) *packetIO {
	return &packetIO{
		conn: conn,
		r:    bufio.NewReaderSize(conn, defaultReaderSize),
		w:    bufio.NewWriterSize(conn, defaultWriterSize),
	}
}

// ReadStartupMessage reads startup message
// format:
// 4 bytes, length of message contents in bytes, including self
// length-4 bytes: data
func (pkt *packetIO) ReadStartupMessage() ([]byte, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(pkt.r, buf); err != nil {
		return nil, err
	}

	len := binary.BigEndian.Uint32(buf)
	data := make([]byte, len-4)
	if _, err := io.ReadFull(pkt.r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// ReadMessage reads normal message
// format:
// 1 byte, message type
// 4 bytes, length of message contents in bytes, including self
// length-4 bytes: data
func (pkt *packetIO) ReadMessage() (MessageType, []byte, error) {
	buf := make([]byte, 5)
	if _, err := io.ReadFull(pkt.r, buf); err != nil {
		return MessageTypeInvalid, nil, err
	}

	tp := MessageType(buf[0])
	len := binary.BigEndian.Uint32(buf[1:])
	data := make([]byte, len-4)
	if _, err := io.ReadFull(pkt.r, data); err != nil {
		return MessageTypeInvalid, nil, err
	}

	return tp, data, nil
}

func (pkt *packetIO) WriteMessage(tp MessageType, data []byte) error {
	pkt.w.WriteByte(byte(tp))
	binary.Write(pkt.w, binary.BigEndian, uint32(len(data)+4))
	if len(data) > 0 {
		pkt.w.Write(data)
	}
	return pkt.w.Flush()
}

func (pkt *packetIO) Write(p []byte) (int, error) {
	return pkt.w.Write(p)
}

func (pkt *packetIO) Flush() error {
	return pkt.w.Flush()
}

func (pkt *packetIO) Close() error {
	return pkt.conn.Close()
}
