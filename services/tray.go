package services

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nooboo/models"
)

// A Sims 4 tray item is a set of files sharing one instance:
// 0x<group>!0x<instance>.<ext>.
var trayNameRe = regexp.MustCompile(`(?i)^0x([0-9a-f]{1,16})!0x([0-9a-f]{1,16})\.([a-z0-9]+)$`)

var trayKinds = map[string]string{
	"householdbinary": "household",
	"blueprint":       "lot",
	"room":            "room",
}

var trayThumbs = map[string]bool{"hhi": true, "bpi": true, "rmi": true}

const (
	trayHeaderBytes = 8
	trayImageHeader = 24
	maxTrayImage    = 32 << 20
	maxTrayFile     = 64 << 20
)

var trayImageKey = []byte{0x41, 0x25, 0xE6, 0xCD, 0x47, 0xBA, 0xB2, 0x1A}

// TrayService reads and fills the Tray folder.
type TrayService struct {
	cfg *ConfigService
}

func NewTrayService(cfg *ConfigService) *TrayService { return &TrayService{cfg: cfg} }

func (t *TrayService) dir() (string, error) {
	root := t.cfg.Get().GameRoot
	if root == "" {
		return "", errors.New("set your Sims 4 folder first")
	}
	return filepath.Join(root, "Tray"), nil
}

// Scan lists every complete set in Tray, newest metadata first.
func (t *TrayService) Scan() ([]models.TrayItem, error) {
	empty := []models.TrayItem{}
	dir, err := t.dir()
	if err != nil {
		return empty, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return empty, nil
	}

	groups := map[string][]string{}
	anchors := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !isTrayName(e.Name()) {
			continue
		}
		m := trayNameRe.FindStringSubmatch(e.Name())
		if strings.EqualFold(m[3], "sgi") {
			continue // per-sim portraits belong to a household, not a row
		}
		key := strings.ToLower(m[2])
		groups[key] = append(groups[key], e.Name())
		if strings.EqualFold(m[3], "trayitem") {
			anchors[key] = e.Name()
		}
	}

	items := make([]models.TrayItem, 0, len(groups))
	for key, files := range groups {
		id := anchors[key]
		if id == "" {
			id = files[0]
		}
		item := models.TrayItem{
			ID:    id,
			Name:  displayName(id),
			Kind:  "household",
			Files: []string{},
			Path:  filepath.Join(dir, id),
		}
		if name, creator := readTrayItem(filepath.Join(dir, id)); name != "" {
			item.Name = name
			item.Creator = creator
		}
		var newest time.Time
		var best []byte
		bestArea := -1
		for _, f := range files {
			full := filepath.Join(dir, f)
			if info, err := os.Stat(full); err == nil {
				item.Size += info.Size()
				if info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f), "."))
			if kind, ok := trayKinds[ext]; ok {
				item.Kind = kind
			}
			if trayThumbs[ext] {
				if img := readTrayImage(full); img != nil {
					if area := imageArea(img); area > bestArea {
						best, bestArea = img, area
					}
				}
			}
			item.Files = append(item.Files, f)
		}
		if !newest.IsZero() {
			item.Modified = newest.Format(time.RFC3339)
		}
		if best != nil {
			item.Thumb = dataURL(best)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

// hasTraySet reports whether source carries a tray item, so Install can route it.
func hasTraySet(source string) bool {
	info, err := os.Stat(source)
	if err != nil {
		return false
	}
	if !info.IsDir() && strings.EqualFold(filepath.Ext(source), ".zip") {
		r, err := zip.OpenReader(source)
		if err != nil {
			return false
		}
		defer r.Close()
		for _, f := range r.File {
			if isTrayName(filepath.Base(f.Name)) {
				return true
			}
		}
		return false
	}
	if !info.IsDir() {
		return isTrayName(filepath.Base(source))
	}
	found := false
	_ = filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if !d.IsDir() && isTrayName(d.Name()) {
			found = true
		}
		return nil
	})
	return found
}

type traySource struct {
	name string
	path string
	data []byte
}

// installTraySet copies a whole set into Tray, never half of one.
func installTraySet(cfg *ConfigService, source string) (models.TrayItem, error) {
	sources, err := traySources(source)
	if err != nil {
		return models.TrayItem{}, err
	}
	if len(sources) == 0 {
		return models.TrayItem{}, errors.New("no tray files in this download")
	}

	dir, err := NewTrayService(cfg).dir()
	if err != nil {
		return models.TrayItem{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return models.TrayItem{}, err
	}

	anchor := ""
	kind := "household"
	for _, s := range sources {
		if err := copyTraySource(s, dir); err != nil {
			return models.TrayItem{}, err
		}
		m := trayNameRe.FindStringSubmatch(s.name)
		if m == nil {
			continue
		}
		ext := strings.ToLower(m[3])
		if ext == "trayitem" {
			anchor = s.name
		}
		if k, ok := trayKinds[ext]; ok {
			kind = k
		}
	}
	if anchor == "" {
		anchor = sources[0].name
	}

	items, _ := NewTrayService(cfg).Scan()
	for _, item := range items {
		if item.ID == anchor {
			return item, nil
		}
	}
	return models.TrayItem{ID: anchor, Name: displayName(anchor), Kind: kind}, nil
}

func traySources(source string) ([]traySource, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, errors.New("that file is gone")
	}

	if !info.IsDir() && strings.EqualFold(filepath.Ext(source), ".zip") {
		r, err := zip.OpenReader(source)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		var out []traySource
		seen := map[string]bool{}
		for _, f := range r.File {
			name := filepath.Base(f.Name)
			if f.FileInfo().IsDir() || !isTrayName(name) || seen[name] {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(rc, maxTrayFile+1))
			rc.Close()
			if err != nil {
				return nil, err
			}
			if len(data) > maxTrayFile {
				return nil, errors.New("a tray file in this download is too big")
			}
			seen[name] = true
			out = append(out, traySource{name: name, data: data})
		}
		return out, nil
	}

	if !info.IsDir() {
		paths := traySiblings(source)
		out := make([]traySource, 0, len(paths))
		for _, p := range paths {
			out = append(out, traySource{name: filepath.Base(p), path: p})
		}
		return out, nil
	}

	var out []traySource
	seen := map[string]bool{}
	err = filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isTrayName(d.Name()) || seen[d.Name()] {
			return nil
		}
		seen[d.Name()] = true
		out = append(out, traySource{name: d.Name(), path: p})
		return nil
	})
	return out, err
}

// traySiblings is the rest of the set beside a picked file, portraits included.
func traySiblings(path string) []string {
	m := trayNameRe.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return []string{path}
	}
	instance := strings.ToLower(m[2])
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{path}
	}
	out := []string{path}
	seen := map[string]bool{filepath.Base(path): true}
	for _, e := range entries {
		if e.IsDir() || seen[e.Name()] {
			continue
		}
		s := trayNameRe.FindStringSubmatch(e.Name())
		if s == nil {
			continue
		}
		sameItem := strings.ToLower(s[2]) == instance
		portrait := strings.EqualFold(s[3], "sgi")
		if !sameItem && !portrait {
			continue
		}
		seen[e.Name()] = true
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func copyTraySource(src traySource, dir string) error {
	target := filepath.Join(dir, src.name)
	tmp := target + ".part"
	var r io.ReadCloser
	if src.data != nil {
		r = io.NopCloser(bytes.NewReader(src.data))
	} else {
		f, err := os.Open(src.path)
		if err != nil {
			return err
		}
		r = f
	}
	defer r.Close()
	if err := copyReader(r, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isTrayName(name string) bool {
	m := trayNameRe.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	ext := strings.ToLower(m[3])
	if ext == "trayitem" || ext == "sgi" || trayThumbs[ext] {
		return true
	}
	_, ok := trayKinds[ext]
	return ok
}

// readTrayItem pulls the name and creator out of the metadata protobuf.
func readTrayItem(path string) (name, creator string) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) <= trayHeaderBytes {
		return "", ""
	}
	fields := protoFields(raw[trayHeaderBytes:])
	return cleanText(fields[4]), cleanText(fields[7])
}

// protoFields keeps the last length-delimited value for each field number.
// Enough for the tray metadata; every other wire type is skipped.
func protoFields(b []byte) map[int][]byte {
	out := map[int][]byte{}
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return out
		}
		b = b[n:]
		field := int(key >> 3)
		switch int(key & 7) {
		case 0:
			_, n = binary.Uvarint(b)
			if n <= 0 {
				return out
			}
			b = b[n:]
		case 1:
			if len(b) < 8 {
				return out
			}
			b = b[8:]
		case 2:
			size, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < size {
				return out
			}
			b = b[n:]
			out[field] = b[:size]
			b = b[size:]
		case 5:
			if len(b) < 4 {
				return out
			}
			b = b[4:]
		default:
			return out
		}
	}
	return out
}

func cleanText(b []byte) string {
	return strings.TrimSpace(string(b))
}

// readTrayImage undoes the tray's XOR obfuscation.
func readTrayImage(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) <= trayImageHeader {
		return nil
	}
	declared := binary.LittleEndian.Uint64(raw[:8])
	if declared == 0 || declared > maxTrayImage || uint64(len(raw)) < trayImageHeader+declared {
		return nil
	}
	out := make([]byte, declared)
	for i := range out {
		out[i] = raw[trayImageHeader+i] ^ trayImageKey[i&7]
	}
	if !isJPEG(out) && !isPNG(out) {
		return nil
	}
	return out
}

func imageArea(b []byte) int {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0
	}
	return cfg.Width * cfg.Height
}

func isJPEG(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
}

func isPNG(b []byte) bool {
	return len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E &&
		b[3] == 0x47 && b[4] == 0x0D && b[5] == 0x0A && b[6] == 0x1A && b[7] == 0x0A
}

func dataURL(b []byte) string {
	mime := "image/png"
	if isJPEG(b) {
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}
