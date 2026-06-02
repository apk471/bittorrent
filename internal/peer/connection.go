package peer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type PeerConn struct {
	conn       net.Conn
	InfoHash   [20]byte
	PeerID     [20]byte
	RemoteID   [20]byte
	Choked     bool
	Interested bool
	closed     bool
	mu         sync.Mutex
	bufReader  io.Reader
}

func Dial(addr string, infoHash, peerID [20]byte, timeout time.Duration) (*PeerConn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("peer: dial %s: %w", addr, err)
	}

	pc := &PeerConn{
		conn:     conn,
		InfoHash: infoHash,
		PeerID:   peerID,
		Choked:   true,
		bufReader: conn,
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("peer: set deadline: %w", err)
	}

	hs, err := DoHandshake(conn, infoHash, peerID)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("peer: handshake: %w", err)
	}

	pc.RemoteID = hs.PeerID

	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("peer: clear deadline: %w", err)
	}

	return pc, nil
}

func (pc *PeerConn) ReadMessage() (*Message, error) {
	pc.mu.Lock()
	r := pc.bufReader
	pc.mu.Unlock()

	msg, err := ReadMessage(r)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (pc *PeerConn) SendMessage(msg *Message) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.closed {
		return errors.New("peer: connection closed")
	}
	return SendMessage(pc.conn, msg)
}

func (pc *PeerConn) Close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if !pc.closed {
		pc.closed = true
		pc.conn.Close()
	}
}

func (pc *PeerConn) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

func (pc *PeerConn) IsClosed() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.closed
}
