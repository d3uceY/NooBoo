package services

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testRes struct {
	typ, group uint32
	inst       uint64
	data       []byte
	comp       uint16
}

// writeDbpf builds a minimal v2 package: header, blobs, index.
func writeDbpf(t *testing.T, path string, res []testRes) {
	t.Helper()
	blobs := make([][]byte, len(res))
	offsets := make([]uint32, len(res))
	pos := uint32(96)
	for i, r := range res {
		data := r.data
		if r.comp == 0x5A42 {
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			if _, err := zw.Write(data); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			data = buf.Bytes()
		}
		blobs[i] = data
		offsets[i] = pos
		pos += uint32(len(data))
	}

	var index bytes.Buffer
	_ = binary.Write(&index, binary.LittleEndian, uint32(0)) // flags
	for i, r := range res {
		raw := uint32(len(blobs[i]))
		if r.comp != 0 {
			raw |= 0x80000000
		}
		_ = binary.Write(&index, binary.LittleEndian, r.typ)
		_ = binary.Write(&index, binary.LittleEndian, r.group)
		_ = binary.Write(&index, binary.LittleEndian, uint32(r.inst>>32))
		_ = binary.Write(&index, binary.LittleEndian, uint32(r.inst))
		_ = binary.Write(&index, binary.LittleEndian, offsets[i])
		_ = binary.Write(&index, binary.LittleEndian, raw)
		_ = binary.Write(&index, binary.LittleEndian, uint32(len(r.data)))
		if r.comp != 0 {
			_ = binary.Write(&index, binary.LittleEndian, r.comp)
			_ = binary.Write(&index, binary.LittleEndian, uint16(0))
		}
	}

	out := make([]byte, 96, 96+int(pos)-96+index.Len())
	copy(out, "DBPF")
	binary.LittleEndian.PutUint32(out[4:], 2)
	binary.LittleEndian.PutUint32(out[36:], uint32(len(res)))
	binary.LittleEndian.PutUint32(out[40:], pos)
	binary.LittleEndian.PutUint32(out[44:], uint32(index.Len()))
	binary.LittleEndian.PutUint32(out[64:], pos)
	for _, b := range blobs {
		out = append(out, b...)
	}
	out = append(out, index.Bytes()...)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDbpfIndexReadsEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.package")
	writeDbpf(t, path, []testRes{{typ: 0x034AEECB, group: 1, inst: 42, data: []byte("x")}})

	entries, err := readDbpfIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].Type != 0x034AEECB || entries[0].Instance != 42 || entries[0].Size != 1 {
		t.Fatalf("entry = %+v", entries[0])
	}
}

func TestDbpfReadsZlibResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "z.package")
	writeDbpf(t, path, []testRes{{typ: 1, inst: 1, data: []byte("hello"), comp: 0x5A42}})

	entries, err := readDbpfIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := string(readDbpfResource(f, entries[0], 1024)); got != "hello" {
		t.Fatalf("resource = %q", got)
	}
}

func TestDbpfRejectsTruncatedIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.package")
	writeDbpf(t, path, []testRes{{typ: 1, inst: 1, data: []byte("x")}})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(raw[44:], 1<<30)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDbpfIndex(path); err == nil {
		t.Fatal("a gigabyte index was accepted")
	}
}

func TestInsightMapsKind(t *testing.T) {
	dir := t.TempDir()
	cas := filepath.Join(dir, "cas.package")
	writeDbpf(t, cas, []testRes{{typ: 0x034AEECB, inst: 1, data: []byte("x")}})
	if got := scanPackage(cas); got == nil || got.Kind != "CAS" {
		t.Fatalf("CAS package read as %+v", got)
	}

	object := filepath.Join(dir, "object.package")
	writeDbpf(t, object, []testRes{{typ: 0xC0DB5AE7, inst: 1, data: []byte("x")}})
	if got := scanPackage(object); got == nil || got.Kind != "Build Buy" {
		t.Fatalf("object package read as %+v", got)
	}

	tuning := filepath.Join(dir, "tuning.package")
	writeDbpf(t, tuning, []testRes{{typ: 0x0333406C, inst: 1, data: []byte("x")}})
	if got := scanPackage(tuning); got == nil || got.Kind != "Gameplay" {
		t.Fatalf("tuning package read as %+v", got)
	}
}

func TestInsightFindsLargestThumbnail(t *testing.T) {
	small := encodePNG(t, 2, 2)
	large := encodePNG(t, 8, 8)
	path := filepath.Join(t.TempDir(), "thumb.package")
	writeDbpf(t, path, []testRes{
		{typ: 0x3C1AF1F2, inst: 1, data: small},
		{typ: 0x3C1AF1F2, inst: 2, data: large},
	})

	ins := scanPackage(path)
	if ins == nil || ins.Thumb == nil {
		t.Fatal("no thumbnail found")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(ins.Thumb))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 8 {
		t.Fatalf("kept a %dx%d image", cfg.Width, cfg.Height)
	}
}

func TestInsightDropsKeysOverCap(t *testing.T) {
	res := make([]testRes, maxInsightKeys+1)
	for i := range res {
		res[i] = testRes{typ: 1, inst: uint64(i), data: []byte{0}}
	}
	path := filepath.Join(t.TempDir(), "big.package")
	writeDbpf(t, path, res)

	ins := scanPackage(path)
	if ins == nil {
		t.Fatal("package did not parse")
	}
	if len(ins.Keys) != 0 {
		t.Fatalf("kept %d keys over the cap", len(ins.Keys))
	}
}

func TestInsightCacheInvalidatesOnChange(t *testing.T) {
	cfg, base := newTestConfig(t)
	path := filepath.Join(base, "a.package")
	writeDbpf(t, path, []testRes{{typ: 0x034AEECB, inst: 1, data: []byte("x")}})

	svc := NewModsService(cfg)
	info, _ := os.Stat(path)
	first := svc.insightForPath(path, info.Size(), info.ModTime())
	if first == nil || first.Kind != "CAS" {
		t.Fatalf("first read = %+v", first)
	}

	writeDbpf(t, path, []testRes{{typ: 0xC0DB5AE7, inst: 1, data: []byte("x")}})
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	second := svc.insightForPath(path, info.Size(), info.ModTime())
	if second == nil || second.Kind != "Build Buy" {
		t.Fatalf("second read = %+v", second)
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestThumbnailReturnsDataURL(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	writeDbpf(t, filepath.Join(game, "Mods", "Thumb", "x.package"), []testRes{
		{typ: 0x3C1AF1F2, inst: 1, data: encodePNG(t, 2, 2)},
	})

	svc := NewModsService(cfg)
	if _, err := svc.Scan(); err != nil {
		t.Fatal(err)
	}
	if got := svc.Thumbnail("Thumb"); !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("thumb = %.40q", got)
	}
}
