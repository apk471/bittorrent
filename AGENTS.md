# Repository Guidelines

## Project Overview

This repository is a BitTorrent client library and demo CLI written in Go.
The module path is `github.com/ayush-amin/bittorrent`.

Core package responsibilities:

- `pkg/bencode`: bencode values plus encode/decode logic.
- `pkg/torrent`: `.torrent` parsing, info hash calculation, and metadata helpers.
- `pkg/tracker`: HTTP tracker announce requests and peer response parsing.
- `pkg/peer`: peer wire protocol, handshakes, messages, and connection behavior.
- `pkg/piece`: piece state, rarest-first selection, and block tracking.
- `pkg/storage`: file layout, sparse writes, piece reads, and SHA-1 verification.
- `pkg/download`: session orchestration across trackers, peers, storage, and pieces.
- `cmd/btdemo`: CLI entry point and demo/test torrent generation.

## Common Commands

Use the Makefile targets where possible:

```bash
make build
make test
make lint
make release
make clean
```

Direct Go commands are also fine for scoped work:

```bash
go test ./...
go test ./pkg/bencode
go test ./pkg/tracker -run TestName
go vet ./...
go run ./cmd/btdemo --version
go run ./cmd/btdemo --debug leaves.torrent
```

`make build` writes `build/bittorrent`. Do not commit generated build outputs.

## Development Practices

- Keep package boundaries clear. Protocol parsing belongs in the protocol package; orchestration belongs in `pkg/download`.
- Prefer small, testable helpers for binary/protocol code. Avoid hiding protocol edge cases in large control-flow blocks.
- Preserve exact byte semantics for bencode, torrent info hashes, tracker compact peer lists, handshakes, and peer messages.
- Use `fmt.Errorf("context: %w", err)` when wrapping errors.
- Keep network-facing code bounded with timeouts and cancellation/stop-channel behavior.
- Be careful with file paths in `pkg/storage`; torrent paths can come from untrusted metadata.
- Avoid changing public exported types or method behavior unless the caller impact is intentional.
- Run `gofmt` on changed Go files before testing.

## Testing Guidance

- Add or update unit tests next to the package being changed.
- For parser changes, include malformed input, boundary lengths, empty values, and valid fixtures.
- For tracker and peer protocol changes, test both compact/binary forms and dictionary/message forms where applicable.
- For piece and storage behavior, cover final-piece sizing, resume/verification paths, and multi-file layout cases.
- For CLI changes, keep parsing behavior simple and verify commands with `go run ./cmd/btdemo ...` when practical.

Before opening a PR, run:

```bash
make test
make lint
```

If a command cannot be run locally, note that in the PR.

## PR Notes

- Keep changes focused and describe behavior changes in concrete terms.
- Mention tests run, especially for protocol, storage, or download-session changes.
- Do not include downloaded torrent payloads, generated binaries, or local output directories.
- Existing sample `.torrent` fixtures may be used for debugging, but avoid adding large fixtures.
