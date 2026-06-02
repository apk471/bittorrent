package peer

import (
	"fmt"
	"io"
	"net"
)

type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
	Reserved [8]byte
}

const (
	handshakePstr    = "BitTorrent protocol"
	handshakePstrLen = 19
	handshakeSize    = 68
)

func NewHandshake(infoHash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     handshakePstr,
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

func (h *Handshake) Marshal() []byte {
	buf := make([]byte, handshakeSize)
	buf[0] = handshakePstrLen
	copy(buf[1:20], h.Pstr)
	copy(buf[20:28], h.Reserved[:])
	copy(buf[28:48], h.InfoHash[:])
	copy(buf[48:68], h.PeerID[:])
	return buf
}

func (h *Handshake) Unmarshal(data []byte) error {
	if len(data) < handshakeSize {
		return fmt.Errorf("handshake: too short (%d bytes)", len(data))
	}
	if data[0] != handshakePstrLen {
		return fmt.Errorf("handshake: invalid pstr length %d", data[0])
	}
	h.Pstr = string(data[1:20])
	if h.Pstr != handshakePstr {
		return fmt.Errorf("handshake: invalid protocol string %q", h.Pstr)
	}
	copy(h.Reserved[:], data[20:28])
	copy(h.InfoHash[:], data[28:48])
	copy(h.PeerID[:], data[48:68])
	return nil
}

func DoHandshake(conn net.Conn, infoHash, peerID [20]byte) (*Handshake, error) {
	req := NewHandshake(infoHash, peerID)
	data := req.Marshal()

	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("handshake: write: %w", err)
	}

	resp := make([]byte, handshakeSize)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, fmt.Errorf("handshake: read: %w", err)
	}

	var hs Handshake
	if err := hs.Unmarshal(resp); err != nil {
		return nil, fmt.Errorf("handshake: parse: %w", err)
	}

	if hs.InfoHash != infoHash {
		return nil, fmt.Errorf("handshake: info hash mismatch")
	}

	return &hs, nil
}
