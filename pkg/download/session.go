package download

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/apk471/bittorrent/pkg/peer"
	"github.com/apk471/bittorrent/pkg/piece"
	"github.com/apk471/bittorrent/pkg/storage"
	"github.com/apk471/bittorrent/pkg/torrent"
	"github.com/apk471/bittorrent/pkg/tracker"
)

// Logger is the interface for logging. Pass nil to disable all logging.
type Logger interface {
	Printf(format string, v ...any)
}

// LoggerFunc is an adapter to use ordinary functions as loggers.
type LoggerFunc func(format string, v ...any)

func (f LoggerFunc) Printf(format string, v ...any) { f(format, v...) }

type Session struct {
	Torrent   *torrent.TorrentFile
	PieceMgr  *piece.Manager
	Storage   *storage.Storage
	PeerID    [20]byte
	client    *tracker.TrackerClient
	OutputDir string
	StopCh    chan struct{}
	trackerURL string
	numWorkers int
	log        Logger
}

// SetLogger sets the logger for the session. Pass nil to disable logging.
func (s *Session) SetLogger(l Logger) { s.log = l }

func (s *Session) logf(format string, v ...any) {
	if s.log != nil {
		s.log.Printf(format, v...)
	}
}

func New(tf *torrent.TorrentFile, outputDir string) (*Session, error) {
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
		Torrent:    tf,
		PieceMgr:   pm,
		Storage:    st,
		PeerID:     peerID,
		client:     client,
		OutputDir:  outputDir,
		StopCh:     make(chan struct{}),
		trackerURL: tf.TrackerURL(),
		numWorkers: 30,
	}, nil
}

func (s *Session) Stop() {
	select {
	case <-s.StopCh:
	default:
		close(s.StopCh)
	}
}

func (s *Session) VerifyAll() (total, checked, failed int, err error) {
	numPieces := s.Torrent.NumPieces()
	for i := 0; i < numPieces; i++ {
		if !s.PieceMgr.Have(i) {
			continue
		}
		checked++
		data, err := s.Storage.ReadPiece(i, s.Torrent.Info.PieceLength)
		if err != nil {
			failed++
			continue
		}
		expected := s.Torrent.PieceHash(i)
		if !s.Storage.VerifyPiece(data, expected) {
			failed++
		}
	}
	return numPieces, checked, failed, nil
}

func (s *Session) Resume() (int, error) {
	if !s.Storage.Exists() {
		return 0, nil
	}
	numPieces := s.Torrent.NumPieces()
	restored := 0
	for i := 0; i < numPieces; i++ {
		data, err := s.Storage.ReadPiece(i, s.Torrent.Info.PieceLength)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return restored, fmt.Errorf("read piece %d during resume: %w", i, err)
		}
		pieceLen := s.PieceMgr.PieceLength(i)
		if int64(len(data)) != pieceLen {
			continue
		}
		expected := s.Torrent.PieceHash(i)
		if s.Storage.VerifyPiece(data, expected) {
			s.PieceMgr.MarkDownloaded(i)
			restored++
		}
	}
	if restored > 0 {
		s.logf("Resumed %d/%d pieces (%.1f%%)", restored, numPieces, float64(restored)/float64(numPieces)*100)
	}
	return restored, nil
}

func (s *Session) Run() error {
	if s.trackerURL == "" {
		return fmt.Errorf("no tracker URL (trackerless torrents not supported)")
	}

	restored, err := s.Resume()
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	_ = restored

	if s.PieceMgr.Complete() {
		s.logf("All pieces already downloaded")
		return nil
	}

	peers := make(chan tracker.Peer, 200)
	var wg sync.WaitGroup

	for i := 0; i < s.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if s.PieceMgr.Complete() {
					return
				}
				select {
				case <-s.StopCh:
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

	var trackerWg sync.WaitGroup
	trackerWg.Add(1)
	go func() {
		defer trackerWg.Done()
		defer close(peers)
		interval := 30 * time.Second

		announce := func(event string) {
			resp, err := s.client.Announce(s.trackerURL, &tracker.AnnounceRequest{
				InfoHash: s.Torrent.InfoHash,
				Port:     uint16(6881 + rand.Intn(100)),
				Uploaded: 0,
				Left:     s.Torrent.TotalSize(),
				Event:    event,
			})
			if err != nil {
				s.logf("Tracker announce failed: %v", err)
				return
			}
			if resp.Interval > 0 {
				interval = time.Duration(resp.Interval) * time.Second
			}
			s.logf("Got %d peers from tracker (%d seeders, %d leechers)",
				len(resp.Peers), resp.Complete, resp.Incomplete)

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
			case <-s.StopCh:
				announce("stopped")
				return
			case <-time.After(interval):
				if s.PieceMgr.Complete() {
					announce("completed")
					return
				}
				announce("")
			}
		}
	}()

	wg.Wait()
	s.Stop()
	trackerWg.Wait()

	if s.PieceMgr.Complete() {
		return nil
	}
	return fmt.Errorf("download incomplete: %.1f%% done", s.PieceMgr.Progress())
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
		pc.SetReadTimeout(30 * time.Second)
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
		case <-s.StopCh:
			s.logf("  download stopped while waiting for unchoke")
			return false
		case <-time.After(30 * time.Second):
			s.logf("  timeout waiting for unchoke")
			return false
		}
	}
}

func (s *Session) downloadFromPeer(p tracker.Peer) {
	addr := net.JoinHostPort(p.IP.String(), fmt.Sprintf("%d", p.Port))
	s.logf("Connecting to peer %s", addr)

	pc, err := peer.Dial(addr, s.Torrent.InfoHash, s.PeerID, 10*time.Second)
	if err != nil {
		s.logf("  connect failed: %v", err)
		return
	}
	defer pc.Close()

	s.logf("  connected, peer ID: %x", pc.RemoteID)

	pc.SetReadTimeout(10 * time.Second)
	firstMsg, err := pc.ReadMessage()
	if err != nil {
		s.logf("  read first message: %v", err)
		return
	}
	peerBitfield := s.parsePeerBitfield(firstMsg, s.Torrent.NumPieces())

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
	s.logf("  peer has %d/%d pieces (first msg: %s)", have, s.Torrent.NumPieces(), firstMsgID)

	if have == 0 {
		s.logf("  peer has no pieces, skipping")
		return
	}

	ch := make(chan *peer.Message, 64)
	errCh := make(chan error, 1)
	go s.readPeerMessages(pc, ch, errCh)

	if firstMsg != nil && firstMsg.ID != peer.MsgBitfield {
		ch <- firstMsg
	}

	s.PieceMgr.UpdatePeerBitfield(string(pc.RemoteID[:]), peerBitfield)

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
		if s.PieceMgr.Complete() {
			return
		}

		select {
		case <-s.StopCh:
			s.logf("  download stopped")
			return
		default:
		}

		pieceIndex, ok := s.PieceMgr.PickPiece(peerBitfield)
		if !ok {
			s.logf("  no more pieces to download from this peer")
			return
		}

		s.logf("  downloading piece %d/%d", pieceIndex, s.Torrent.NumPieces()-1)

		if err := s.downloadPiece(pc, ch, errCh, pieceIndex); err != nil {
			s.logf("  piece %d failed: %v", pieceIndex, err)
			s.PieceMgr.ReleasePiece(pieceIndex)
			s.PieceMgr.RemovePeer(string(pc.RemoteID[:]))
			return
		}
	}
}

func (s *Session) downloadPiece(pc *peer.PeerConn, msgCh chan *peer.Message, errCh chan error, index int) error {
	pieceLen := s.PieceMgr.PieceLength(index)
	expectedHash := s.Torrent.PieceHash(index)

	data := make([]byte, pieceLen)
	const blockSize = 16384

	received := int64(0)
	requested := int64(0)
	outstanding := 0
	const maxOutstanding = 5

	for received < pieceLen {
		for outstanding < maxOutstanding && requested < pieceLen {
			size := blockSize
			if pieceLen-requested < int64(size) {
				size = int(pieceLen - requested)
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
			requested += int64(size)
		}

		var resp *peer.Message
		select {
		case resp = <-msgCh:
		case err := <-errCh:
			return fmt.Errorf("read piece block: %w", err)
		case <-time.After(60 * time.Second):
			return fmt.Errorf("timeout waiting for piece block")
		}

		if resp == nil {
			continue
		}
		switch resp.ID {
		case peer.MsgPiece:
			outstanding--
			end := int(resp.Begin) + len(resp.Payload)
			if end > len(data) {
				return fmt.Errorf("block exceeds piece buffer: begin=%d len=%d cap=%d", resp.Begin, len(resp.Payload), len(data))
			}
			copy(data[resp.Begin:], resp.Payload)
			received += int64(len(resp.Payload))
		case peer.MsgChoke:
			return fmt.Errorf("peer choked us")
		case peer.MsgHave:
			s.PieceMgr.PeerHasPiece(string(pc.RemoteID[:]), int(resp.Index))
		}
	}

	if !s.Storage.VerifyPiece(data, expectedHash) {
		return fmt.Errorf("hash mismatch for piece %d", index)
	}

	if err := s.Storage.WritePiece(index, data, s.Torrent.Info.PieceLength); err != nil {
		return fmt.Errorf("write piece %d: %w", index, err)
	}

	s.PieceMgr.MarkDownloaded(index)
	s.logf("  piece %d complete (%.1f%%)", index, s.PieceMgr.Progress())
	return nil
}
