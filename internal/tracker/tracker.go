package tracker

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ayushamin/bittorent/internal/bencode"
)

type TrackerClient struct {
	httpClient *http.Client
	PeerID     [20]byte
}

type AnnounceRequest struct {
	InfoHash   [20]byte
	Port       uint16
	Uploaded   int64
	Downloaded int64
	Left       int64
	Event      string
	NumWant    int
}

type AnnounceResponse struct {
	FailureReason string
	Interval      int
	Complete      int
	Incomplete    int
	Peers         []Peer
}

type Peer struct {
	IP   net.IP
	Port uint16
}

func NewTrackerClient() *TrackerClient {
	return &TrackerClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		PeerID:     generatePeerID(),
	}
}

func generatePeerID() [20]byte {
	var id [20]byte
	copy(id[:], []byte("-GO0001-"))
	rand.Read(id[8:])
	return id
}

func (c *TrackerClient) Announce(announceURL string, req *AnnounceRequest) (*AnnounceResponse, error) {
	u, err := url.Parse(announceURL)
	if err != nil {
		return nil, fmt.Errorf("tracker: invalid announce URL: %w", err)
	}

	infoHashStr := string(req.InfoHash[:])
	peerIDStr := string(c.PeerID[:])

	q := u.Query()
	q.Set("info_hash", infoHashStr)
	q.Set("peer_id", peerIDStr)
	q.Set("port", strconv.Itoa(int(req.Port)))
	q.Set("uploaded", strconv.FormatInt(req.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(req.Downloaded, 10))
	q.Set("left", strconv.FormatInt(req.Left, 10))
	q.Set("compact", "1")
	if req.Event != "" {
		q.Set("event", req.Event)
	}
	if req.NumWant > 0 {
		q.Set("numwant", strconv.Itoa(req.NumWant))
	}
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("tracker: announce request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tracker: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseAnnounceResponse(body)
}

func parseAnnounceResponse(data []byte) (*AnnounceResponse, error) {
	v, err := bencode.DecodeBytes(data)
	if err != nil {
		return nil, fmt.Errorf("tracker: decode response: %w", err)
	}

	d, ok := v.(bencode.Dict)
	if !ok {
		return nil, fmt.Errorf("tracker: response is not a dict")
	}

	res := &AnnounceResponse{}

	if failure, err := d.GetString("failure reason"); err == nil {
		res.FailureReason = string(failure)
		return res, fmt.Errorf("tracker: %s", res.FailureReason)
	}

	if interval, err := d.GetInt("interval"); err == nil {
		res.Interval = int(interval)
	}

	if complete, err := d.GetInt("complete"); err == nil {
		res.Complete = int(complete)
	}

	if incomplete, err := d.GetInt("incomplete"); err == nil {
		res.Incomplete = int(incomplete)
	}

	peersVal, ok := d["peers"]
	if !ok {
		return res, nil
	}

	switch pv := peersVal.(type) {
	case bencode.String:
		parsed, err := parseCompactPeers([]byte(pv))
		if err != nil {
			return nil, fmt.Errorf("tracker: parse peers: %w", err)
		}
		res.Peers = parsed
	case bencode.List:
		var parsed []Peer
		for _, item := range pv {
			pd, ok := item.(bencode.Dict)
			if !ok {
				continue
			}
			peer, err := parseDictPeer(pd)
			if err != nil {
				continue
			}
			parsed = append(parsed, peer)
		}
		res.Peers = parsed
	}

	return res, nil
}

func parseCompactPeers(data []byte) ([]Peer, error) {
	if len(data)%6 != 0 {
		return nil, fmt.Errorf("invalid compact peer data length %d", len(data))
	}
	peers := make([]Peer, 0, len(data)/6)
	for i := 0; i < len(data); i += 6 {
		ip := net.IP(data[i : i+4])
		port := binary.BigEndian.Uint16(data[i+4 : i+6])
		peers = append(peers, Peer{IP: ip, Port: port})
	}
	return peers, nil
}

func parseDictPeer(d bencode.Dict) (Peer, error) {
	var peer Peer

	if ip, err := d.GetString("ip"); err == nil {
		peer.IP = net.ParseIP(string(ip))
	}

	if port, err := d.GetInt("port"); err == nil {
		peer.Port = uint16(port)
	}

	if peer.IP == nil {
		return peer, fmt.Errorf("no valid IP")
	}

	return peer, nil
}
