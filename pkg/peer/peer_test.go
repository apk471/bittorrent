package peer

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"testing"
	"time"
)

func randHash() [20]byte {
	var h [20]byte
	rand.Read(h[:])
	return h
}

func TestHandshakeMarshalUnmarshal(t *testing.T) {
	infoHash := randHash()
	peerID := randHash()

	hs := NewHandshake(infoHash, peerID)
	data := hs.Marshal()

	var parsed Handshake
	if err := parsed.Unmarshal(data); err != nil {
		t.Fatal(err)
	}

	if parsed.Pstr != "BitTorrent protocol" {
		t.Fatalf("expected protocol string, got %q", parsed.Pstr)
	}
	if parsed.InfoHash != infoHash {
		t.Fatalf("info hash mismatch")
	}
	if parsed.PeerID != peerID {
		t.Fatalf("peer ID mismatch")
	}
}

func TestHandshakeShortData(t *testing.T) {
	var hs Handshake
	if err := hs.Unmarshal([]byte{0, 1, 2}); err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestHandshakeWrongPstr(t *testing.T) {
	data := make([]byte, 68)
	data[0] = 19
	copy(data[1:], "FakeProtocol")
	var hs Handshake
	if err := hs.Unmarshal(data); err == nil {
		t.Fatal("expected error for wrong protocol string")
	}
}

func runHandshakeServer(t *testing.T, ln net.Listener, respondWith func(Handshake) *Handshake) {
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 68)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}

		var req Handshake
		if err := req.Unmarshal(buf); err != nil {
			return
		}

		resp := respondWith(req)
		conn.Write(resp.Marshal())
	}()
}

func TestDoHandshake(t *testing.T) {
	infoHash := randHash()
	peerID := randHash()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	runHandshakeServer(t, ln, func(req Handshake) *Handshake {
		return NewHandshake(req.InfoHash, randHash())
	})

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs, err := DoHandshake(conn, infoHash, peerID)
	if err != nil {
		t.Fatal(err)
	}
	if hs.InfoHash != infoHash {
		t.Fatal("info hash mismatch in response")
	}
}

func TestDoHandshakeWrongInfoHash(t *testing.T) {
	infoHash := randHash()
	peerID := randHash()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	runHandshakeServer(t, ln, func(req Handshake) *Handshake {
		return NewHandshake(randHash(), randHash())
	})

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = DoHandshake(conn, infoHash, peerID)
	if err == nil {
		t.Fatal("expected error for wrong info hash")
	}
}

func TestMessageChoke(t *testing.T) {
	msg := &Message{ID: MsgChoke}
	data := msg.Marshal()
	parsed, err := ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID != MsgChoke {
		t.Fatalf("expected choke, got %s", parsed.ID)
	}
}

func TestMessageUnchoke(t *testing.T) {
	msg := &Message{ID: MsgUnchoke}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgUnchoke {
		t.Fatal("expected unchoke")
	}
}

func TestMessageInterested(t *testing.T) {
	msg := &Message{ID: MsgInterested}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgInterested {
		t.Fatal("expected interested")
	}
}

func TestMessageHave(t *testing.T) {
	msg := &Message{ID: MsgHave, Index: 42}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgHave || parsed.Index != 42 {
		t.Fatalf("expected have index=42, got index=%d", parsed.Index)
	}
}

func TestMessageRequest(t *testing.T) {
	msg := &Message{ID: MsgRequest, Index: 5, Begin: 16384, Length: 16384}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgRequest || parsed.Index != 5 || parsed.Begin != 16384 || parsed.Length != 16384 {
		t.Fatalf("request mismatch: index=%d begin=%d length=%d", parsed.Index, parsed.Begin, parsed.Length)
	}
}

func TestMessageCancel(t *testing.T) {
	msg := &Message{ID: MsgCancel, Index: 3, Begin: 0, Length: 16384}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgCancel || parsed.Index != 3 {
		t.Fatal("cancel mismatch")
	}
}

func TestMessagePiece(t *testing.T) {
	payload := []byte("hello piece data")
	msg := &Message{ID: MsgPiece, Index: 1, Begin: 0, Payload: payload}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgPiece || parsed.Index != 1 || parsed.Begin != 0 {
		t.Fatal("piece header mismatch")
	}
	if !bytes.Equal(parsed.Payload, payload) {
		t.Fatal("piece payload mismatch")
	}
}

func TestMessageBitfield(t *testing.T) {
	bitfield := []byte{0xFF, 0x00, 0xAA}
	msg := &Message{ID: MsgBitfield, Payload: bitfield}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgBitfield {
		t.Fatal("expected bitfield")
	}
	if !bytes.Equal(parsed.Payload, bitfield) {
		t.Fatal("bitfield payload mismatch")
	}
}

func TestMessagePort(t *testing.T) {
	msg := &Message{ID: MsgPort, Index: 6881}
	data := msg.Marshal()
	parsed, _ := ReadMessage(bytes.NewReader(data))
	if parsed.ID != MsgPort || parsed.Index != 6881 {
		t.Fatal("port mismatch")
	}
}

func TestKeepAlive(t *testing.T) {
	msg, err := ReadMessage(bytes.NewReader([]byte{0, 0, 0, 0}))
	if err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		t.Fatal("expected nil for keep-alive")
	}
}

func TestDialTimeout(t *testing.T) {
	_, err := Dial("127.0.0.1:1", randHash(), randHash(), 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for unreachable peer")
	}
}

func TestDialAndHandshake(t *testing.T) {
	infoHash := randHash()
	peerID := randHash()
	remoteID := randHash()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 68)
		io.ReadFull(conn, buf)
		var req Handshake
		req.Unmarshal(buf)
		resp := NewHandshake(req.InfoHash, remoteID)
		conn.Write(resp.Marshal())
	}()

	pc, err := Dial(ln.Addr().String(), infoHash, peerID, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	if pc.RemoteID() != remoteID {
		t.Fatalf("expected remoteID %x, got %x", remoteID, pc.RemoteID())
	}
}

func TestSendReceiveMessage(t *testing.T) {
	infoHash := randHash()
	peerID := randHash()
	remoteID := randHash()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type serverMsg struct {
		msg *Message
		err error
	}
	msgCh := make(chan serverMsg, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 68)
		io.ReadFull(conn, buf)

		resp := NewHandshake(infoHash, remoteID)
		conn.Write(resp.Marshal())

		msg, err := ReadMessage(conn)
		msgCh <- serverMsg{msg, err}
	}()

	pc, err := Dial(ln.Addr().String(), infoHash, peerID, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	pc.SendMessage(&Message{ID: MsgInterested})

	result := <-msgCh
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.msg.ID != MsgInterested {
		t.Fatalf("expected interested, got %s", result.msg.ID)
	}
}
