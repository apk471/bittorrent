package download

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayush-amin/bittorrent/pkg/dht"
	"github.com/ayush-amin/bittorrent/pkg/peer"
	"github.com/ayush-amin/bittorrent/pkg/piece"
	"github.com/ayush-amin/bittorrent/pkg/storage"
	"github.com/ayush-amin/bittorrent/pkg/torrent"
	"github.com/ayush-amin/bittorrent/pkg/tracker"
)

// Logger is the interface for logging. Pass nil to disable all logging.
type Logger interface {
	Printf(format string, v ...any)
}

// LoggerFunc is an adapter to use ordinary functions as loggers.
type LoggerFunc func(format string, v ...any)

func (f LoggerFunc) Printf(format string, v ...any) { f(format, v...) }

// Sentinel errors returned by the download package.
var (
	ErrTrackerless = errors.New("no tracker URL (trackerless torrents not supported)")
	ErrTimeout     = errors.New("timeout waiting for piece block")
	ErrPeerChoked  = errors.New("peer choked us")
)

// Config contains configurable parameters for a Session.
// Zero values use sensible defaults.
type Config struct {
	NumWorkers       int
	BlockSize        int
	PipelineDepth    int
	PeerChanSize     int
	MsgChanSize      int
	PortOffset       int
	DialTimeout      time.Duration
	ReadTimeout      time.Duration
	PieceTimeout     time.Duration
	EndgameThreshold int
	Logger           Logger
}

func (c *Config) defaults() {
	if c.NumWorkers <= 0 {
		c.NumWorkers = 30
	}
	if c.BlockSize <= 0 {
		c.BlockSize = 16384
	}
	if c.PipelineDepth <= 0 {
		c.PipelineDepth = 5
	}
	if c.PeerChanSize <= 0 {
		c.PeerChanSize = 200
	}
	if c.MsgChanSize <= 0 {
		c.MsgChanSize = 64
	}
	if c.PortOffset <= 0 {
		c.PortOffset = 6881
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = 10 * time.Second
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.PieceTimeout <= 0 {
		c.PieceTimeout = 60 * time.Second
	}
	if c.EndgameThreshold <= 0 {
		c.EndgameThreshold = 20
	}
}

// Session orchestrates a full BitTorrent download: tracker announces,
// peer connections, piece selection, and storage I/O.
type Session struct {
	torrent    *torrent.TorrentFile
	pieceMgr   *piece.Manager
	storage    *storage.Storage
	peerID     [20]byte
	client     *tracker.TrackerClient
	outputDir  string
	stopCh     chan struct{}
	trackers   []string
	cfg        Config
	log        Logger

	dhtNode *dht.DHT

	endgame        atomic.Bool
	receivedBlocks map[int][]bool
	receivedMu     sync.Mutex
}

// Torrent returns the parsed torrent metadata.
func (s *Session) Torrent() *torrent.TorrentFile { return s.torrent }

// PieceMgr returns the piece manager.
func (s *Session) PieceMgr() *piece.Manager { return s.pieceMgr }

// Storage returns the storage engine.
func (s *Session) Storage() *storage.Storage { return s.storage }

// PeerID returns the peer ID used by this session.
func (s *Session) PeerID() [20]byte { return s.peerID }

// OutputDir returns the output directory path.
func (s *Session) OutputDir() string { return s.outputDir }

// StopCh returns a channel that is closed when Stop is called.
func (s *Session) StopCh() <-chan struct{} { return s.stopCh }

func (s *Session) blockAlreadyReceived(index int, begin int64) bool {
	s.receivedMu.Lock()
	defer s.receivedMu.Unlock()
	blocks, ok := s.receivedBlocks[index]
	if !ok {
		return false
	}
	blockIndex := begin / int64(s.cfg.BlockSize)
	return blockIndex < int64(len(blocks)) && blocks[blockIndex]
}

func (s *Session) markBlockReceived(index int, begin int64) {
	s.receivedMu.Lock()
	defer s.receivedMu.Unlock()
	blocks, ok := s.receivedBlocks[index]
	if !ok {
		numBlocks := (s.pieceMgr.PieceLength(index) + int64(s.cfg.BlockSize) - 1) / int64(s.cfg.BlockSize)
		blocks = make([]bool, numBlocks)
		s.receivedBlocks[index] = blocks
	}
	blockIndex := begin / int64(s.cfg.BlockSize)
	if blockIndex < int64(len(blocks)) {
		blocks[blockIndex] = true
	}
}

// SetLogger sets the logger for the session. Pass nil to disable logging.
func (s *Session) SetLogger(l Logger) { s.log = l }

func (s *Session) logf(format string, v ...any) {
	if s.log != nil {
		s.log.Printf(format, v...)
	}
}

// New creates a download Session for the given torrent and output directory.
// Pass a nil or zero Config to use sensible defaults.
func New(tf *torrent.TorrentFile, outputDir string, cfg *Config) (*Session, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.defaults()

	client := tracker.NewTrackerClient()
	peerID := client.PeerID

	pm := piece.NewManager(
		tf.NumPieces(),
		tf.Info.PieceLength,
		tf.TotalSize(),
	)

	var files []storage.FileInfo
	for _, f := range tf.Info.Files {
		files = append(files, storage.FileInfo{
			Length: f.Length,
			Path:   f.Path,
		})
	}

	st, err := storage.New(outputDir, tf.Info.Name, tf.Info.Length, files)
	if err != nil {
		return nil, err
	}

	return &Session{
		torrent:    tf,
		pieceMgr:   pm,
		storage:    st,
		peerID:     peerID,
		client:     client,
		outputDir:  outputDir,
		stopCh:     make(chan struct{}),
		trackers:   tf.HTTPTrackers(),
		cfg:        *cfg,
		log:        cfg.Logger,
	}, nil
}

// Stop signals all workers to shut down gracefully.
func (s *Session) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
	if s.dhtNode != nil {
		s.dhtNode.Stop()
	}
}

// VerifyAll checks SHA-1 hashes for all Have-marked pieces on disk.
func (s *Session) VerifyAll() (total, checked, failed int, err error) {
	numPieces := s.torrent.NumPieces()
	for i := 0; i < numPieces; i++ {
		if !s.pieceMgr.Have(i) {
			continue
		}
		checked++
		data, err := s.storage.ReadPiece(i, s.torrent.Info.PieceLength)
		if err != nil {
			failed++
			continue
		}
		expected := s.torrent.PieceHash(i)
		if !s.storage.VerifyPiece(data, expected) {
			failed++
		}
	}
	return numPieces, checked, failed, nil
}

// Resume scans storage for existing pieces, verifies SHA-1 hashes,
// and marks valid pieces as downloaded.
func (s *Session) Resume() (int, error) {
	if !s.storage.Exists() {
		return 0, nil
	}
	numPieces := s.torrent.NumPieces()
	restored := 0
	for i := 0; i < numPieces; i++ {
		data, err := s.storage.ReadPiece(i, s.torrent.Info.PieceLength)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return restored, fmt.Errorf("read piece %d during resume: %w", i, err)
		}
		pieceLen := s.pieceMgr.PieceLength(i)
		if int64(len(data)) != pieceLen {
			continue
		}
		expected := s.torrent.PieceHash(i)
		if s.storage.VerifyPiece(data, expected) {
			s.pieceMgr.MarkDownloaded(i)
			restored++
		}
	}
	if restored > 0 {
		s.logf("Resumed %d/%d pieces (%.1f%%)", restored, numPieces, float64(restored)/float64(numPieces)*100)
	}
	return restored, nil
}

// Run starts the download session: announces to the tracker and/or DHT,
// spawns worker goroutines, and downloads pieces until completion or stop.
func (s *Session) Run() error {
	restored, err := s.Resume()
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	_ = restored

	if s.pieceMgr.Complete() {
		s.logf("All pieces already downloaded")
		return nil
	}

	peers := make(chan tracker.Peer, s.cfg.PeerChanSize)
	var wg sync.WaitGroup

	for i := 0; i < s.cfg.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if s.pieceMgr.Complete() {
					return
				}
				select {
				case <-s.stopCh:
					return
				case p, ok := <-peers:
					if !ok {
						return
					}
					s.downloadFromPeer(p)
				}
			}
		}()
	}

	var sourceWg sync.WaitGroup
	launch := func(fn func(chan<- tracker.Peer)) {
		sourceWg.Add(1)
		go func() {
			defer sourceWg.Done()
			fn(peers)
		}()
	}

	for _, url := range s.trackers {
		url := url
		launch(func(p chan<- tracker.Peer) { s.runTracker(url, p) })
	}
	// Always run DHT alongside trackers for extra peer discovery; it is the
	// only source for trackerless torrents (empty announce lists).
	launch(func(p chan<- tracker.Peer) { s.runDHT(p) })

	// Close the peers channel once every source has stopped.
	go func() {
		sourceWg.Wait()
		close(peers)
	}()

	wg.Wait()
	s.Stop()
	sourceWg.Wait()

	if s.pieceMgr.Complete() {
		return nil
	}
	return fmt.Errorf("download incomplete: %.1f%% done", s.pieceMgr.Progress())
}

func (s *Session) runTracker(trackerURL string, peers chan<- tracker.Peer) {
	interval := 30 * time.Second

	announce := func(event string) {
		resp, err := s.client.Announce(trackerURL, &tracker.AnnounceRequest{
			InfoHash: s.torrent.InfoHash,
			Port:     uint16(s.cfg.PortOffset + rand.Intn(100)),
			Uploaded: 0,
			Left:     s.torrent.TotalSize(),
			Event:    event,
		})
		if err != nil {
			s.logf("Tracker %s announce failed: %v", trackerURL, err)
			return
		}
		if resp.Interval > 0 {
			interval = time.Duration(resp.Interval) * time.Second
		}
		s.logf("Got %d peers from %s (%d seeders, %d leechers)",
			len(resp.Peers), trackerURL, resp.Complete, resp.Incomplete)

		shuffled := make([]tracker.Peer, len(resp.Peers))
		copy(shuffled, resp.Peers)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		for _, p := range shuffled {
			select {
			case peers <- p:
			default:
			}
		}
	}

	announce("started")

	for {
		select {
		case <-s.stopCh:
			announce("stopped")
			return
		case <-time.After(interval):
			if s.pieceMgr.Complete() {
				announce("completed")
				return
			}
			announce("")
		}
	}
}

func (s *Session) runDHT(peers chan<- tracker.Peer) {
	s.logf("Starting DHT node for trackerless torrent...")

	dhtNode, err := dht.New(s.cfg.PortOffset+rand.Intn(100), peers, s.log)
	if err != nil {
		s.logf("DHT: %v", err)
		return
	}
	s.dhtNode = dhtNode

	infoHash := s.torrent.InfoHash

	go func() {
		time.Sleep(2 * time.Second)
		dhtNode.GetPeers(infoHash)
	}()

	if err := dhtNode.Run(); err != nil {
		s.logf("DHT: %v", err)
	}
}

func (s *Session) parsePeerBitfield(msg *peer.Message, numPieces int) []bool {
	bf := make([]bool, numPieces)
	if msg == nil || msg.ID != peer.MsgBitfield {
		return bf
	}
	for i := 0; i < len(msg.Payload)*8 && i < numPieces; i++ {
		if msg.Payload[i/8]&(1<<(7-i%8)) != 0 {
			bf[i] = true
		}
	}
	return bf
}

func (s *Session) readPeerMessages(pc *peer.PeerConn, ch chan *peer.Message, errCh chan error) {
	for {
		pc.SetReadTimeout(s.cfg.ReadTimeout)
		msg, err := pc.ReadMessage()
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		ch <- msg
	}
}

func (s *Session) waitForUnchoke(pc *peer.PeerConn, msgCh chan *peer.Message, errCh chan error) bool {
	for {
		select {
		case msg := <-msgCh:
			if msg == nil {
				continue
			}
			switch msg.ID {
			case peer.MsgUnchoke:
				return true
			case peer.MsgChoke:
				return false
			case peer.MsgBitfield:
			default:
			}
		case err := <-errCh:
			s.logf("  error waiting for unchoke: %v", err)
			return false
		case <-s.stopCh:
			s.logf("  download stopped while waiting for unchoke")
			return false
		case <-time.After(s.cfg.ReadTimeout):
			s.logf("  timeout waiting for unchoke")
			return false
		}
	}
}

func (s *Session) downloadFromPeer(p tracker.Peer) {
	addr := net.JoinHostPort(p.IP.String(), fmt.Sprintf("%d", p.Port))
	s.logf("Connecting to peer %s", addr)

	pc, err := peer.Dial(addr, s.torrent.InfoHash, s.peerID, s.cfg.DialTimeout)
	if err != nil {
		s.logf("  connect failed: %v", err)
		return
	}
	defer pc.Close()

	remoteID := pc.RemoteID()
	s.logf("  connected, peer ID: %x", remoteID)

	pc.SetReadTimeout(s.cfg.ReadTimeout)
	firstMsg, err := pc.ReadMessage()
	if err != nil {
		s.logf("  read first message: %v", err)
		return
	}
	peerBitfield := s.parsePeerBitfield(firstMsg, s.torrent.NumPieces())

	have := 0
	for _, b := range peerBitfield {
		if b {
			have++
		}
	}
	firstMsgID := "keep-alive"
	if firstMsg != nil {
		firstMsgID = firstMsg.ID.String()
	}
	s.logf("  peer has %d/%d pieces (first msg: %s)", have, s.torrent.NumPieces(), firstMsgID)

	if have == 0 {
		s.logf("  peer has no pieces, skipping")
		return
	}

	ch := make(chan *peer.Message, s.cfg.MsgChanSize)
	errCh := make(chan error, 1)
	go s.readPeerMessages(pc, ch, errCh)

	if firstMsg != nil && firstMsg.ID != peer.MsgBitfield {
		ch <- firstMsg
	}

	s.pieceMgr.UpdatePeerBitfield(string(remoteID[:]), peerBitfield)

	if err := pc.SendMessage(&peer.Message{ID: peer.MsgInterested}); err != nil {
		s.logf("  send interested failed: %v", err)
		return
	}

	s.logf("  waiting for unchoke...")
	if !s.waitForUnchoke(pc, ch, errCh) {
		s.logf("  peer did not unchoke us")
		return
	}
	s.logf("  unchoked, starting download")

	for {
		if s.pieceMgr.Complete() {
			return
		}

		select {
		case <-s.stopCh:
			s.logf("  download stopped")
			return
		default:
		}

		pieceIndex, ok := s.pieceMgr.PickPiece(peerBitfield)
		if !ok {
			if s.pieceMgr.MissingCount() <= s.cfg.EndgameThreshold {
				s.endgame.Store(true)
				pieceIndex, ok = s.pieceMgr.PickAnyMissing(peerBitfield)
			}
			if !ok {
				s.logf("  no more pieces to download from this peer")
				return
			}
		}

		s.logf("  downloading piece %d/%d", pieceIndex, s.torrent.NumPieces()-1)

		if err := s.downloadPiece(pc, ch, errCh, pieceIndex); err != nil {
			s.logf("  piece %d failed: %v", pieceIndex, err)
			if !s.endgame.Load() {
				s.pieceMgr.ReleasePiece(pieceIndex)
			}
			s.pieceMgr.RemovePeer(string(remoteID[:]))
			return
		}
	}
}

func (s *Session) downloadPiece(pc *peer.PeerConn, msgCh chan *peer.Message, errCh chan error, index int) error {
	if s.pieceMgr.Have(index) {
		return nil
	}
	pieceLen := s.pieceMgr.PieceLength(index)
	expectedHash := s.torrent.PieceHash(index)
	remoteID := pc.RemoteID()

	data := make([]byte, pieceLen)
	blockSize := int64(s.cfg.BlockSize)

	received := int64(0)
	requested := int64(0)
	outstanding := 0
	maxOutstanding := s.cfg.PipelineDepth

	for received < pieceLen {
		for outstanding < maxOutstanding && requested < pieceLen {
			size := blockSize
			if pieceLen-requested < size {
				size = pieceLen - requested
			}

			msg := &peer.Message{
				ID:     peer.MsgRequest,
				Index:  uint32(index),
				Begin:  uint32(requested),
				Length: uint32(size),
			}
			if err := pc.SendMessage(msg); err != nil {
				return fmt.Errorf("send request: %w", err)
			}
			outstanding++
			requested += size
		}

		var resp *peer.Message
		select {
		case resp = <-msgCh:
		case err := <-errCh:
			return fmt.Errorf("read piece block: %w", err)
		case <-time.After(s.cfg.PieceTimeout):
			return ErrTimeout
		}

		if resp == nil {
			continue
		}
		switch resp.ID {
		case peer.MsgPiece:
			outstanding--
			if s.endgame.Load() && s.blockAlreadyReceived(index, int64(resp.Begin)) {
				continue
			}
			end := int(resp.Begin) + len(resp.Payload)
			if end > len(data) {
				return fmt.Errorf("block exceeds piece buffer: begin=%d len=%d cap=%d", resp.Begin, len(resp.Payload), len(data))
			}
			copy(data[resp.Begin:], resp.Payload)
			received += int64(len(resp.Payload))
			if s.endgame.Load() {
				s.markBlockReceived(index, int64(resp.Begin))
			}
		case peer.MsgChoke:
			return ErrPeerChoked
		case peer.MsgHave:
			s.pieceMgr.PeerHasPiece(string(remoteID[:]), int(resp.Index))
		}
	}

	if !s.storage.VerifyPiece(data, expectedHash) {
		return fmt.Errorf("hash mismatch for piece %d", index)
	}

	if err := s.storage.WritePiece(index, data, s.torrent.Info.PieceLength); err != nil {
		return fmt.Errorf("write piece %d: %w", index, err)
	}

	s.pieceMgr.MarkDownloaded(index)
	s.receivedMu.Lock()
	delete(s.receivedBlocks, index)
	s.receivedMu.Unlock()
	s.logf("  piece %d complete (%.1f%%)", index, s.pieceMgr.Progress())
	return nil
}
