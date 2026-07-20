package torrent

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/ayush-amin/bittorrent/pkg/bencode"
)

// TorrentFile represents a parsed .torrent file.
type TorrentFile struct {
	Announce     string
	AnnounceList [][]string
	CreatedBy    string
	CreationDate int64
	Comment      string
	Info         InfoDict
	InfoHash     [20]byte
}

// InfoDict is the info dictionary from a torrent file.
type InfoDict struct {
	Name        string
	PieceLength int64
	Pieces      []byte
	Length      int64
	Files       []FileInfo
}

// FileInfo describes a single file in a multi-file torrent.
type FileInfo struct {
	Length int64
	Path   []string
}

// Parse decodes a .torrent file and returns a TorrentFile.
func Parse(data []byte) (*TorrentFile, error) {
	v, err := bencode.DecodeBytes(data)
	if err != nil {
		return nil, fmt.Errorf("torrent: decode: %w", err)
	}

	root, ok := v.(bencode.Dict)
	if !ok {
		return nil, fmt.Errorf("torrent: expected top-level dict")
	}

	t := &TorrentFile{}

	if announce, err := root.GetString("announce"); err == nil {
		t.Announce = string(announce)
	}

	if announceList, err := root.GetList("announce-list"); err == nil {
		t.AnnounceList = make([][]string, 0, len(announceList))
		for _, tier := range announceList {
			tierList, ok := tier.(bencode.List)
			if !ok {
				continue
			}
			urls := make([]string, 0, len(tierList))
			for _, u := range tierList {
				if s, ok := u.(bencode.String); ok {
					urls = append(urls, string(s))
				}
			}
			if len(urls) > 0 {
				t.AnnounceList = append(t.AnnounceList, urls)
			}
		}
	}

	if createdBy, err := root.GetString("created by"); err == nil {
		t.CreatedBy = string(createdBy)
	}

	if creationDate, err := root.GetInt("creation date"); err == nil {
		t.CreationDate = int64(creationDate)
	}

	if comment, err := root.GetString("comment"); err == nil {
		t.Comment = string(comment)
	}

	infoVal, ok := root["info"]
	if !ok {
		return nil, fmt.Errorf("torrent: missing info dict")
	}
	infoDict, ok := infoVal.(bencode.Dict)
	if !ok {
		return nil, fmt.Errorf("torrent: info is not a dict")
	}

	infoBytes, err := bencode.EncodeBytes(infoVal)
	if err != nil {
		return nil, fmt.Errorf("torrent: re-encode info: %w", err)
	}
	t.InfoHash = sha1.Sum(infoBytes)

	info := InfoDict{}

	if name, err := infoDict.GetString("name"); err == nil {
		info.Name = string(name)
	} else {
		return nil, fmt.Errorf("torrent: info missing name: %w", err)
	}

	if pieceLength, err := infoDict.GetInt("piece length"); err == nil {
		info.PieceLength = int64(pieceLength)
	} else {
		return nil, fmt.Errorf("torrent: info missing piece length: %w", err)
	}

	if pieces, err := infoDict.GetString("pieces"); err == nil {
		info.Pieces = []byte(pieces)
		if len(info.Pieces)%20 != 0 {
			return nil, fmt.Errorf("torrent: invalid pieces length %d (must be multiple of 20)", len(info.Pieces))
		}
	} else {
		return nil, fmt.Errorf("torrent: info missing pieces: %w", err)
	}

	if length, err := infoDict.GetInt("length"); err == nil {
		info.Length = int64(length)
	}

	if filesVal, err := infoDict.GetList("files"); err == nil {
		info.Files = make([]FileInfo, 0, len(filesVal))
		for _, fv := range filesVal {
			fd, ok := fv.(bencode.Dict)
			if !ok {
				continue
			}
			fi := FileInfo{}

			if length, err := fd.GetInt("length"); err == nil {
				fi.Length = int64(length)
			}

			if pathVal, err := fd.GetList("path"); err == nil {
				fi.Path = make([]string, 0, len(pathVal))
				for _, p := range pathVal {
					if s, ok := p.(bencode.String); ok {
						fi.Path = append(fi.Path, string(s))
					}
				}
			}

			info.Files = append(info.Files, fi)
		}
	}

	t.Info = info
	return t, nil
}

// NumPieces returns the total number of pieces.
func (t *TorrentFile) NumPieces() int {
	return len(t.Info.Pieces) / 20
}

// PieceHash returns the SHA-1 hash for the piece at the given index.
func (t *TorrentFile) PieceHash(index int) [20]byte {
	var hash [20]byte
	if index < 0 || index >= t.NumPieces() {
		return hash
	}
	start := index * 20
	copy(hash[:], t.Info.Pieces[start:start+20])
	return hash
}

// IsSingleFile returns true if this is a single-file torrent.
func (t *TorrentFile) IsSingleFile() bool {
	return t.Info.Length > 0
}

// IsMultiFile returns true if this is a multi-file torrent.
func (t *TorrentFile) IsMultiFile() bool {
	return len(t.Info.Files) > 0
}

// TotalSize returns the total download size in bytes.
func (t *TorrentFile) TotalSize() int64 {
	if t.IsSingleFile() {
		return t.Info.Length
	}
	var total int64
	for _, f := range t.Info.Files {
		total += f.Length
	}
	return total
}

// TrackerURL returns the primary tracker announce URL.
func (t *TorrentFile) TrackerURL() string {
	if t.Announce != "" {
		return t.Announce
	}
	if len(t.AnnounceList) > 0 && len(t.AnnounceList[0]) > 0 {
		return t.AnnounceList[0][0]
	}
	return ""
}

// HTTPTrackers returns all unique http/https announce URLs from the
// Announce field and AnnounceList, in order. UDP trackers are skipped
// because the tracker client only speaks HTTP.
func (t *TorrentFile) HTTPTrackers() []string {
	seen := make(map[string]struct{})
	var urls []string
	add := func(u string) {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	add(t.Announce)
	for _, tier := range t.AnnounceList {
		for _, u := range tier {
			add(u)
		}
	}
	return urls
}

// IsTrackerless returns true if no tracker URL is available.
func (t *TorrentFile) IsTrackerless() bool {
	return t.TrackerURL() == ""
}

// PieceLength returns the length of the piece at the given index.
// The last piece may be shorter than PieceLength.
func (t *TorrentFile) PieceLength(index int) int64 {
	num := t.NumPieces()
	if index < 0 || index >= num {
		return 0
	}
	if index < num-1 {
		return t.Info.PieceLength
	}
	last := t.TotalSize() - (int64(num)-1)*t.Info.PieceLength
	if last <= 0 {
		return t.Info.PieceLength
	}
	return last
}
