package main

import (
	"fmt"
	"os"

	"github.com/ayushamin/bittorent/internal/torrent"
	"github.com/ayushamin/bittorent/internal/tracker"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--gen" {
		path := "/tmp/test.torrent"
		if len(os.Args) > 2 {
			path = os.Args[2]
		}
		if err := generateTorrent(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\nNow run: go run ./cmd/btdemo/ /tmp/test.torrent")
		return
	}

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage:\n  %s --gen [path]     Generate test torrent\n  %s <file.torrent>   Parse and announce\n", os.Args[0], os.Args[0])
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	tf, err := torrent.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing torrent: %v\n", err)
		os.Exit(1)
	}

	trackerURL := tf.TrackerURL()

	fmt.Printf("Name:        %s\n", tf.Info.Name)
	if trackerURL != "" {
		fmt.Printf("Tracker:     %s\n", trackerURL)
	} else {
		fmt.Printf("Tracker:     (trackerless - DHT)\n")
	}
	if tf.IsSingleFile() {
		fmt.Printf("Size:        %d bytes\n", tf.TotalSize())
	} else {
		fmt.Printf("Size:        %d bytes (%d files)\n", tf.TotalSize(), len(tf.Info.Files))
		for _, f := range tf.Info.Files {
			fmt.Printf("  - %v (%d bytes)\n", f.Path, f.Length)
		}
	}
	fmt.Printf("Pieces:      %d (%d bytes each)\n", tf.NumPieces(), tf.Info.PieceLength)
	fmt.Printf("Info Hash:   %x\n", tf.InfoHash)
	if tf.CreatedBy != "" {
		fmt.Printf("Created By:  %s\n", tf.CreatedBy)
	}
	if tf.Comment != "" {
		fmt.Printf("Comment:     %s\n", tf.Comment)
	}

	if trackerURL == "" {
		fmt.Println("\nNo tracker URL found. DHT/PEX support not yet implemented.")
		return
	}

	client := tracker.NewTrackerClient()
	fmt.Printf("\nAnnouncing to %s ...\n", trackerURL)

	resp, err := client.Announce(trackerURL, &tracker.AnnounceRequest{
		InfoHash: tf.InfoHash,
		Port:     6881,
		Uploaded: 0,
		Left:     tf.TotalSize(),
		Event:    "started",
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Tracker error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Interval:    %ds\n", resp.Interval)
	fmt.Printf("Seeders:     %d\n", resp.Complete)
	fmt.Printf("Leechers:    %d\n", resp.Incomplete)
	fmt.Printf("Peers:       %d\n", len(resp.Peers))
	for i, p := range resp.Peers {
		fmt.Printf("  %d. %s:%d\n", i+1, p.IP, p.Port)
		if i >= 19 {
			fmt.Printf("  ... and %d more\n", len(resp.Peers)-20)
			break
		}
	}
}
