package dht

import (
	"crypto/rand"
	"encoding/binary"
	"net"
)

const (
	keySize  = 20
	bucketK = 8
)

type nodeID [keySize]byte

func randomNodeID() nodeID {
	var id nodeID
	rand.Read(id[:])
	return id
}

func distance(a, b nodeID) (d nodeID) {
	for i := range a {
		d[i] = a[i] ^ b[i]
	}
	return d
}

func commonBits(a, b nodeID) int {
	d := distance(a, b)
	for i := 0; i < keySize; i++ {
		for bit := 7; bit >= 0; bit-- {
			if d[i]&(1<<uint(bit)) != 0 {
				return i*8 + (7 - bit)
			}
		}
	}
	return keySize * 8
}

type nodeInfo struct {
	id   nodeID
	addr *net.UDPAddr
}

func compactNodeInfo(id nodeID, addr *net.UDPAddr) []byte {
	buf := make([]byte, 26)
	ip := addr.IP.To4()
	copy(buf[0:4], ip)
	binary.BigEndian.PutUint16(buf[4:6], uint16(addr.Port))
	copy(buf[6:26], id[:])
	return buf
}

func parseCompactNodeInfo(data []byte) (nodeID, *net.UDPAddr, bool) {
	if len(data) < 26 {
		return nodeID{}, nil, false
	}
	var id nodeID
	copy(id[:], data[6:26])
	ip := net.IP(data[0:4])
	port := int(binary.BigEndian.Uint16(data[4:6]))
	addr := &net.UDPAddr{IP: ip, Port: port}
	return id, addr, true
}

func compactPeerInfo(addr *net.UDPAddr) []byte {
	buf := make([]byte, 6)
	ip := addr.IP.To4()
	copy(buf[0:4], ip)
	binary.BigEndian.PutUint16(buf[4:6], uint16(addr.Port))
	return buf
}

func parseCompactPeerInfo(data []byte) *net.UDPAddr {
	if len(data) < 6 {
		return nil
	}
	ip := net.IP(data[0:4])
	port := int(binary.BigEndian.Uint16(data[4:6]))
	return &net.UDPAddr{IP: ip, Port: port}
}
