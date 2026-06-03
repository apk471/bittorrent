package storage

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage struct {
	basePath string
	name     string
	length   int64
	files    []FileInfo
}

type FileInfo struct {
	Length int64
	Path   []string
}

func New(basePath, name string, length int64, files []FileInfo) (*Storage, error) {
	s := &Storage{
		basePath: basePath,
		name:     name,
		length:   length,
		files:    files,
	}
	if err := os.MkdirAll(s.dir(), 0755); err != nil {
		return nil, fmt.Errorf("storage: create dir: %w", err)
	}
	return s, nil
}

func (s *Storage) dir() string {
	return filepath.Join(s.basePath, s.name)
}

func (s *Storage) IsSingleFile() bool {
	return s.length > 0
}

func (s *Storage) VerifyPiece(data []byte, expectedHash [20]byte) bool {
	h := sha1.Sum(data)
	return h == expectedHash
}

func (s *Storage) WritePiece(index int, data []byte, pieceLength int64) error {
	offset := int64(index) * pieceLength

	if s.IsSingleFile() {
		return s.writeSingleFile(offset, data)
	}
	return s.writeMultiFile(offset, data)
}

func (s *Storage) writeSingleFile(offset int64, data []byte) error {
	fpath := filepath.Join(s.dir(), s.name)
	f, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("storage: open file: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return fmt.Errorf("storage: seek: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("storage: write: %w", err)
	}
	return nil
}

func (s *Storage) writeMultiFile(offset int64, data []byte) error {
	written := int64(0)
	dataLen := int64(len(data))

	for _, fi := range s.files {
		fileOffset := int64(0)
		fpath := filepath.Join(s.dir(), filepath.Join(fi.Path...))

		if offset >= written+fi.Length {
			written += fi.Length
			continue
		}

		if offset > written {
			fileOffset = offset - written
		}

		writeLen := fi.Length - fileOffset
		if dataLen-written < writeLen {
			writeLen = dataLen - written
		}
		if writeLen <= 0 {
			written += fi.Length
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return fmt.Errorf("storage: mkdir: %w", err)
		}

		f, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("storage: open file: %w", err)
		}

		if _, err := f.Seek(fileOffset, 0); err != nil {
			f.Close()
			return fmt.Errorf("storage: seek: %w", err)
		}
		if _, err := f.Write(data[written : written+writeLen]); err != nil {
			f.Close()
			return fmt.Errorf("storage: write: %w", err)
		}
		f.Close()

		written += writeLen
		if written >= dataLen {
			break
		}
	}

	return nil
}

func (s *Storage) ReadPiece(index int, pieceLength int64) ([]byte, error) {
	offset := int64(index) * pieceLength
	if s.IsSingleFile() {
		return s.readSingleFile(offset, pieceLength)
	}
	return s.readMultiFile(offset, pieceLength)
}

func (s *Storage) readSingleFile(offset, size int64) ([]byte, error) {
	fpath := filepath.Join(s.dir(), s.name)
	f, err := os.Open(fpath)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	defer f.Close()

	data := make([]byte, size)
	n, err := f.ReadAt(data, offset)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("storage: read: %w", err)
	}
	return data[:n], nil
}

func (s *Storage) readMultiFile(offset, size int64) ([]byte, error) {
	data := make([]byte, size)
	read := int64(0)

	for _, fi := range s.files {
		if read >= size {
			break
		}
		fpath := filepath.Join(s.dir(), filepath.Join(fi.Path...))
		fileStart := read
		fileEnd := read + fi.Length

		if offset >= fileEnd {
			read += fi.Length
			continue
		}
		if offset+size <= fileStart {
			break
		}

		overlapStart := offset
		if overlapStart < fileStart {
			overlapStart = fileStart
		}
		overlapEnd := offset + size
		if overlapEnd > fileEnd {
			overlapEnd = fileEnd
		}

		if overlapStart >= overlapEnd {
			read += fi.Length
			continue
		}

		fileReadOffset := overlapStart - fileStart
		dataStart := overlapStart - offset
		dataEnd := overlapEnd - offset

		f, err := os.Open(fpath)
		if err != nil {
			return nil, fmt.Errorf("storage: open %s: %w", fpath, err)
		}

		if _, err := f.ReadAt(data[dataStart:dataEnd], fileReadOffset); err != nil {
			f.Close()
			return nil, fmt.Errorf("storage: read %s: %w", fpath, err)
		}
		f.Close()

		read += fi.Length
	}

	return data, nil
}

func (s *Storage) Exists() bool {
	if s.IsSingleFile() {
		fpath := filepath.Join(s.dir(), s.name)
		_, err := os.Stat(fpath)
		return err == nil
	}
	for _, fi := range s.files {
		fpath := filepath.Join(s.dir(), filepath.Join(fi.Path...))
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			return false
		}
	}
	return true
}
