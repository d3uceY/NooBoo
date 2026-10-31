package services

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nooboo/models"
)

const trayInstance = "0011223344556677"

func writeBytes(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func trayFile(ext string) string { return "0x00000001!0x" + trayInstance + "." + ext }

func protoString(field int, s string) []byte {
	b := []byte{byte(field<<3 | 2), byte(len(s))}
	return append(b, s...)
}

func writeTrayItem(t *testing.T, path, name, creator string) {
	t.Helper()
	body := append([]byte{}, make([]byte, trayHeaderBytes)...)
	body = append(body, protoString(4, name)...)
	body = append(body, protoString(7, creator)...)
	writeBytes(t, path, body)
}

func obfuscatedPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	out := make([]byte, trayImageHeader+len(body))
	binary.LittleEndian.PutUint64(out[:8], uint64(len(body)))
	for i, b := range body {
		out[trayImageHeader+i] = b ^ trayImageKey[i&7]
	}
	return out
}

func TestTrayZipRoutedToTray(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "house.zip")
	makeZip(t, src, map[string]string{
		trayFile("trayitem"):  "meta",
		trayFile("blueprint"): "lot",
	})

	res, err := NewModsService(cfg).InstallDownload(models.InstallRequest{Source: src, Name: "House"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != "tray" {
		t.Fatalf("target = %q, want tray", res.Target)
	}
	if !fileExists(filepath.Join(game, "Tray", trayFile("blueprint"))) {
		t.Fatal("the blueprint did not land in Tray")
	}
	if fileExists(filepath.Join(game, "Mods", "House")) {
		t.Fatal("a tray download was filed as a mod")
	}
}

func TestTraySingleItemPullsSiblings(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	dir := filepath.Join(base, "dl")
	writeTrayItem(t, filepath.Join(dir, trayFile("trayitem")), "Sunset House", "A Creator")
	write(t, filepath.Join(dir, trayFile("blueprint")), "lot")

	res, err := NewModsService(cfg).InstallDownload(models.InstallRequest{
		Source: filepath.Join(dir, trayFile("trayitem")),
		Name:   "Sunset House",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != "tray" || res.Name != "Sunset House" {
		t.Fatalf("result = %+v", res)
	}
	if !fileExists(filepath.Join(game, "Tray", trayFile("blueprint"))) {
		t.Fatal("the sibling file was left behind")
	}
}

func TestTrayScanReadsNameKindAndThumb(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	tray := filepath.Join(game, "Tray")

	writeTrayItem(t, filepath.Join(tray, trayFile("trayitem")), "Sunset House", "A Creator")
	write(t, filepath.Join(tray, trayFile("blueprint")), "lot")
	writeBytes(t, filepath.Join(tray, trayFile("bpi")), obfuscatedPNG(t))
	write(t, filepath.Join(tray, "0x00000001!0x00000000ABCDEF01.sgi"), "portrait")

	items, err := NewTrayService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(items), items)
	}
	item := items[0]
	if item.Name != "Sunset House" || item.Creator != "A Creator" {
		t.Errorf("name/creator = %q / %q", item.Name, item.Creator)
	}
	if item.Kind != "lot" {
		t.Errorf("kind = %q, want lot", item.Kind)
	}
	if !strings.HasPrefix(item.Thumb, "data:image/png;base64,") {
		t.Errorf("thumb = %.30q", item.Thumb)
	}
}

func TestTrayImageRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.bpi")
	writeBytes(t, path, make([]byte, trayImageHeader+4))
	if got := readTrayImage(path); got != nil {
		t.Fatalf("a zero header produced %d bytes", len(got))
	}
}

func TestInstallDownloadRoutesMods(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "Cool.package")
	write(t, src, "x")
	res, err := NewModsService(cfg).InstallDownload(models.InstallRequest{Source: src, Name: "Cool"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != "mods" || res.Name != "Cool" {
		t.Fatalf("result = %+v", res)
	}
}
