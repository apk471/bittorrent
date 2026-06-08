package peer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// PeerConn is a BitTorrent peer wire-protocol connection.
type PeerConn struct {
	conn       net.Conn
	infoHash   [20]byte
	peerID     [20]byte
	remoteID   [20]byte
	choked     bool
	interested bool
	closed     bool
	mu         sync.Mutex
	bufReader  io.Reader
}

// RemoteID returns the peer's ID as received in the handshake.
func (pc *PeerConn) RemoteID() [20]byte { return pc.remoteID }

// Choked returns true if the peer has choked us.
func (pc *PeerConn) Choked() bool { return pc.choked }

// Interested returns true if we have expressed interest to the peer.
func (pc *PeerConn) Interested() bool { return pc.interested }

// Dial connects to a peer, performs the BitTorrent handshake,
// and returns an established connection.
func Dial(addr string, infoHash, peerID [20]byte, timeout time.Duration) (*PeerConn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("peer: dial %s: %w", addr, err)
	}

	pc := &PeerConn{
		conn:      conn,
		infoHash:  infoHash,
		peerID:    peerID,
		choked:    true,
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

	pc.remoteID = hs.PeerID

	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("peer: clear deadline: %w", err)
	}

	return pc, nil
}

// ReadMessage reads and returns the next message from the peer.
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

// SendMessage sends a message to the peer.
func (pc *PeerConn) SendMessage(msg *Message) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.closed {
		return errors.New("peer: connection closed")
	}
	return SendMessage(pc.conn, msg)
}

// Close closes the peer connection.
func (pc *PeerConn) Close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if !pc.closed {
		pc.closed = true
		pc.conn.Close()
	}
}

// SetReadTimeout sets the read deadline for subsequent reads.
func (pc *PeerConn) SetReadTimeout(d time.Duration) {
	if d > 0 {
		pc.conn.SetReadDeadline(time.Now().Add(d))
	} else {
		pc.conn.SetReadDeadline(time.Time{})
	}
}

// RemoteAddr returns the remote address of the peer.
func (pc *PeerConn) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

// IsClosed returns true if the connection has been closed.
func (pc *PeerConn) IsClosed() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.closed
}
