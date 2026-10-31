package services

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The Sims 4 packages are DBPF v2 archives: a header, the resource blobs, and
// a v2 index. Only the index and embedded thumbnails are read; resource data
// stays on disk. Everything is best-effort: a truncated file yields nil.

const (
	maxIndexBytes   = 32 << 20
	minIndexEntry   = 16
	maxInsightKeys  = 8192
	maxThumbBytes   = 256 << 10
	maxProbeCount   = 512
	probeByteBudget = 8 << 20
	goodEnoughArea  = 512 * 512
)

var errNotDbpf = errors.New("not a DBPF package")

type resourceKey struct {
	Type     uint32
	Group    uint32
	Instance uint64
}

func (k resourceKey) String() string {
	return fmt.Sprintf("%08x:%08x:%016x", k.Type, k.Group, k.Instance)
}

type dbpfEntry struct {
	Type        uint32
	Group       uint32
	Instance    uint64
	Offset      uint32
	Size        uint32
	MemSize     uint32
	Compression uint16
}

type packageInsight struct {
	Thumb     []byte
	Resources int
	Contents  map[string]int
	Keys      []resourceKey
	Kind      string
}

var thumbnailTypes = map[uint32]bool{
	0x3C1AF1F2: true,
	0x5B282D45: true,
	0xCD9DE247: true,
	0xE254AE6E: true,
	0x0D338A3A: true,
	0x16CCF748: true,
	0x3BD45407: true,
	0xE18CAEE2: true,
}

var dbpfTypeLabels = map[uint32]string{
	0x034AEECB: "CAS parts",
	0xC0DB5AE7: "objects",
	0x319E4F1D: "objects",
	0x0333406C: "tunings",
	0x545AC67A: "tunings",
	0x220557DA: "text tables",
	0x3453CF95: "textures",
	0x015A1849: "meshes",
}

func readDbpfIndex(path string) ([]dbpfEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	header := make([]byte, 96)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, errNotDbpf
	}
	if string(header[:4]) != "DBPF" {
		return nil, errNotDbpf
	}

	major := binary.LittleEndian.Uint32(header[4:])
	count := binary.LittleEndian.Uint32(header[36:])
	offsetV1 := binary.LittleEndian.Uint32(header[40:])
	indexSize := binary.LittleEndian.Uint32(header[44:])
	offsetV2 := binary.LittleEndian.Uint32(header[64:])

	if major < 2 || count == 0 || indexSize == 0 {
		return nil, errNotDbpf
	}
	offset := offsetV1
	if offsetV2 != 0 {
		offset = offsetV2
	}
	fileLen := st.Size()
	if offset < 96 || int64(offset) >= fileLen ||
		indexSize > maxIndexBytes || int64(indexSize) > fileLen-int64(offset) ||
		count > indexSize/minIndexEntry {
		return nil, errNotDbpf
	}

	index := make([]byte, indexSize)
	if _, err := f.ReadAt(index, int64(offset)); err != nil {
		return nil, errNotDbpf
	}
	entries := parseIndexV2(index, count)
	if len(entries) == 0 {
		return nil, errNotDbpf
	}
	return entries, nil
}

func parseIndexV2(b []byte, count uint32) []dbpfEntry {
	pos := 0
	u32 := func() (uint32, bool) {
		if pos+4 > len(b) {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b[pos:])
		pos += 4
		return v, true
	}
	u16 := func() (uint16, bool) {
		if pos+2 > len(b) {
			return 0, false
		}
		v := binary.LittleEndian.Uint16(b[pos:])
		pos += 2
		return v, true
	}

	flags, ok := u32()
	if !ok {
		return nil
	}
	var constType, constGroup, constHigh uint32
	hasType, hasGroup, hasHigh := false, false, false
	if flags&1 != 0 {
		if constType, ok = u32(); !ok {
			return nil
		}
		hasType = true
	}
	if flags&2 != 0 {
		if constGroup, ok = u32(); !ok {
			return nil
		}
		hasGroup = true
	}
	if flags&4 != 0 {
		if constHigh, ok = u32(); !ok {
			return nil
		}
		hasHigh = true
	}

	out := make([]dbpfEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		var e dbpfEntry
		if hasType {
			e.Type = constType
		} else if e.Type, ok = u32(); !ok {
			break
		}
		if hasGroup {
			e.Group = constGroup
		} else if e.Group, ok = u32(); !ok {
			break
		}
		if hasHigh {
			e.Instance = uint64(constHigh) << 32
		} else {
			high, ok := u32()
			if !ok {
				break
			}
			e.Instance = uint64(high) << 32
		}
		low, ok := u32()
		if !ok {
			break
		}
		e.Instance |= uint64(low)
		if e.Offset, ok = u32(); !ok {
			break
		}
		sizeRaw, ok := u32()
		if !ok {
			break
		}
		if e.MemSize, ok = u32(); !ok {
			break
		}
		if sizeRaw&0x80000000 != 0 {
			c, ok := u16()
			if !ok {
				break
			}
			e.Compression = c
			if pos+2 > len(b) {
				break
			}
			pos += 2
		}
		e.Size = sizeRaw & 0x7FFFFFFF
		out = append(out, e)
	}
	return out
}

func readDbpfResource(f *os.File, e dbpfEntry, max int) []byte {
	if e.Size == 0 || int(e.Size) > max*8 {
		return nil
	}
	raw := make([]byte, e.Size)
	if _, err := f.ReadAt(raw, int64(e.Offset)); err != nil {
		return nil
	}
	switch e.Compression {
	case 0:
		if len(raw) > max {
			return nil
		}
		return raw
	case 0x5A42:
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil
		}
		defer zr.Close()
		out, err := io.ReadAll(io.LimitReader(zr, int64(max)+1))
		if err != nil || len(out) > max {
			return nil
		}
		return out
	}
	return nil
}

func scanPackage(path string) *packageInsight {
	entries, err := readDbpfIndex(path)
	if err != nil {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	ins := &packageInsight{Resources: len(entries), Contents: map[string]int{}}
	for _, e := range entries {
		if label, ok := dbpfTypeLabels[e.Type]; ok {
			ins.Contents[label]++
		}
	}
	if len(entries) <= maxInsightKeys {
		ins.Keys = make([]resourceKey, 0, len(entries))
		for _, e := range entries {
			ins.Keys = append(ins.Keys, resourceKey{Type: e.Type, Group: e.Group, Instance: e.Instance})
		}
	}
	ins.Thumb = bestThumbnail(f, entries)
	ins.Kind = insightKind(ins.Contents, false)
	return ins
}

func probeSize(e dbpfEntry) int {
	if e.MemSize > 0 {
		return int(e.MemSize)
	}
	return int(e.Size)
}

func bestThumbnail(f *os.File, entries []dbpfEntry) []byte {
	var best []byte
	bestArea := -1
	try := func(e dbpfEntry) bool {
		if e.Size == 0 || int(e.Size) > maxThumbBytes {
			return false
		}
		data := readDbpfResource(f, e, maxThumbBytes)
		if data == nil || (!isPNG(data) && !isJPEG(data)) {
			return false
		}
		area := imageArea(data)
		if area > bestArea {
			best, bestArea = data, area
		}
		return area >= goodEnoughArea
	}

	for _, e := range entries {
		if thumbnailTypes[e.Type] && try(e) {
			return best
		}
	}

	sorted := append([]dbpfEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		return probeSize(sorted[i]) < probeSize(sorted[j])
	})
	budget := probeByteBudget
	probes := 0
	for _, e := range sorted {
		if probes >= maxProbeCount || budget <= 0 {
			break
		}
		if thumbnailTypes[e.Type] || int(e.Size) > budget {
			continue
		}
		budget -= int(e.Size)
		probes++
		if try(e) {
			break
		}
	}
	return best
}

func insightKind(c map[string]int, hasScript bool) string {
	switch {
	case c["CAS parts"] > 0:
		return "CAS"
	case c["objects"] > 0:
		return "Build Buy"
	case c["tunings"]+c["behaviors"] > 0:
		return "Gameplay"
	case hasScript:
		return "Script"
	}
	return "Package"
}

func baseCategory(name string) string {
	if isScript(name) {
		return "Script"
	}
	if hasAnySuffix(stripDisabled(name), ".package") {
		return "Package"
	}
	return "File"
}

// stripDisabled removes a switch-off suffix, leaving the real extension.
func stripDisabled(name string) string {
	if s, ok := disabledSuffixOf(name); ok {
		return name[:len(name)-len(s)]
	}
	return name
}

type cachedInsight struct {
	size    int64
	newest  time.Time
	insight *packageInsight
}

type modRef struct {
	path   string
	size   int64
	newest time.Time
}

func (m *ModsService) cachedInsight(path string, size int64, newest time.Time) (*packageInsight, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.insights[path]
	if !ok || c.size != size || !c.newest.Equal(newest) {
		return nil, false
	}
	return c.insight, true
}

func (m *ModsService) insightForPath(path string, size int64, newest time.Time) *packageInsight {
	if ins, ok := m.cachedInsight(path, size, newest); ok {
		return ins
	}

	var ins *packageInsight
	info, err := os.Stat(path)
	switch {
	case err != nil:
	case !info.IsDir():
		if isScript(path) {
			ins = &packageInsight{Kind: "Script", Contents: map[string]int{}}
		} else if hasAnySuffix(stripDisabled(path), ".package") {
			ins = scanPackage(path)
		}
	default:
		ins = m.scanFolder(path)
	}

	m.mu.Lock()
	m.insights[path] = cachedInsight{size: size, newest: newest, insight: ins}
	m.mu.Unlock()
	return ins
}

func (m *ModsService) insightForFile(path string, size int64, newest time.Time) *packageInsight {
	if ins, ok := m.cachedInsight(path, size, newest); ok {
		return ins
	}
	ins := scanPackage(path)
	m.mu.Lock()
	m.insights[path] = cachedInsight{size: size, newest: newest, insight: ins}
	m.mu.Unlock()
	return ins
}

// scanFolder merges the packages inside a mod folder. A folder of thousands of
// packages keeps the first 64; the row only needs a kind and a picture.
func (m *ModsService) scanFolder(dir string) *packageInsight {
	merged := &packageInsight{Contents: map[string]int{}, Kind: "Package"}
	hasScript := false
	packages := 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if isScript(d.Name()) {
			hasScript = true
		}
		if !hasAnySuffix(stripDisabled(d.Name()), ".package") || packages >= 64 {
			return nil
		}
		packages++
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mergeInsight(merged, m.insightForFile(p, info.Size(), info.ModTime()))
		return nil
	})
	if merged.Resources == 0 && hasScript {
		merged.Kind = "Script"
	} else {
		merged.Kind = insightKind(merged.Contents, hasScript)
	}
	return merged
}

func mergeInsight(into, add *packageInsight) {
	if add == nil {
		return
	}
	into.Resources += add.Resources
	for k, v := range add.Contents {
		into.Contents[k] += v
	}
	if len(into.Keys)+len(add.Keys) <= maxInsightKeys {
		into.Keys = append(into.Keys, add.Keys...)
	}
	if into.Thumb == nil || imageArea(add.Thumb) > imageArea(into.Thumb) {
		into.Thumb = add.Thumb
	}
}

// Thumbnail returns a data URL for one row, or "" when the package has none.
func (m *ModsService) Thumbnail(id string) string {
	m.mu.Lock()
	ref, ok := m.mods[id]
	m.mu.Unlock()

	if !ok {
		dir, err := m.dir()
		if err != nil {
			return ""
		}
		root := filepath.Clean(dir)
		target := filepath.Clean(filepath.Join(root, filepath.FromSlash(id)))
		if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return ""
		}
		info, err := os.Stat(target)
		if err != nil {
			return ""
		}
		ref = modRef{path: target, size: info.Size(), newest: info.ModTime()}
	}

	ins := m.insightForPath(ref.path, ref.size, ref.newest)
	if ins == nil || ins.Thumb == nil {
		return ""
	}
	return dataURL(ins.Thumb)
}
