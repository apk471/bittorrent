package dht

import (
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"github.com/ayush-amin/bittorrent/pkg/bencode"
)

type messageType byte

const (
	msgQuery messageType = 'q'
	msgReply messageType = 'r'
	msgError messageType = 'e'
)

type queryMethod string

const (
	methodPing        queryMethod = "ping"
	methodFindNode    queryMethod = "find_node"
	methodGetPeers    queryMethod = "get_peers"
	methodAnnouncePeer queryMethod = "announce_peer"
)

type message struct {
	t          string
	y          messageType
	q          queryMethod
	a          bencode.Dict
	r          bencode.Dict
	e          bencode.List
	remoteAddr *net.UDPAddr
}

func newTransactionID() string {
	buf := make([]byte, 2)
	rand.Read(buf)
	return fmt.Sprintf("%x", buf)
}

func (d *DHT) sendQuery(addr *net.UDPAddr, method queryMethod, args bencode.Dict) (string, error) {
	t := newTransactionID()

	req := bencode.Dict{
		"t": bencode.String(t),
		"y": bencode.String(string(msgQuery)),
		"q": bencode.String(string(method)),
		"a": args,
	}

	data, err := bencode.EncodeBytes(req)
	if err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}

	d.mu.Lock()
	d.pending[t] = &pendingTx{
		addr:   addr,
		method: method,
		args:   args,
		sent:   time.Now(),
	}
	d.mu.Unlock()

	if _, err := d.conn.WriteToUDP(data, addr); err != nil {
		d.mu.Lock()
		delete(d.pending, t)
		d.mu.Unlock()
		return "", fmt.Errorf("send: %w", err)
	}

	return t, nil
}

func unmarshalResponse(data []byte) (*message, error) {
	v, err := bencode.DecodeBytes(data)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	root, ok := v.(bencode.Dict)
	if !ok {
		return nil, fmt.Errorf("not a dict")
	}

	msg := &message{}

	if tv, err := root.GetString("t"); err == nil {
		msg.t = string(tv)
	}

	if yv, err := root.GetString("y"); err == nil {
		msg.y = messageType(yv[0])
	}

	switch msg.y {
	case msgQuery:
		if qv, err := root.GetString("q"); err == nil {
			msg.q = queryMethod(qv)
		}
		if av, err := root.GetDict("a"); err == nil {
			msg.a = av
		}
	case msgReply:
		if rv, err := root.GetDict("r"); err == nil {
			msg.r = rv
		}
	case msgError:
		if ev, err := root.GetList("e"); err == nil {
			msg.e = ev
		}
	}

	return msg, nil
}

type pendingTx struct {
	addr   *net.UDPAddr
	method queryMethod
	args   bencode.Dict
	sent   time.Time
	done   chan *message
}

func (d *DHT) handleResponse(msg *message) {
	d.mu.Lock()
	tx, ok := d.pending[msg.t]
	if ok {
		delete(d.pending, msg.t)
	}
	d.mu.Unlock()

	if !ok || tx.done == nil {
		return
	}

	select {
	case tx.done <- msg:
	default:
	}
}

func (d *DHT) handleQuery(msg *message, addr *net.UDPAddr) {
	switch msg.q {
	case methodPing:
		d.replyPing(msg.t, addr)
	case methodFindNode:
		targetV, _ := msg.a.GetString("target")
		d.replyFindNode(msg.t, string(targetV), addr)
	case methodGetPeers:
		hashV, _ := msg.a.GetString("info_hash")
		d.replyGetPeers(msg.t, string(hashV), addr)
	case methodAnnouncePeer:
		d.replyAnnouncePeer(msg.t, addr)
	}
}

func (d *DHT) reply(t string, resp bencode.Dict, addr *net.UDPAddr) {
	reply := bencode.Dict{
		"t": bencode.String(t),
		"y": bencode.String(string(msgReply)),
		"r": resp,
	}

	data, err := bencode.EncodeBytes(reply)
	if err != nil {
		return
	}

	d.conn.WriteToUDP(data, addr)
}

func (d *DHT) replyPing(t string, addr *net.UDPAddr) {
	d.reply(t, bencode.Dict{"id": bencode.String(d.nodeID[:])}, addr)
}

func (d *DHT) replyFindNode(t string, target string, addr *net.UDPAddr) {
	var targetID nodeID
	copy(targetID[:], target)

	nodes := d.table.findClosest(targetID, bucketK)
	compact := make([]byte, 0, len(nodes)*26)
	for _, n := range nodes {
		compact = append(compact, compactNodeInfo(n.id, n.addr)...)
	}

	d.reply(t, bencode.Dict{
		"id":    bencode.String(d.nodeID[:]),
		"nodes": bencode.String(compact),
	}, addr)
}

func (d *DHT) replyGetPeers(t string, infoHash string, addr *net.UDPAddr) {
	var hash [20]byte
	copy(hash[:], infoHash)

	d.mu.Lock()
	token, ok := d.tokens[string(hash[:])]
	if !ok {
		token = newTransactionID()
		d.tokens[string(hash[:])] = token
	}
	d.mu.Unlock()

	nodes := d.table.findClosest(hash, bucketK)
	compact := make([]byte, 0, len(nodes)*26)
	for _, n := range nodes {
		compact = append(compact, compactNodeInfo(n.id, n.addr)...)
	}

	d.reply(t, bencode.Dict{
		"id":    bencode.String(d.nodeID[:]),
		"token": bencode.String(token),
		"nodes": bencode.String(compact),
	}, addr)
}

func (d *DHT) replyAnnouncePeer(t string, addr *net.UDPAddr) {
	d.reply(t, bencode.Dict{"id": bencode.String(d.nodeID[:])}, addr)
}

func (d *DHT) queryTimeout() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for t, tx := range d.pending {
		if now.Sub(tx.sent) > 15*time.Second {
			delete(d.pending, t)
		}
	}
}

func (d *DHT) incomingKRPC() {
	buf := make([]byte, 65536)
	for {
		n, addr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		go d.processIncoming(data, addr)
	}
}

func (d *DHT) processIncoming(data []byte, addr *net.UDPAddr) {
	msg, err := unmarshalResponse(data)
	if err != nil || msg == nil {
		return
	}

	d.mu.Lock()
	d.table.insert(nodeIDFromMsg(msg), addr)
	d.mu.Unlock()

	switch msg.y {
	case msgReply, msgError:
		d.handleResponse(msg)
	case msgQuery:
		d.handleQuery(msg, addr)
	}
}

func nodeIDFromMsg(msg *message) nodeID {
	var id nodeID
	var raw string
	switch msg.y {
	case msgQuery:
		if v, err := msg.a.GetString("id"); err == nil {
			raw = string(v)
		}
	case msgReply:
		if v, err := msg.r.GetString("id"); err == nil {
			raw = string(v)
		}
	}
	if len(raw) >= keySize {
		copy(id[:], raw)
	}
	return id
}
