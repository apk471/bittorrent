package storage

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyPiece(t *testing.T) {
	data := []byte("hello world this is a test piece of data")
	hash := sha1.Sum(data)

	s, err := New(t.TempDir(), "test", 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !s.VerifyPiece(data, hash) {
		t.Fatal("expected valid hash")
	}

	wrongHash := sha1.Sum([]byte("wrong data"))
	if s.VerifyPiece(data, wrongHash) {
		t.Fatal("expected invalid hash")
	}
}

func TestWriteSingleFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello world this is piece zero data!!")
	pieceLen := int64(len(data))

	s, err := New(dir, "downloads", int64(len(data)), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.WritePiece(0, data, pieceLen); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "downloads", "downloads"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(data) {
		t.Fatalf("expected %q, got %q", data, content)
	}
}

func TestWriteMultiplePieces(t *testing.T) {
	dir := t.TempDir()
	pieceLen := int64(16)
	totalSize := int64(48)

	piece0 := []byte("AAAAAAAABBBBBBBB")
	piece1 := []byte("CCCCCCCCDDDDDDDD")
	piece2 := []byte("EEEEEEEEFFFFFFFF")

	s, err := New(dir, "downloads", totalSize, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.WritePiece(0, piece0, pieceLen)
	s.WritePiece(1, piece1, pieceLen)
	s.WritePiece(2, piece2, pieceLen)

	content, err := os.ReadFile(filepath.Join(dir, "downloads", "downloads"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "AAAAAAAABBBBBBBBCCCCCCCCDDDDDDDDEEEEEEEEFFFFFFFF"
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, content)
	}
}

func TestWriteMultiFile(t *testing.T) {
	dir := t.TempDir()

	files := []FileInfo{
		{Length: 10, Path: []string{"dir1", "file1.txt"}},
		{Length: 10, Path: []string{"file2.txt"}},
	}

	s, err := New(dir, "multifile", 0, files)
	if err != nil {
		t.Fatal(err)
	}

	pieceLen := int64(20)
	piece0 := []byte("AAAAAAAAAABBBBBBBBBB")

	if err := s.WritePiece(0, piece0, pieceLen); err != nil {
		t.Fatal(err)
	}

	c1, err := os.ReadFile(filepath.Join(dir, "multifile", "dir1", "file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(c1) != "AAAAAAAAAA" {
		t.Fatalf("expected 'AAAAAAAAAA', got %q", c1)
	}

	c2, err := os.ReadFile(filepath.Join(dir, "multifile", "file2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(c2) != "BBBBBBBBBB" {
		t.Fatalf("expected 'BBBBBBBBBB', got %q", c2)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "newfile", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Exists() {
		t.Fatal("expected file to not exist yet")
	}

	s.WritePiece(0, make([]byte, 100), 100)
	if !s.Exists() {
		t.Fatal("expected file to exist after write")
	}
}

func TestVerifyAfterWrite(t *testing.T) {
	dir := t.TempDir()
	data := []byte("verify me please")
	hash := sha1.Sum(data)
	pieceLen := int64(len(data))

	s, err := New(dir, "verify", int64(len(data)), nil)
	if err != nil {
		t.Fatal(err)
	}

	s.WritePiece(0, data, pieceLen)
	if !s.VerifyPiece(data, hash) {
		t.Fatal("verify should pass after write")
	}
}
