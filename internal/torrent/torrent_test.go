package torrent

import (
	"crypto/sha1"
	"testing"

	"github.com/ayushamin/bittorent/internal/bencode"
)

func makePieces(count int) []byte {
	pieces := make([]byte, count*20)
	for i := range count {
		h := sha1.Sum([]byte{byte(i)})
		copy(pieces[i*20:], h[:])
	}
	return pieces
}

func makeSingleFileTorrent() []byte {
	pieces := makePieces(4)
	info := bencode.Dict{
		"name":         bencode.String("ubuntu.iso"),
		"piece length": bencode.Int(262144),
		"length":       bencode.Int(1048576),
		"pieces":       bencode.String(pieces),
	}
	root := bencode.Dict{
		"announce": bencode.String("http://tracker.example.com/announce"),
		"info":     info,
	}
	data, err := bencode.EncodeBytes(root)
	if err != nil {
		panic(err)
	}
	return data
}

func makeMultiFileTorrent() []byte {
	pieces := makePieces(3)
	info := bencode.Dict{
		"name":         bencode.String("my-files"),
		"piece length": bencode.Int(65536),
		"pieces":       bencode.String(pieces),
		"files": bencode.List{
			bencode.Dict{
				"length": bencode.Int(100000),
				"path":   bencode.List{bencode.String("dir1"), bencode.String("file1.txt")},
			},
			bencode.Dict{
				"length": bencode.Int(50000),
				"path":   bencode.List{bencode.String("file2.txt")},
			},
		},
	}
	root := bencode.Dict{
		"announce":      bencode.String("http://tracker2.example.com/announce"),
		"announce-list": bencode.List{bencode.List{bencode.String("http://tracker2.example.com/announce"), bencode.String("udp://tracker2.example.com:6969")}},
		"created by":    bencode.String("TestMaker 1.0"),
		"creation date": bencode.Int(1700000000),
		"comment":       bencode.String("This is a test torrent"),
		"info":          info,
	}
	data, err := bencode.EncodeBytes(root)
	if err != nil {
		panic(err)
	}
	return data
}

func TestParseSingleFile(t *testing.T) {
	data := makeSingleFileTorrent()
	tf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	if tf.Announce != "http://tracker.example.com/announce" {
		t.Fatalf("expected announce URL, got %q", tf.Announce)
	}

	if tf.Info.Name != "ubuntu.iso" {
		t.Fatalf("expected name 'ubuntu.iso', got %q", tf.Info.Name)
	}

	if tf.Info.PieceLength != 262144 {
		t.Fatalf("expected piece length 262144, got %d", tf.Info.PieceLength)
	}

	if tf.Info.Length != 1048576 {
		t.Fatalf("expected length 1048576, got %d", tf.Info.Length)
	}

	if tf.NumPieces() != 4 {
		t.Fatalf("expected 4 pieces, got %d", tf.NumPieces())
	}

	if !tf.IsSingleFile() {
		t.Fatal("expected single file mode")
	}

	if tf.IsMultiFile() {
		t.Fatal("expected not multi-file mode")
	}

	if tf.TotalSize() != 1048576 {
		t.Fatalf("expected total size 1048576, got %d", tf.TotalSize())
	}

	pieceLen := tf.PieceLength(0)
	if pieceLen != 262144 {
		t.Fatalf("expected first piece length 262144, got %d", pieceLen)
	}

	lastPieceLen := tf.PieceLength(3)
	if lastPieceLen != 262144 {
		t.Fatalf("expected last piece length 262144, got %d", lastPieceLen)
	}

	hash := tf.PieceHash(0)
	if hash == ([20]byte{}) {
		t.Fatal("expected valid piece hash")
	}

	if tf.InfoHash == ([20]byte{}) {
		t.Fatal("expected valid info hash")
	}
}

func TestParseMultiFile(t *testing.T) {
	data := makeMultiFileTorrent()
	tf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	if tf.Announce != "http://tracker2.example.com/announce" {
		t.Fatalf("expected announce URL, got %q", tf.Announce)
	}

	if len(tf.AnnounceList) != 1 {
		t.Fatalf("expected 1 announce-list tier, got %d", len(tf.AnnounceList))
	}
	urls := tf.AnnounceList[0]
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs in tier, got %d", len(urls))
	}
	if urls[1] != "udp://tracker2.example.com:6969" {
		t.Fatalf("expected second URL, got %q", urls[1])
	}

	if tf.CreatedBy != "TestMaker 1.0" {
		t.Fatalf("expected 'TestMaker 1.0', got %q", tf.CreatedBy)
	}

	if tf.CreationDate != 1700000000 {
		t.Fatalf("expected creation date 1700000000, got %d", tf.CreationDate)
	}

	if tf.Comment != "This is a test torrent" {
		t.Fatalf("unexpected comment, got %q", tf.Comment)
	}

	if tf.Info.Name != "my-files" {
		t.Fatalf("expected name 'my-files', got %q", tf.Info.Name)
	}

	if !tf.IsMultiFile() {
		t.Fatal("expected multi-file mode")
	}

	if tf.IsSingleFile() {
		t.Fatal("expected not single-file mode")
	}

	if len(tf.Info.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(tf.Info.Files))
	}

	f1 := tf.Info.Files[0]
	if f1.Length != 100000 {
		t.Fatalf("expected file1 length 100000, got %d", f1.Length)
	}
	if len(f1.Path) != 2 || f1.Path[0] != "dir1" || f1.Path[1] != "file1.txt" {
		t.Fatalf("expected file1 path [dir1, file1.txt], got %v", f1.Path)
	}

	f2 := tf.Info.Files[1]
	if f2.Length != 50000 {
		t.Fatalf("expected file2 length 50000, got %d", f2.Length)
	}
	if len(f2.Path) != 1 || f2.Path[0] != "file2.txt" {
		t.Fatalf("expected file2 path [file2.txt], got %v", f2.Path)
	}

	if tf.TotalSize() != 150000 {
		t.Fatalf("expected total size 150000, got %d", tf.TotalSize())
	}

	if tf.NumPieces() != 3 {
		t.Fatalf("expected 3 pieces, got %d", tf.NumPieces())
	}
}

func TestParseMissingInfo(t *testing.T) {
	root := bencode.Dict{
		"announce": bencode.String("http://tracker.com/announce"),
	}
	data, _ := bencode.EncodeBytes(root)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing info dict")
	}
}

func TestParseInvalidTopLevel(t *testing.T) {
	_, err := Parse([]byte("4:spam"))
	if err == nil {
		t.Fatal("expected error for non-dict top-level")
	}
}

func TestParseInvalidPieces(t *testing.T) {
	info := bencode.Dict{
		"name":         bencode.String("test"),
		"piece length": bencode.Int(256),
		"length":       bencode.Int(1000),
		"pieces":       bencode.String("21-byte-string-here!!"),
	}
	root := bencode.Dict{
		"announce": bencode.String("http://tracker.com/announce"),
		"info":     info,
	}
	data, _ := bencode.EncodeBytes(root)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for invalid pieces length")
	}
}

func TestParseMissingName(t *testing.T) {
	pieces := makePieces(1)
	info := bencode.Dict{
		"piece length": bencode.Int(256),
		"length":       bencode.Int(1000),
		"pieces":       bencode.String(pieces),
	}
	root := bencode.Dict{
		"announce": bencode.String("http://tracker.com/announce"),
		"info":     info,
	}
	data, _ := bencode.EncodeBytes(root)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestInfoHash(t *testing.T) {
	data := makeSingleFileTorrent()
	tf1, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	tf2, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	if tf1.InfoHash != tf2.InfoHash {
		t.Fatal("info hash should be deterministic")
	}
}

func TestPieceHashValues(t *testing.T) {
	data := makeSingleFileTorrent()
	tf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	for i := range tf.NumPieces() {
		h := tf.PieceHash(i)
		if h == ([20]byte{}) {
			t.Fatalf("piece %d hash is zero", i)
		}
	}
}

func TestOutOfRangePieceHash(t *testing.T) {
	data := makeSingleFileTorrent()
	tf, _ := Parse(data)

	hash := tf.PieceHash(999)
	if hash != ([20]byte{}) {
		t.Fatal("expected zero hash for out-of-range piece")
	}
}

func TestPieceLengthSmallFile(t *testing.T) {
	pieces := makePieces(1)
	info := bencode.Dict{
		"name":         bencode.String("small.bin"),
		"piece length": bencode.Int(65536),
		"length":       bencode.Int(100),
		"pieces":       bencode.String(pieces),
	}
	root := bencode.Dict{
		"announce": bencode.String("http://t.com/a"),
		"info":     info,
	}
	data, _ := bencode.EncodeBytes(root)
	tf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if tf.NumPieces() != 1 {
		t.Fatalf("expected 1 piece, got %d", tf.NumPieces())
	}
	if tf.PieceLength(0) != 100 {
		t.Fatalf("expected piece length 100, got %d", tf.PieceLength(0))
	}
}
