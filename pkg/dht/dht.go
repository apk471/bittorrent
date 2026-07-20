package dht

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/ayush-amin/bittorrent/pkg/bencode"
	"github.com/ayush-amin/bittorrent/pkg/tracker"
)

// Logger is the interface for DHT logging. Pass nil to disable.
type Logger interface {
	Printf(format string, v ...any)
}

var bootstrapNodes = []string{
	"router.bittorrent.com:6881",
	"dht.transmissionbt.com:6881",
	"dht.aelitis.com:6881",
}

type DHT struct {
	nodeID    nodeID
	table     *routingTable
	conn      *net.UDPConn
	port      int

	mu      sync.Mutex
	pending map[string]*pendingTx
	tokens  map[string]string
	stopCh  chan struct{}

	peerCh     chan<- tracker.Peer
	infoHashes [][20]byte
	log        Logger
}

func New(port int, peerCh chan<- tracker.Peer, log Logger) (*DHT, error) {
	addr := &net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("dht: listen: %w", err)
	}

	id := randomNodeID()

	d := &DHT{
		nodeID:  id,
		table:   newRoutingTable(id),
		conn:    conn,
		port:    conn.LocalAddr().(*net.UDPAddr).Port,
		pending: make(map[string]*pendingTx),
		tokens:  make(map[string]string),
		stopCh:  make(chan struct{}),
		peerCh:  peerCh,
		log:     log,
	}

	return d, nil
}

func (d *DHT) Port() int { return d.port }

func (d *DHT) logf(format string, v ...any) {
	if d.log != nil {
		d.log.Printf(format, v...)
	}
}

func (d *DHT) Run() error {
	go d.incomingKRPC()

	d.bootstrap()
	d.refreshLoop()

	return nil
}

func (d *DHT) bootstrap() {
	d.mu.Lock()
	d.table.insert(d.nodeID, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: d.port})
	d.mu.Unlock()

	var wg sync.WaitGroup
	for _, addr := range bootstrapNodes {
		udpAddr, err := net.ResolveUDPAddr("udp4", addr)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(a *net.UDPAddr) {
			defer wg.Done()
			d.findNode(a, d.nodeID)
		}(udpAddr)
	}
	wg.Wait()
}

func (d *DHT) Stop() {
	close(d.stopCh)
	d.conn.Close()
}

func (d *DHT) refreshLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.queryTimeout()

			nodes := d.table.getAllNodes()
			if len(nodes) == 0 {
				d.bootstrap()
				continue
			}

			shuffleNodes(nodes)
			limit := 8
			if len(nodes) < limit {
				limit = len(nodes)
			}
			for _, n := range nodes[:limit] {
				d.findNode(n.addr, d.nodeID)
			}

			for _, ih := range d.infoHashes {
				d.getPeers(ih)
			}
		}
	}
}

func (d *DHT) findNode(addr *net.UDPAddr, target nodeID) {
	args := bencode.Dict{
		"id":     bencode.String(d.nodeID[:]),
		"target": bencode.String(target[:]),
	}

	tid, err := d.sendQuery(addr, methodFindNode, args)
	if err != nil {
		return
	}

	d.mu.Lock()
	tx := d.pending[tid]
	tx.done = make(chan *message, 1)
	d.mu.Unlock()

	select {
	case resp := <-tx.done:
		if resp.r == nil {
			return
		}
		nodesV, err := resp.r.GetString("nodes")
		if err != nil {
			return
		}
		d.processCompactNodes(string(nodesV))
	case <-time.After(10 * time.Second):
	case <-d.stopCh:
	}
}

func (d *DHT) processCompactNodes(data string) {
	buf := []byte(data)
	for i := 0; i+26 <= len(buf); i += 26 {
		id, addr, ok := parseCompactNodeInfo(buf[i : i+26])
		if !ok {
			continue
		}
		d.mu.Lock()
		d.table.insert(id, addr)
		d.mu.Unlock()
	}
}

func (d *DHT) GetPeers(infoHash [20]byte) {
	d.mu.Lock()
	d.infoHashes = append(d.infoHashes, infoHash)
	d.mu.Unlock()

	d.getPeers(infoHash)
}

func (d *DHT) getPeers(infoHash [20]byte) {
	closest := d.table.findClosest(infoHash, bucketK)
	if len(closest) == 0 {
		return
	}

	limit := 8
	if len(closest) < limit {
		limit = len(closest)
	}
	for _, n := range closest[:limit] {
		if n.id == d.nodeID {
			continue
		}
		d.queryGetPeers(n.addr, infoHash)
	}
}

func (d *DHT) queryGetPeers(addr *net.UDPAddr, infoHash [20]byte) {
	args := bencode.Dict{
		"id":        bencode.String(d.nodeID[:]),
		"info_hash": bencode.String(infoHash[:]),
	}

	tid, err := d.sendQuery(addr, methodGetPeers, args)
	if err != nil {
		return
	}

	d.mu.Lock()
	tx := d.pending[tid]
	tx.done = make(chan *message, 1)
	d.mu.Unlock()

	go func() {
		select {
		case resp := <-tx.done:
			if resp.r == nil {
				return
			}

			if nodesV, err := resp.r.GetString("nodes"); err == nil {
				d.processCompactNodes(string(nodesV))
			}

			if valuesV, err := resp.r.GetList("values"); err == nil {
				d.processPeerValues(valuesV)
			}
		case <-time.After(10 * time.Second):
		case <-d.stopCh:
		}
	}()
}

func (d *DHT) processPeerValues(values bencode.List) {
	for _, item := range values {
		s, ok := item.(bencode.String)
		if !ok {
			continue
		}
		addr := parseCompactPeerInfo([]byte(s))
		if addr == nil {
			continue
		}
		p := tracker.Peer{IP: addr.IP, Port: uint16(addr.Port)}
		select {
		case d.peerCh <- p:
		default:
		}
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
