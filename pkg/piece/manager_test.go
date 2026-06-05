package piece

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	pm := NewManager(10, 16384, 100000)
	if pm.NumPieces() != 10 {
		t.Fatalf("expected 10 pieces, got %d", pm.NumPieces())
	}
}

func TestMarkDownloaded(t *testing.T) {
	pm := NewManager(5, 1024, 5000)
	if pm.Have(0) {
		t.Fatal("expected piece 0 to be missing initially")
	}
	pm.MarkDownloaded(0)
	if !pm.Have(0) {
		t.Fatal("expected piece 0 to be have after mark")
	}
}

func TestProgress(t *testing.T) {
	pm := NewManager(4, 1024, 4096)
	if pm.Progress() != 0 {
		t.Fatal("expected 0% progress initially")
	}
	pm.MarkDownloaded(0)
	pm.MarkDownloaded(1)
	if pm.Progress() != 50 {
		t.Fatalf("expected 50%% progress, got %f", pm.Progress())
	}
}

func TestComplete(t *testing.T) {
	pm := NewManager(3, 1024, 3072)
	if pm.Complete() {
		t.Fatal("expected not complete initially")
	}
	pm.MarkDownloaded(0)
	pm.MarkDownloaded(1)
	pm.MarkDownloaded(2)
	if !pm.Complete() {
		t.Fatal("expected complete after all pieces")
	}
}

func TestPickPieceNoPeers(t *testing.T) {
	pm := NewManager(5, 1024, 5120)
	_, ok := pm.PickPiece([]bool{false, false, false, false, false})
	if ok {
		t.Fatal("expected no piece when peer has nothing we need")
	}
}

func TestPickPieceRarestFirst(t *testing.T) {
	pm := NewManager(3, 1024, 3072)

	pm.UpdatePeerBitfield("peer1", []bool{true, true, false})
	pm.UpdatePeerBitfield("peer2", []bool{true, false, false})

	peerBf := []bool{true, true, true}
	index, ok := pm.PickPiece(peerBf)
	if !ok {
		t.Fatal("expected a piece to pick")
	}

	if index == 0 {
		t.Fatalf("expected rarest piece (not 0, which 2 peers have), got %d", index)
	}
}

func TestPickPieceOnlyAvailable(t *testing.T) {
	pm := NewManager(5, 1024, 5120)
	pm.MarkDownloaded(0)
	pm.MarkDownloaded(1)
	pm.MarkDownloaded(3)

	peerBf := []bool{true, true, true, true, true}
	index, ok := pm.PickPiece(peerBf)
	if !ok {
		t.Fatal("expected a piece")
	}
	if index == 0 || index == 1 || index == 3 {
		t.Fatalf("picked already downloaded piece %d", index)
	}
}

func TestMarkInProgress(t *testing.T) {
	pm := NewManager(3, 1024, 3072)
	pm.MarkInProgress(1)

	peerBf := []bool{true, true, true}
	_, ok := pm.PickPiece(peerBf)
	if !ok {
		t.Fatal("expected a piece even when one is in progress")
	}
}

func TestMissingPieces(t *testing.T) {
	pm := NewManager(4, 1024, 4096)
	pm.MarkDownloaded(0)
	pm.MarkDownloaded(2)
	missing := pm.MissingPieces()
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing pieces, got %d", len(missing))
	}
	if missing[0] != 1 || missing[1] != 3 {
		t.Fatalf("expected missing pieces [1,3], got %v", missing)
	}
}

func TestUpdatePeerBitfield(t *testing.T) {
	pm := NewManager(5, 1024, 5120)
	pm.UpdatePeerBitfield("peer1", []bool{true, false, true, false, true})

	count := pm.CountPeerForPiece(0)
	if count != 1 {
		t.Fatalf("expected 1 peer for piece 0, got %d", count)
	}

	count = pm.CountPeerForPiece(1)
	if count != 0 {
		t.Fatalf("expected 0 peers for piece 1, got %d", count)
	}
}

func TestRemovePeer(t *testing.T) {
	pm := NewManager(3, 1024, 3072)
	pm.UpdatePeerBitfield("peer1", []bool{true, false, true})
	pm.RemovePeer("peer1")

	count := pm.CountPeerForPiece(0)
	if count != 0 {
		t.Fatal("expected 0 peers after removal")
	}
}

func TestReleasePiece(t *testing.T) {
	pm := NewManager(3, 1024, 3072)
	pm.MarkDownloaded(0)
	pm.MarkDownloaded(2)
	pm.MarkInProgress(1)
	pm.ReleasePiece(1)

	peerBf := []bool{false, true, false}
	index, ok := pm.PickPiece(peerBf)
	if !ok || index != 1 {
		t.Fatalf("expected released piece 1 to be pickable, got %d, %v", index, ok)
	}
}

func TestReleasePieceNoopOnHave(t *testing.T) {
	pm := NewManager(3, 1024, 3072)
	pm.MarkDownloaded(0)
	pm.ReleasePiece(0)

	_, ok := pm.PickPiece([]bool{true, false, false})
	if ok {
		t.Fatal("expected no piece after releasing Have piece")
	}
}

func TestPeerHasPiece(t *testing.T) {
	pm := NewManager(5, 1024, 5120)
	pm.UpdatePeerBitfield("peer1", []bool{true, false, true, false, true})
	pm.PeerHasPiece("peer1", 1)

	count := pm.CountPeerForPiece(1)
	if count != 1 {
		t.Fatalf("expected 1 peer for piece 1 after PeerHasPiece, got %d", count)
	}

	pm.PeerHasPiece("unknown", 0)
}

func TestPieceLength(t *testing.T) {
	pm := NewManager(4, 262144, 1048576)
	if pm.PieceLength(0) != 262144 {
		t.Fatal("expected 262144 for first piece")
	}
	if pm.PieceLength(3) != 262144 {
		t.Fatal("expected 262144 for last piece when evenly divisible")
	}

	pm2 := NewManager(4, 262144, 1000000)
	expectedLast := int64(1000000 - 3*262144)
	if pm2.PieceLength(3) != expectedLast {
		t.Fatalf("expected %d for last piece, got %d", expectedLast, pm2.PieceLength(3))
	}
}
