package services

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Builds a file laid out the way the game writes a tray thumbnail, byte by byte,
// so the decoder's assumptions are visible rather than implied.
func TestReadTrayImageLayout(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	imageBytes := encoded.Bytes()

	file := make([]byte, trayImageHeader+len(imageBytes))
	binary.LittleEndian.PutUint64(file[:8], uint64(len(imageBytes))) // 8 bytes: image length
	// bytes 8..23 are the 16-byte padding the game writes; left zero here.
	for i, b := range imageBytes {
		file[trayImageHeader+i] = b ^ trayImageKey[i&7] // scrambled with the key
	}

	path := filepath.Join(t.TempDir(), "x.bpi")
	if err := os.WriteFile(path, file, 0o644); err != nil {
		t.Fatal(err)
	}

	got := readTrayImage(path)
	if !bytes.Equal(got, imageBytes) {
		t.Fatalf("decoded %d bytes, want %d", len(got), len(imageBytes))
	}
}
