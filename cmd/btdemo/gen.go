package main

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ayush-amin/bittorrent/pkg/bencode"
)

func makePieces(count int) []byte {
	pieces := make([]byte, count*20)
	for i := range count {
		h := sha1.Sum([]byte{byte(i)})
		copy(pieces[i*20:], h[:])
	}
	return pieces
}

func generateTorrent(path string) error {
	pieces := makePieces(10)
	info := bencode.Dict{
		"name":         bencode.String("ubuntu-24.04-desktop.iso"),
		"piece length": bencode.Int(524288),
		"length":       bencode.Int(5242880),
		"pieces":       bencode.String(pieces),
	}
	root := bencode.Dict{
		"announce":      bencode.String("https://torrent.ubuntu.com/announce"),
		"announce-list": bencode.List{bencode.List{bencode.String("https://torrent.ubuntu.com/announce"), bencode.String("https://ipv6.torrent.ubuntu.com/announce")}},
		"created by":    bencode.String("bittorent-demo"),
		"creation date": bencode.Int(1717200000),
		"comment":       bencode.String("Demo torrent file for bittorent client testing"),
		"info":          info,
	}
	data, err := bencode.EncodeBytes(root)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Printf("Generated: %s (%d bytes)\n", path, len(data))
	return nil
}
