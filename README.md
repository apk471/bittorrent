# bittorrent

A BitTorrent client library and CLI tool built from scratch in Go.

## Install

```bash
go install github.com/ayush-amin/bittorrent/cmd/btdemo@latest
```

Or build from source:

```bash
git clone git@github.com:ayush-amin/bittorrent.git
cd bittorrent
make build
./build/bittorrent --version
```

## Usage

### CLI

```bash
# Download a torrent
btdemo ubuntu-26.04-desktop-amd64.iso.torrent /tmp/output

# Verify downloaded pieces
btdemo --verify ubuntu-26.04-desktop-amd64.iso.torrent /tmp/output

# Show torrent metadata
btdemo --debug ubuntu-26.04-desktop-amd64.iso.torrent
```

Resume is automatic — Ctrl+C and re-run continues where you left off.

### Library

```go
import (
    "github.com/ayush-amin/bittorrent/pkg/download"
    "github.com/ayush-amin/bittorrent/pkg/torrent"
)

data, _ := os.ReadFile("file.torrent")
tf, _ := torrent.Parse(data)

sess, _ := download.New(tf, "/tmp/output")
sess.Run()
```

## Features

- [x] Bencode encoding/decoding
- [x] Torrent file parsing (single-file, multi-file)
- [x] HTTP tracker announce (compact/dict peers, IPv4 + IPv6)
- [x] Peer wire protocol (all message types, keep-alive)
- [x] Rarest-first piece picking
- [x] Block pipelining (5 outstanding per connection)
- [x] Concurrent downloads (worker pool)
- [x] Storage engine with SHA-1 verification
- [x] Sparse file support
- [x] Resume support (disk hash verification on restart)
- [x] Graceful shutdown (SIGINT)
- [x] Tracker re-announce (started/stopped/completed events)
- [x] Cross-platform release builds

### Planned

- [ ] Endgame mode (duplicate requests near completion)
- [ ] DHT (trackerless torrents, BEP-5)
- [ ] Magnet links

## Packages

| Package | Description |
|---------|-------------|
| `pkg/bencode` | Bencode encoding and decoding |
| `pkg/torrent` | Torrent file parsing |
| `pkg/tracker` | HTTP tracker communication |
| `pkg/peer` | Peer wire protocol |
| `pkg/piece` | Piece state management |
| `pkg/storage` | File I/O with integrity checking |
| `pkg/download` | Session orchestration |

## Build

```bash
make build        # current platform
make test         # run tests
make lint         # go vet
make release      # cross-compile all targets
```

## License

MIT
