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

// PickFolder opens the native picker for an unpacked mod folder, for people who
// downloaded a whole folder of packages instead of a single archive.
func (m *ModsService) PickFolder() (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(false).
		CanChooseDirectories(true).
		SetTitle("Choose a mod folder").
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
				mods = append(mods, m.describe(full, displayName(name), "", models.KindMod))
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

// Install copies a download into place, from a file or a whole folder. Mods get
// their own folder; custom content is grouped under the creator.
func (m *ModsService) Install(req models.InstallRequest) (models.Mod, error) {
	dir, err := m.dir()
	if err != nil {
		return models.Mod{}, err
	}
	info, err := os.Stat(req.Source)
	if err != nil {
		return models.Mod{}, errors.New("pick a .package, .ts4script, .zip or a folder")
	}
	if !info.IsDir() && !isLoadable(req.Source) && !strings.EqualFold(filepath.Ext(req.Source), ".zip") {
		return models.Mod{}, errors.New("only .package, .ts4script, .zip and folders are supported — extract .rar or .7z first")
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

	if info.IsDir() {
		if err := copyModFiles(req.Source, dest); err != nil {
			removeEmptyDir(dest)
			return models.Mod{}, err
		}
	} else if strings.EqualFold(filepath.Ext(req.Source), ".zip") {
		if err := extractZip(req.Source, dest); err != nil {
			removeEmptyDir(dest)
			return models.Mod{}, err
		}
	} else if err := copyFile(req.Source, filepath.Join(dest, filepath.Base(req.Source))); err != nil {
		removeEmptyDir(dest)
		return models.Mod{}, err
	}

	if !hasLoadableDirect(dest) {
		removeEmptyDir(dest)
		return models.Mod{}, fmt.Errorf("no .package or .ts4script inside %s", filepath.Base(req.Source))
	}
	return m.describe(dest, name, creator, req.Kind), nil
}

// copyModFiles copies everything the game loads out of a folder the user picked,
// flattening subfolders so the result matches what a zip install produces.
// Disabled files keep their suffix, so a folder of half-switched-off mods
// arrives in the same state. A repeated file name keeps the shallowest copy.
func copyModFiles(src, dst string) error {
	taken := map[string]bool{}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != src && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !isLoadable(name) && !isDisabled(name) {
			return nil
		}
		if taken[name] {
			return nil
		}
		taken[name] = true
		return copyFile(p, filepath.Join(dst, name))
	})
}

// removeEmptyDir clears the folder Install made when the copy failed or the
// archive held nothing the game loads, so a failed install leaves no litter.
func removeEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}
	_ = os.Remove(dir)
}

// Tidy moves loose files in Mods into a folder of their own, the same shape
// Install already produces. Files that share a stem land together, so a package
// and its ts4script get one switch instead of two. Names keep their `!` or
// `[bracket]` prefix and their Creator_Item underscores, because that is how the
// player sorted them by hand.
func (m *ModsService) Tidy() (models.TidyResult, error) {
	res := models.TidyResult{Moved: []models.TidyMove{}, Skipped: []string{}}
	dir, err := m.dir()
	if err != nil {
		return res, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, err
	}

	folders := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "Resource.cfg") {
			continue
		}
		if !isLoadable(name) && !isDisabled(name) {
			continue
		}

		stem := strings.TrimSpace(displayName(name))
		if stem == "" {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		folder := folderSafe(stem)
		target := filepath.Join(dir, folder)
		if exists, isDir := statKind(target); exists {
			// A folder holding only subfolders is a creator folder. Moving a file
			// into it would turn the whole thing into one mod, so leave it be.
			if !isDir || !hasLoadableDirect(target) {
				res.Skipped = append(res.Skipped, name)
				continue
			}
		}
		dest := filepath.Join(target, name)
		if fileExists(dest) {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return res, err
		}
		if err := os.Rename(filepath.Join(dir, name), dest); err != nil {
			return res, err
		}
		res.Moved = append(res.Moved, models.TidyMove{File: name, Into: folder})
		folders[folder] = true
	}
	res.Folders = len(folders)
	return res, nil
}

// statKind separates "gone" from "here, and a folder or not".
func statKind(path string) (bool, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, info.IsDir()
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
	return movePath(target, filepath.Join(aside, filepath.Base(target)))
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

// displayName is a loose file's name with the switch-off suffix and the real
// extension gone, so "tool.package.off" reads as "tool".
func displayName(name string) string {
	base := name
	if isDisabled(base) {
		base = base[:len(base)-len(disabledSuffix)]
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
