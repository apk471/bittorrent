package dht

import (
	"math/rand"
	"net"
	"sync"
)

type bucket struct {
	nodes []*nodeInfo
}

type routingTable struct {
	mu      sync.Mutex
	self    nodeID
	buckets [keySize * 8]*bucket
}

func newRoutingTable(self nodeID) *routingTable {
	rt := &routingTable{self: self}
	for i := range rt.buckets {
		rt.buckets[i] = &bucket{}
	}
	return rt
}

func (rt *routingTable) insert(id nodeID, addr *net.UDPAddr) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if id == rt.self {
		return
	}

	b := commonBits(rt.self, id)
	if b >= len(rt.buckets) {
		return
	}
	bk := rt.buckets[b]

	for _, n := range bk.nodes {
		if n.id == id {
			return
		}
	}

	if len(bk.nodes) < bucketK {
		bk.nodes = append(bk.nodes, &nodeInfo{id: id, addr: addr})
		return
	}
}

func (rt *routingTable) findClosest(target nodeID, count int) []*nodeInfo {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var candidates []*nodeInfo
	for _, bk := range rt.buckets {
		for _, n := range bk.nodes {
			candidates = append(candidates, n)
		}
	}

	sortByDistance(candidates, target)

	if len(candidates) > count {
		candidates = candidates[:count]
	}
	return candidates
}

func (rt *routingTable) getAllNodes() []*nodeInfo {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var all []*nodeInfo
	for _, bk := range rt.buckets {
		all = append(all, bk.nodes...)
	}
	return all
}

func (rt *routingTable) size() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	n := 0
	for _, bk := range rt.buckets {
		n += len(bk.nodes)
	}
	return n
}

func sortByDistance(nodes []*nodeInfo, target nodeID) {
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			di := distance(nodes[i].id, target)
			dj := distance(nodes[j].id, target)
			if lessThan(dj, di) {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
}

func lessThan(a, b nodeID) bool {
	for i := range a {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return false
}

func shuffleNodes(nodes []*nodeInfo) {
	rand.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})
}
