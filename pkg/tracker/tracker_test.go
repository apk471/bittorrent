package tracker

import (
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ayushamin/bittorent/pkg/bencode"
)

func makeCompactPeers(peers []Peer) []byte {
	data := make([]byte, len(peers)*6)
	for i, p := range peers {
		ip4 := p.IP.To4()
		copy(data[i*6:], ip4)
		binary.BigEndian.PutUint16(data[i*6+4:], p.Port)
	}
	return data
}

func TestParseCompactPeers(t *testing.T) {
	peers := []Peer{
		{IP: net.ParseIP("192.168.1.1"), Port: 6881},
		{IP: net.ParseIP("10.0.0.1"), Port: 6882},
	}
	data := makeCompactPeers(peers)

	parsed, err := parseCompactPeers(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(parsed))
	}
	if !parsed[0].IP.Equal(net.ParseIP("192.168.1.1")) {
		t.Fatalf("expected IP 192.168.1.1, got %s", parsed[0].IP)
	}
	if parsed[0].Port != 6881 {
		t.Fatalf("expected port 6881, got %d", parsed[0].Port)
	}
	if !parsed[1].IP.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("expected IP 10.0.0.1, got %s", parsed[1].IP)
	}
	if parsed[1].Port != 6882 {
		t.Fatalf("expected port 6882, got %d", parsed[1].Port)
	}
}

func TestParseCompactPeersInvalidLength(t *testing.T) {
	_, err := parseCompactPeers([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for invalid data length")
	}
}

func TestParseDictPeer(t *testing.T) {
	d := bencode.Dict{
		"ip":   bencode.String("10.0.0.1"),
		"port": bencode.Int(6881),
	}
	peer, err := parseDictPeer(d)
	if err != nil {
		t.Fatal(err)
	}
	if !peer.IP.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("expected IP 10.0.0.1, got %s", peer.IP)
	}
	if peer.Port != 6881 {
		t.Fatalf("expected port 6881, got %d", peer.Port)
	}
}

func TestGeneratePeerID(t *testing.T) {
	id := generatePeerID()
	if len(id) != 20 {
		t.Fatalf("expected 20 bytes, got %d", len(id))
	}
	if string(id[:8]) != "-GO0001-" {
		t.Fatalf("expected prefix -GO0001-, got %q", string(id[:8]))
	}
}

func TestAnnounceCompactResponse(t *testing.T) {
	peers := []Peer{
		{IP: net.ParseIP("1.2.3.4"), Port: 6881},
		{IP: net.ParseIP("5.6.7.8"), Port: 6882},
	}
	compactData := makeCompactPeers(peers)
	respDict := bencode.Dict{
		"interval":   bencode.Int(1800),
		"complete":   bencode.Int(10),
		"incomplete": bencode.Int(2),
		"peers":      bencode.String(compactData),
	}
	respData, _ := bencode.EncodeBytes(respDict)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(respData)
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{
		Port:    6881,
		Left:    1000,
		NumWant: 50,
	}
	resp, err := client.Announce(server.URL, req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Interval != 1800 {
		t.Fatalf("expected interval 1800, got %d", resp.Interval)
	}
	if resp.Complete != 10 {
		t.Fatalf("expected complete 10, got %d", resp.Complete)
	}
	if resp.Incomplete != 2 {
		t.Fatalf("expected incomplete 2, got %d", resp.Incomplete)
	}
	if len(resp.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(resp.Peers))
	}
	if !resp.Peers[0].IP.Equal(net.ParseIP("1.2.3.4")) || resp.Peers[0].Port != 6881 {
		t.Fatal("first peer mismatch")
	}
	if !resp.Peers[1].IP.Equal(net.ParseIP("5.6.7.8")) || resp.Peers[1].Port != 6882 {
		t.Fatal("second peer mismatch")
	}
}

func TestAnnounceDictPeers(t *testing.T) {
	respDict := bencode.Dict{
		"interval":   bencode.Int(600),
		"complete":   bencode.Int(5),
		"incomplete": bencode.Int(1),
		"peers": bencode.List{
			bencode.Dict{
				"ip":   bencode.String("192.168.0.1"),
				"port": bencode.Int(7000),
			},
			bencode.Dict{
				"ip":   bencode.String("192.168.0.2"),
				"port": bencode.Int(7001),
			},
		},
	}
	respData, _ := bencode.EncodeBytes(respDict)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(respData)
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{Port: 6881, Left: 500}
	resp, err := client.Announce(server.URL, req)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(resp.Peers))
	}
}

func TestAnnounceFailure(t *testing.T) {
	respDict := bencode.Dict{
		"failure reason": bencode.String("torrent not found"),
	}
	respData, _ := bencode.EncodeBytes(respDict)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(respData)
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{Port: 6881, Left: 100}
	_, err := client.Announce(server.URL, req)
	if err == nil {
		t.Fatal("expected failure error")
	}
}

func TestAnnounceHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{Port: 6881, Left: 100}
	_, err := client.Announce(server.URL, req)
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestAnnounceEmptyPeers(t *testing.T) {
	respDict := bencode.Dict{
		"interval":   bencode.Int(1800),
		"complete":   bencode.Int(0),
		"incomplete": bencode.Int(0),
		"peers":      bencode.String(""),
	}
	respData, _ := bencode.EncodeBytes(respDict)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(respData)
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{Port: 6881, Left: 100}
	resp, err := client.Announce(server.URL, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(resp.Peers))
	}
}

func TestAnnounceQueryParams(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		respDict := bencode.Dict{
			"interval": bencode.Int(60),
			"peers":    bencode.String(""),
		}
		respData, _ := bencode.EncodeBytes(respDict)
		w.Write(respData)
	}))
	defer server.Close()

	client := NewTrackerClient()
	req := &AnnounceRequest{
		Port:       6881,
		Uploaded:   1000,
		Downloaded: 500,
		Left:       10000,
		Event:      "started",
		NumWant:    50,
	}
	_, err := client.Announce(server.URL, req)
	if err != nil {
		t.Fatal(err)
	}

	parsed := urlParseQuery(capturedURL)
	checks := []struct {
		key, expected string
	}{
		{"port", "6881"},
		{"uploaded", "1000"},
		{"downloaded", "500"},
		{"left", "10000"},
		{"event", "started"},
		{"compact", "1"},
		{"numwant", "50"},
	}
	for _, c := range checks {
		if parsed[c.key] != c.expected {
			t.Errorf("expected %s=%s, got %s", c.key, c.expected, parsed[c.key])
		}
	}

	if parsed["peer_id"] == "" {
		t.Error("expected peer_id in query")
	}
	if parsed["info_hash"] == "" {
		t.Error("expected info_hash in query")
	}
}

func urlParseQuery(rawURL string) map[string]string {
	result := make(map[string]string)
	for i := 0; i < len(rawURL); i++ {
		if rawURL[i] == '?' {
			raw := rawURL[i+1:]
			for len(raw) > 0 {
				eq := -1
				amp := -1
				for j, c := range raw {
					if c == '=' && eq < 0 {
						eq = j
					}
					if c == '&' && amp < 0 {
						amp = j
						break
					}
				}
				if eq < 0 {
					break
				}
				key := raw[:eq]
				valEnd := amp
				if valEnd < 0 {
					valEnd = len(raw)
				}
				val := raw[eq+1 : valEnd]
				result[key] = val
				if amp < 0 {
					break
				}
				raw = raw[amp+1:]
			}
			break
		}
	}
	return result
}
