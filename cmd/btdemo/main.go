package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/apk471/bittorrent/pkg/download"
	"github.com/apk471/bittorrent/pkg/torrent"
)

var version = "dev"

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

	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage:\n  %s --version           Show version\n  %s --gen [path]        Generate test torrent\n  %s --verify <torrent> <dir>   Verify pieces\n  %s --debug <torrent>   Show metadata\n  %s <torrent> [dir]     Download\n", os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
		os.Exit(1)
	}

	if os.Args[1] == "--verify" {
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Usage: %s --verify <file.torrent> <output-dir>\n", os.Args[0])
			os.Exit(1)
		}
		verifyCmd(os.Args[2], os.Args[3])
		return
	}

	if os.Args[1] == "--debug" {
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: %s --debug <file.torrent>\n", os.Args[0])
			os.Exit(1)
		}
		debugCmd(os.Args[2])
		return
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

	outputDir := "."
	if len(os.Args) > 2 {
		outputDir = os.Args[2]
	}

	fmt.Printf("Name:        %s\n", tf.Info.Name)
	if u := tf.TrackerURL(); u != "" {
		fmt.Printf("Tracker:     %s\n", u)
	} else {
		fmt.Printf("Tracker:     (trackerless - DHT)\n")
	}
	if tf.IsSingleFile() {
		fmt.Printf("Size:        %s\n", formatSize(tf.TotalSize()))
	} else {
		fmt.Printf("Size:        %s (%d files)\n", formatSize(tf.TotalSize()), len(tf.Info.Files))
	}
	fmt.Printf("Pieces:      %d (%s each)\n", tf.NumPieces(), formatSize(tf.Info.PieceLength))
	fmt.Printf("Info Hash:   %x\n", tf.InfoHash)
	fmt.Printf("Output:      %s\n", outputDir)

	if tf.IsTrackerless() {
		fmt.Println("\nTrackerless torrents not supported yet.")
		return
	}

	fmt.Println("\nStarting download...")
	sess, err := download.New(tf, outputDir)
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		sess.Stop()
	}()

	if err := sess.Run(); err != nil {
		log.Printf("Download incomplete: %v", err)
		os.Exit(1)
	}

	fmt.Println("\nDownload complete!")
}

func debugCmd(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	tf, err := torrent.Parse(data)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PieceLength: %d (%s)\n", tf.Info.PieceLength, formatSize(tf.Info.PieceLength))
	fmt.Printf("NumPieces: %d\n", tf.NumPieces())
	fmt.Printf("TotalSize: %d (%s)\n", tf.TotalSize(), formatSize(tf.TotalSize()))
	last := tf.PieceLength(tf.NumPieces() - 1)
	fmt.Printf("LastPieceLength: %d\n", last)
	computedTotal := int64(tf.NumPieces()-1)*tf.Info.PieceLength + last
	fmt.Printf("Computed total from pieces: %d\n", computedTotal)
	fmt.Printf("Expected: %d pieces\n", (tf.TotalSize()+tf.Info.PieceLength-1)/tf.Info.PieceLength)
	for i := 0; i < tf.NumPieces(); i++ {
		pl := tf.PieceLength(i)
		if int64(i)*tf.Info.PieceLength+pl > tf.TotalSize() {
			fmt.Printf("ERROR: piece %d would exceed file (offset=%d, len=%d, total=%d)\n", i, i*int(tf.Info.PieceLength), pl, tf.TotalSize())
		}
	}
}

func verifyCmd(torrentPath, outputDir string) {
	data, err := os.ReadFile(torrentPath)
	if err != nil {
		log.Fatalf("Error reading torrent: %v", err)
	}
	tf, err := torrent.Parse(data)
	if err != nil {
		log.Fatalf("Error parsing torrent: %v", err)
	}

	sess, err := download.New(tf, outputDir)
	if err != nil {
		log.Fatalf("Error creating session: %v", err)
	}

	fmt.Println("Verifying downloaded pieces...")
	total, checked, failed, err := sess.VerifyAll()
	if err != nil {
		log.Fatalf("Verification error: %v", err)
	}
	if failed > 0 {
		fmt.Printf("FAILED: %d/%d checked pieces have hash mismatches (%d total pieces)\n", failed, checked, total)
		os.Exit(1)
	}
	fmt.Printf("OK: %d/%d pieces verified successfully (%d unchecked)\n", checked, total, total-checked)
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
