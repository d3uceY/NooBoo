package services

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// disabledSuffix is how a mod is switched off: the engine only loads the two
// real extensions, so the rename is enough and nothing moves on disk.
const disabledSuffix = ".off"

// ModsService reads and rewrites the Mods folder.
type ModsService struct {
	cfg *ConfigService
}

func NewModsService(cfg *ConfigService) *ModsService { return &ModsService{cfg: cfg} }

func (m *ModsService) dir() (string, error) {
	root := m.cfg.Get().GameRoot
	if root == "" {
		return "", errors.New("set your Sims 4 folder first")
	}
	return filepath.Join(root, "Mods"), nil
}

// PickFile opens the native picker for a downloaded mod.
func (m *ModsService) PickFile() (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AddFilter("Sims 4 mods", "*.package;*.ts4script;*.zip").
		SetTitle("Choose a mod file").
		PromptForSingleSelection()
}

// Scan lists everything under Mods, one entry per installable folder.
// No folder set yet reads as an empty library, not an error.
func (m *ModsService) Scan() ([]models.Mod, error) {
	if m.cfg.Get().GameRoot == "" {
		return []models.Mod{}, nil
	}
	dir, err := m.dir()
	if err != nil {
		return []models.Mod{}, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []models.Mod{}, nil
	}
	if err != nil {
		return nil, err
	}

	mods := []models.Mod{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "Resource.cfg") {
			continue
		}
		full := filepath.Join(dir, name)

		if !e.IsDir() {
			if isLoadable(name) || isDisabled(name) {
				mods = append(mods, m.describe(full, strings.TrimSuffix(name, filepath.Ext(name)), "", models.KindMod))
			}
			continue
		}

		if hasLoadableDirect(full) {
			mods = append(mods, m.describe(full, name, "", models.KindMod))
			continue
		}
		// No loadable files at the top, so this is a creator folder.
		subs, err := os.ReadDir(full)
		if err != nil {
			continue
		}
		for _, s := range subs {
			if !s.IsDir() || strings.HasPrefix(s.Name(), ".") {
				continue
			}
			child := filepath.Join(full, s.Name())
			if hasLoadableDirect(child) || hasNestedLoadable(child) {
				mods = append(mods, m.describe(child, s.Name(), name, models.KindCC))
			}
		}
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Creator != mods[j].Creator {
			return mods[i].Creator < mods[j].Creator
		}
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})
	return mods, nil
}

func (m *ModsService) describe(full, name, creator string, kind models.Kind) models.Mod {
	id, err := filepath.Rel(mustDir(m), full)
	if err != nil {
		id = full
	}
	_, size, files, newest := treeInfo(full, 40)
	modified := ""
	if !newest.IsZero() {
		modified = newest.Format(time.RFC3339)
	}
	return models.Mod{
		ID:       filepath.ToSlash(id),
		Name:     name,
		Creator:  creator,
		Kind:     kind,
		Enabled:  isEnabled(full),
		Path:     full,
		Size:     size,
		Modified: modified,
		Files:    files,
	}
}

func mustDir(m *ModsService) string {
	dir, _ := m.dir()
	return dir
}

// isEnabled is true when at least one loadable file is not renamed.
func isEnabled(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return isLoadable(info.Name())
	}
	enabled := false
	_ = filepath.WalkDir(p, func(q string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || enabled {
			return nil
		}
		if isLoadable(d.Name()) {
			enabled = true
		}
		return nil
	})
	return enabled
}

// SetEnabled toggles a set of mods by renaming their loadable files.
func (m *ModsService) SetEnabled(ids []string, enabled bool) error {
	dir, err := m.dir()
	if err != nil {
		return err
	}
	root := filepath.Clean(dir)
	for _, id := range ids {
		target := filepath.Clean(filepath.Join(root, filepath.FromSlash(id)))
		if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("invalid mod id %q", id)
		}
		if _, err := os.Stat(target); err != nil {
			continue // vanished since the scan
		}
		if err := renameAll(target, enabled); err != nil {
			return err
		}
	}
	return nil
}

func renameAll(root string, enabled bool) error {
	var pending []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if enabled && isDisabled(d.Name()) {
			pending = append(pending, p)
		}
		if !enabled && isLoadable(d.Name()) {
			pending = append(pending, p)
		}
		return nil
	})
	for _, p := range pending {
		if enabled {
			// Trim by length, not TrimSuffix: the suffix may be .OFF.
			if err := os.Rename(p, p[:len(p)-len(disabledSuffix)]); err != nil {
				return err
			}
			continue
		}
		if err := os.Rename(p, p+disabledSuffix); err != nil {
			return err
		}
	}
	return nil
}

// Install copies a downloaded file into place. Mods get their own folder;
// custom content is grouped under the creator.
func (m *ModsService) Install(req models.InstallRequest) (models.Mod, error) {
	dir, err := m.dir()
	if err != nil {
		return models.Mod{}, err
	}
	info, err := os.Stat(req.Source)
	if err != nil || info.IsDir() {
		return models.Mod{}, errors.New("pick a .package, .ts4script or .zip file")
	}
	if !isLoadable(req.Source) && !strings.EqualFold(filepath.Ext(req.Source), ".zip") {
		return models.Mod{}, errors.New("only .package, .ts4script and .zip are supported — extract .rar or .7z first")
	}

	name := folderSafe(req.Name)
	creator := folderSafe(req.Creator)
	dest := filepath.Join(dir, name)
	if req.Kind == models.KindCC && strings.TrimSpace(req.Creator) != "" {
		dest = filepath.Join(dir, creator, name)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return models.Mod{}, err
	}

	if strings.EqualFold(filepath.Ext(req.Source), ".zip") {
		if err := extractZip(req.Source, dest); err != nil {
			return models.Mod{}, err
		}
	} else if err := copyFile(req.Source, filepath.Join(dest, filepath.Base(req.Source))); err != nil {
		return models.Mod{}, err
	}

	if !hasLoadableDirect(dest) {
		return models.Mod{}, fmt.Errorf("no .package or .ts4script inside %s", filepath.Base(req.Source))
	}
	return m.describe(dest, name, creator, req.Kind), nil
}

// Delete sets a mod aside in quarantine, so removing the wrong thing is
// recoverable and lands in the folder the player can already empty.
func (m *ModsService) Delete(id string) error {
	dir, err := m.dir()
	if err != nil {
		return err
	}
	root := filepath.Clean(dir)
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(id)))
	if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return fmt.Errorf("invalid mod id %q", id)
	}
	if !fileExists(target) {
		return errors.New("mod not found")
	}

	aside := uniqueDir(filepath.Join(m.cfg.AppData(), "quarantine", time.Now().Format("2006-01-02_15-04-05")))
	if err := os.MkdirAll(aside, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(aside, filepath.Base(target))
	if err := os.Rename(target, dest); err != nil {
		// Rename fails across volumes; copy then clear instead.
		if _, _, cerr := copyTree(target, dest, nil); cerr != nil {
			return cerr
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}

// extractZip pulls only the files the game loads, flattening directories so a
// zip with nested folders still ends up one level deep.
func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() || !isLoadable(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := copyReader(rc, filepath.Join(dst, path.Base(f.Name))); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func copyReader(r io.Reader, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// hasLoadableDirect reports loadable files immediately inside dir.
func hasLoadableDirect(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && (isLoadable(e.Name()) || isDisabled(e.Name())) {
			return true
		}
	}
	return false
}

// hasNestedLoadable catches mods that ship their own subfolders.
func hasNestedLoadable(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || found || p == dir {
			return nil
		}
		if !d.IsDir() && (isLoadable(d.Name()) || isDisabled(d.Name())) {
			found = true
		}
		return nil
	})
	return found
}

// isDisabled matches a file the toggle renamed.
func isDisabled(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, disabledSuffix) {
		return false
	}
	return isLoadable(strings.TrimSuffix(lower, disabledSuffix))
}
