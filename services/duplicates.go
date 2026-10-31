package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nooboo/models"
)

type duplicateCandidate struct {
	rel     string
	size    int64
	modTime time.Time
	enabled bool
}

// FindDuplicates hashes only files that share a size with another file.
// It runs on demand because a whole library is a lot of reading.
func (m *ModsService) FindDuplicates() ([]models.DuplicateSet, error) {
	dir, err := m.dir()
	if err != nil {
		return nil, err
	}

	bySize := map[int64][]duplicateCandidate{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "Resource.cfg") {
			return nil
		}
		if !isLoadable(name) && !isDisabled(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() <= 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil
		}
		bySize[info.Size()] = append(bySize[info.Size()], duplicateCandidate{
			rel:     filepath.ToSlash(rel),
			size:    info.Size(),
			modTime: info.ModTime(),
			enabled: isLoadable(name),
		})
		return nil
	})

	var candidates []duplicateCandidate
	for _, group := range bySize {
		if len(group) > 1 {
			candidates = append(candidates, group...)
		}
	}
	if len(candidates) == 0 {
		return []models.DuplicateSet{}, nil
	}

	byDigest := map[string][]duplicateCandidate{}
	for i, c := range candidates {
		digest, err := hashFile(filepath.Join(dir, filepath.FromSlash(c.rel)))
		if err != nil {
			continue
		}
		byDigest[digest] = append(byDigest[digest], c)
		emit("Checking copies", i+1, len(candidates), filepath.Base(c.rel))
	}

	sets := make([]models.DuplicateSet, 0)
	for digest, group := range byDigest {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return keeperFirst(group[i], group[j]) })
		set := models.DuplicateSet{
			Digest: digest,
			Size:   group[0].size,
			Wasted: group[0].size * int64(len(group)-1),
			Files:  make([]models.DuplicateFile, 0, len(group)),
		}
		for _, c := range group {
			set.Files = append(set.Files, models.DuplicateFile{File: c.rel, Size: c.size})
		}
		sets = append(sets, set)
	}
	sort.Slice(sets, func(i, j int) bool {
		if sets[i].Wasted != sets[j].Wasted {
			return sets[i].Wasted > sets[j].Wasted
		}
		return sets[i].Digest < sets[j].Digest
	})
	return sets, nil
}

// keeperFirst picks the least surprising copy to keep: on, near the top, old.
func keeperFirst(a, b duplicateCandidate) bool {
	if a.enabled != b.enabled {
		return a.enabled
	}
	da, db := strings.Count(a.rel, "/"), strings.Count(b.rel, "/")
	if da != db {
		return da < db
	}
	if !a.modTime.Equal(b.modTime) {
		return a.modTime.Before(b.modTime)
	}
	return a.rel < b.rel
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SetAsideFile moves one copy into quarantine and tidies the empty folder.
func (m *ModsService) SetAsideFile(rel string) error {
	dir, err := m.dir()
	if err != nil {
		return err
	}
	root := filepath.Clean(dir)
	target := filepath.Clean(modsDir(root, rel))
	if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file %q", rel)
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return errors.New("file not found")
	}

	aside := uniqueDir(filepath.Join(m.cfg.AppData(), "quarantine", time.Now().Format("2006-01-02_15-04-05")))
	dst := filepath.Join(aside, "files", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := movePath(target, dst); err != nil {
		return err
	}
	pruneEmptyDirs(filepath.Dir(target), root)
	return nil
}

func pruneEmptyDirs(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop+string(os.PathSeparator)) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
