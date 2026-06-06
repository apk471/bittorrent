# Library Readiness Plan

What's needed to make this project a proper Go library (beyond the CLI).

## Current Status

| Package | Ready? | Blocker |
|---------|--------|---------|
| `pkg/bencode` | Almost | Missing doc comments |
| `pkg/torrent` | Almost | Missing doc comments |
| `pkg/tracker` | Almost | Missing doc comments |
| `pkg/peer` | Almost | Missing doc comments, mutable exported fields |
| `pkg/piece` | Almost | Missing doc comments |
| `pkg/storage` | Almost | Missing doc comments |
| `pkg/download` | **Not ready** | `log.Printf`, no doc comments, hardcoded config, god object |

## Phases

### Phase 1: Fix `pkg/download` (Critical)

**Remove `log.Printf` from library code** — 22 calls in `session.go`. Library code must not write to stdout/stderr.

Options (pick one):
- **Log callback**: Add a `func(event, msg string, args ...any)` field to `Session` that the caller sets (e.g., to `log.Printf` in the CLI, to `nil` for silent use).
- **Event channel**: Return structured progress/error events via a channel the caller reads.
- **Logger interface**: Accept an `interface{ Printf(string, ...any) }` via functional options.

**Make configuration injectable:**

| Hardcoded value | Line | Should be |
|----------------|------|-----------|
| `numWorkers = 30` | `New()` | Option |
| Peer channel buffer `200` | `Run()` | Option |
| Block size `16384` | `downloadPiece()` | Option |
| Pipeline depth `5` | `downloadPiece()` | Option |
| Read timeout `30s` | `readPeerMessages()` | Option |
| Unchoke timeout `30s` | `waitForUnchoke()` | Option |
| Dial timeout `10s` | `downloadFromPeer()` | Option |
| Piece response timeout `60s` | `downloadPiece()` | Option |

Use a `SessionConfig` struct or functional options pattern:

```go
type SessionConfig struct {
    Workers       int
    BlockSize     int
    PipelineDepth int
    ReadTimeout   time.Duration
    DialTimeout   time.Duration
    Logger        func(format string, args ...any)
}
```

**Unexport mutable fields on `Session`:**
- `Torrent`, `PieceMgr`, `Storage`, `PeerID`, `OutputDir`, `StopCh` — should be unexported or read-only accessors.

**Add doc comments** to `Session`, `New`, `Run`, `Stop`, `Resume`, `VerifyAll`.

### Phase 2: Doc comments everywhere (Medium)

Every exported type, function, const, and method across all 7 packages needs a doc comment for `go doc` and `pkg.go.dev`.

**~70 items total** — mechanical but tedious. Example threshold:

| Package | Items needing docs |
|---------|-------------------|
| `pkg/bencode` | `Value`, `Int`, `String`, `List`, `Dict`, `Decode`, `DecodeBytes`, `Encode`, `EncodeBytes`, `GetString`, `GetInt`, `GetList`, `GetDict`, `Bytes` |
| `pkg/torrent` | `TorrentFile`, `InfoDict`, `FileInfo`, `Parse`, `NumPieces`, `PieceHash`, `IsSingleFile`, `IsMultiFile`, `TotalSize`, `TrackerURL`, `IsTrackerless`, `PieceLength` |
| `pkg/tracker` | `TrackerClient`, `AnnounceRequest`, `AnnounceResponse`, `Peer`, `NewTrackerClient`, `Announce` |
| `pkg/peer` | `PeerConn`, `Handshake`, `MessageID`, `MsgChoke`..`MsgPort`, `Message`, `Dial`, `ReadMessage`, `SendMessage`, `Close`, `SetReadTimeout`, `RemoteAddr`, `IsClosed`, `NewHandshake`, `Marshal`, `Unmarshal`, `DoHandshake` |
| `pkg/piece` | `State`, `Missing`, `InProgress`, `Have`, `Manager`, `NewManager`, `NumPieces`, `Have`, `MarkDownloaded`, `MarkInProgress`, `UpdatePeerBitfield`, `RemovePeer`, `PeerHasPiece`, `ReleasePiece`, `Progress`, `Complete`, `CountPeerForPiece`, `PickPiece`, `MissingPieces`, `PieceLength` |
| `pkg/storage` | `Storage`, `FileInfo`, `New`, `IsSingleFile`, `VerifyPiece`, `WritePiece`, `ReadPiece`, `Exists` |
| `pkg/download` | `Session`, `New`, `Stop`, `VerifyAll`, `Resume`, `Run` |

### Phase 3: API hardening (Medium)

**Unexport mutable fields on value-types that shouldn't be mutated:**
- `PeerConn.InfoHash`, `PeerConn.PeerID`, `PeerConn.RemoteID`, `PeerConn.Choked`, `PeerConn.Interested` → unexport with accessors
- `TrackerClient.PeerID` → make read-only

**Sentinel errors** for common failure modes:
```go
var (
    ErrPieceNotFound  = errors.New("piece not found on disk")
    ErrPeerTimedOut   = errors.New("peer connection timed out")
    ErrHashMismatch   = errors.New("piece hash verification failed")
)
```

### Phase 4: Testing with mocks (Optional)

Current tests are integration-style (full storage, real piece managers). For a library, add:

- `peer_test.go` — mock TCP connections to test handshake/message edge cases
- `tracker_test.go` — mock HTTP server for announce response parsing
- `download/session_test.go` — mock piece manager + storage to test resume/orchestration logic

### Phase 5: `pkg.go.dev` and release

```bash
# After Phase 1-3 are done:
git tag v1.0.0
git push origin v1.0.0
# pkg.go.dev indexes automatically within 24h
# Verify: https://pkg.go.dev/github.com/apk471/bittorrent
```

Add example functions to packages for godoc:

```go
// Example usage in pkg/torrent/example_test.go
func ExampleParse() {
    data, _ := os.ReadFile("debian.torrent")
    tf, _ := Parse(data)
    fmt.Println(tf.Info.Name)
    // Output: debian-12.0.0-amd64-netinst.iso
}
```

## Summary

| Phase | Effort | Impact |
|-------|--------|--------|
| 1. Fix `log.Printf` + config injection | 1-2 days | Enables library use |
| 2. Add doc comments | 1 day | Makes API discoverable |
| 3. API hardening (unexport fields, sentinel errors) | 1 day | Prevents misuse |
| 4. Unit tests with mocks | 2-3 days | CI confidence |
| 5. Release v1.0.0 | 1 hour | Published library |
