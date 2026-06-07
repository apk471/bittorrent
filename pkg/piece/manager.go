package piece

import (
	"math/rand"
	"sync"
)

// State represents the download state of a piece.
type State byte

const (
	Missing    State = 0
	InProgress State = 1
	Have       State = 2
)

// Manager tracks piece state and peer bitfields for rarest-first selection.
type Manager struct {
	mu           sync.Mutex
	numPieces    int
	pieceLength  int64
	totalSize    int64
	state        []State
	peerBitfield map[string][]bool
}

// NewManager creates a piece manager for a torrent with the given parameters.
func NewManager(numPieces int, pieceLength, totalSize int64) *Manager {
	pm := &Manager{
		numPieces:    numPieces,
		pieceLength:  pieceLength,
		totalSize:    totalSize,
		state:        make([]State, numPieces),
		peerBitfield: make(map[string][]bool),
	}
	return pm
}

// NumPieces returns the total number of pieces.
func (pm *Manager) NumPieces() int {
	return pm.numPieces
}

// Have returns true if the piece at the given index has been downloaded.
func (pm *Manager) Have(index int) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if index < 0 || index >= pm.numPieces {
		return false
	}
	return pm.state[index] == Have
}

// MarkDownloaded marks the piece at the given index as downloaded.
func (pm *Manager) MarkDownloaded(index int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if index >= 0 && index < pm.numPieces {
		pm.state[index] = Have
	}
}

// MarkInProgress marks the piece as currently being downloaded.
func (pm *Manager) MarkInProgress(index int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if index >= 0 && index < pm.numPieces {
		pm.state[index] = InProgress
	}
}

// UpdatePeerBitfield records or updates a peer's available pieces.
func (pm *Manager) UpdatePeerBitfield(peerID string, bitfield []bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peerBitfield[peerID] = bitfield
}

// RemovePeer removes a peer's bitfield from tracking.
func (pm *Manager) RemovePeer(peerID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peerBitfield, peerID)
}

// PeerHasPiece records that a peer has a specific piece (from a have message).
func (pm *Manager) PeerHasPiece(peerID string, index int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	bf, ok := pm.peerBitfield[peerID]
	if !ok || index >= len(bf) {
		return
	}
	bf[index] = true
}

// ReleasePiece marks a previously InProgress piece as Missing.
func (pm *Manager) ReleasePiece(index int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if index >= 0 && index < pm.numPieces && pm.state[index] == InProgress {
		pm.state[index] = Missing
	}
}

// Progress returns the download progress as a percentage (0-100).
func (pm *Manager) Progress() float64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	have := 0
	for _, s := range pm.state {
		if s == Have {
			have++
		}
	}
	if pm.numPieces == 0 {
		return 0
	}
	return float64(have) / float64(pm.numPieces) * 100
}

// Complete returns true if all pieces have been downloaded.
func (pm *Manager) Complete() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, s := range pm.state {
		if s != Have {
			return false
		}
	}
	return true
}

// CountPeerForPiece returns how many known peers have the given piece.
func (pm *Manager) CountPeerForPiece(index int) int {
	count := 0
	for _, bf := range pm.peerBitfield {
		if index < len(bf) && bf[index] {
			count++
		}
	}
	return count
}

// PickPiece selects the rarest missing piece available from the peer.
// Returns the piece index and true, or (0, false) if none available.
func (pm *Manager) PickPiece(peerBitfield []bool) (int, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rarest := -1
	rarestCount := int(^uint(0) >> 1)
	candidates := make([]int, 0)

	for i := 0; i < pm.numPieces; i++ {
		if pm.state[i] != Missing {
			continue
		}
		if i >= len(peerBitfield) || !peerBitfield[i] {
			continue
		}
		count := 0
		for _, bf := range pm.peerBitfield {
			if i < len(bf) && bf[i] {
				count++
			}
		}
		if count < rarestCount {
			rarestCount = count
			candidates = []int{i}
		} else if count == rarestCount {
			candidates = append(candidates, i)
		}
	}

	if len(candidates) == 0 {
		return 0, false
	}

	rarest = candidates[rand.Intn(len(candidates))]
	pm.state[rarest] = InProgress
	return rarest, true
}

// MissingPieces returns a list of piece indices that are not yet downloaded.
func (pm *Manager) MissingPieces() []int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	var missing []int
	for i, s := range pm.state {
		if s != Have {
			missing = append(missing, i)
		}
	}
	return missing
}

// PieceLength returns the length of the piece at the given index.
func (pm *Manager) PieceLength(index int) int64 {
	if index < 0 || index >= pm.numPieces {
		return 0
	}
	if index < pm.numPieces-1 {
		return pm.pieceLength
	}
	last := pm.totalSize - (int64(pm.numPieces)-1)*pm.pieceLength
	if last <= 0 {
		return pm.pieceLength
	}
	return last
}
