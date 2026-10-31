package services

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// disabledSuffixes are the switch-off markers the scan understands. .off
// matches CC Magic; .disabled comes from other managers. The app only ever
// writes writingSuffix, so a folder it manages keeps one shape.
var disabledSuffixes = []string{".off", ".disabled"}

const writingSuffix = ".off"

// ModsService reads and rewrites the Mods folder.
type ModsService struct {
	cfg *ConfigService

	mu       sync.Mutex
	insights map[string]cachedInsight
	mods     map[string]modRef
}

func NewModsService(cfg *ConfigService) *ModsService {
	return &ModsService{cfg: cfg, insights: map[string]cachedInsight{}, mods: map[string]modRef{}}
}

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
		AddFilter("Sims 4 mods", "*.package;*.ts4script;*.zip;*.trayitem").
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
	tags := m.cfg.Get().Tags
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "Resource.cfg") {
			continue
		}
		full := filepath.Join(dir, name)

		if !e.IsDir() {
			if isLoadable(name) || isDisabled(name) {
				mods = append(mods, m.describe(full, displayName(name), "", models.KindMod, tags))
			}
			continue
		}

		if hasLoadableDirect(full) || hasNestedScript(full) {
			mods = append(mods, m.describe(full, name, "", models.KindMod, tags))
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
				mods = append(mods, m.describe(child, s.Name(), name, models.KindCC, tags))
			}
		}
	}
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Creator != mods[j].Creator {
			return mods[i].Creator < mods[j].Creator
		}
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})
	m.attachHints(mods)
	return mods, nil
}

func (m *ModsService) describe(full, name, creator string, kind models.Kind, tags map[string][]string) models.Mod {
	id, err := filepath.Rel(mustDir(m), full)
	if err != nil {
		id = full
	}
	enabled, size, files, newest := readMod(full)
	modified := ""
	if !newest.IsZero() {
		modified = newest.Format(time.RFC3339)
	}
	category := baseCategory(filepath.Base(full))
	resources := 0
	hasThumb := false
	if insight := m.insightForPath(full, size, newest); insight != nil {
		category = insight.Kind
		resources = insight.Resources
		hasThumb = insight.Thumb != nil
	}
	idKey := filepath.ToSlash(id)
	mod := models.Mod{
		ID:            idKey,
		Name:          name,
		Creator:       creator,
		Kind:          kind,
		Enabled:       enabled,
		Path:          full,
		Size:          size,
		Modified:      modified,
		Files:         files,
		Category:      category,
		ResourceCount: resources,
		HasThumb:      hasThumb,
		Tags:          tags[modTagKey(idKey)],
	}
	m.mu.Lock()
	m.mods[mod.ID] = modRef{path: full, size: size, newest: newest}
	m.mu.Unlock()
	return mod
}

func mustDir(m *ModsService) string {
	dir, _ := m.dir()
	return dir
}

// readMod collects enabled, size, file names and newest mtime in one walk.
func readMod(p string) (enabled bool, size int64, files []string, newest time.Time) {
	_ = filepath.WalkDir(p, func(q string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size += info.Size()
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if isLoadable(d.Name()) {
			enabled = true
		}
		if len(files) < 40 {
			if q == p {
				files = append(files, filepath.Base(q))
			} else if rel, err := filepath.Rel(p, q); err == nil {
				files = append(files, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return
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
			// Trim the suffix that is actually there, in any case.
			if s, ok := disabledSuffixOf(filepath.Base(p)); ok {
				if err := os.Rename(p, p[:len(p)-len(s)]); err != nil {
					return err
				}
			}
			continue
		}
		if err := os.Rename(p, p+writingSuffix); err != nil {
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
	if !info.IsDir() && !isLoadable(req.Source) && !isDisabled(req.Source) &&
		!strings.EqualFold(filepath.Ext(req.Source), ".zip") {
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

	if !hasNestedLoadable(dest) {
		removeEmptyDir(dest)
		return models.Mod{}, fmt.Errorf("no .package or .ts4script inside %s", filepath.Base(req.Source))
	}
	return m.describe(dest, name, creator, req.Kind, m.cfg.Get().Tags), nil
}

// InstallDownload routes a download by what it contains: a tray set goes to
// Tray, everything else through Install.
func (m *ModsService) InstallDownload(req models.InstallRequest) (models.InstallResult, error) {
	if hasTraySet(req.Source) {
		name, err := installTraySet(m.cfg, req.Source)
		if err != nil {
			return models.InstallResult{}, err
		}
		return models.InstallResult{Target: "tray", Name: name}, nil
	}
	mod, err := m.Install(req)
	if err != nil {
		return models.InstallResult{}, err
	}
	return models.InstallResult{Target: "mods", Name: mod.Name}, nil
}

// copyModFiles copies the loadable files a folder holds, keeping the shape
// inside it. One shared top folder is dropped so a download that wraps
// everything in "Cool Hair" does not become Mods/Cool Hair/Cool Hair.
func copyModFiles(src, dst string) error {
	var items []modItem
	_ = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != src && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isLoadable(d.Name()) && !isDisabled(d.Name()) {
			return nil
		}
		if rel, err := filepath.Rel(src, p); err == nil {
			items = append(items, modItem{rel: rel, path: p})
		}
		return nil
	})
	return copyItems(items, dst)
}

// modItem is one file to copy, from disk or from a zip.
type modItem struct {
	rel  string
	path string
	zip  *zip.File
}

// copyItems keeps every loadable file at its relative path, minus one shared
// wrapper folder. A path taken twice keeps the first copy.
func copyItems(items []modItem, dst string) error {
	root := commonRoot(items)
	taken := map[string]bool{}
	for _, it := range items {
		rel := strings.TrimPrefix(filepath.ToSlash(it.rel), root)
		safe, ok := safeRel(rel)
		if !ok || taken[safe] {
			continue
		}
		taken[safe] = true
		target := filepath.Join(dst, safe)
		if it.zip != nil {
			rc, err := it.zip.Open()
			if err != nil {
				return err
			}
			err = copyReader(rc, target)
			rc.Close()
			if err != nil {
				return err
			}
			continue
		}
		if err := copyFile(it.path, target); err != nil {
			return err
		}
	}
	return nil
}

// commonRoot is the one top folder every file shares, or "".
func commonRoot(items []modItem) string {
	if len(items) == 0 {
		return ""
	}
	parts := strings.SplitN(filepath.ToSlash(items[0].rel), "/", 2)
	if len(parts) < 2 {
		return ""
	}
	root := parts[0]
	for _, it := range items {
		next := strings.SplitN(filepath.ToSlash(it.rel), "/", 2)
		if len(next) < 2 || next[0] != root {
			return ""
		}
	}
	return root + "/"
}

// safeRel refuses a path that would leave the destination.
func safeRel(rel string) (string, bool) {
	rel = filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/"))
	clean := filepath.Clean(rel)
	if clean == "." || clean == "" || filepath.IsAbs(clean) {
		return "", false
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return clean, true
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

// extractZip unpacks the loadable files, keeping the archive's folders and
// dropping one shared wrapper. Only .package/.ts4script arrive, on or off.
func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	var items []modItem
	for _, f := range r.File {
		if f.FileInfo().IsDir() || (!isLoadable(f.Name) && !isDisabled(f.Name)) {
			continue
		}
		if rel, ok := safeRel(f.Name); ok {
			items = append(items, modItem{rel: rel, zip: f})
		}
	}
	return copyItems(items, dst)
}

func copyReader(r io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
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

// disabledSuffixOf reports which switch-off marker name carries.
func disabledSuffixOf(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, s := range disabledSuffixes {
		if strings.HasSuffix(lower, s) && isLoadable(name[:len(name)-len(s)]) {
			return s, true
		}
	}
	return "", false
}

// isDisabled matches a file the toggle renamed, whatever wrote the suffix.
func isDisabled(name string) bool {
	_, ok := disabledSuffixOf(name)
	return ok
}

// isScript reports a .ts4script, switched off or not.
func isScript(name string) bool {
	if s, ok := disabledSuffixOf(name); ok {
		name = name[:len(name)-len(s)]
	}
	return hasAnySuffix(name, ".ts4script")
}

// hasNestedScript catches script mods that ship their code one folder down.
// Deeper folders are creator/item layouts and stay as separate rows.
func hasNestedScript(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, f := range sub {
			if !f.IsDir() && isScript(f.Name()) {
				return true
			}
		}
	}
	return false
}

// displayName is a loose file's name with the switch-off suffix and the real
// extension gone, so "tool.package.off" reads as "tool".
func displayName(name string) string {
	base := name
	if s, ok := disabledSuffixOf(base); ok {
		base = base[:len(base)-len(s)]
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

const (
	maxTagLength  = 24
	maxTagsPerMod = 12
)

// modTagKey is the path a mod carries while it is switched on.
func modTagKey(id string) string {
	native := filepath.FromSlash(id)
	base := filepath.Base(native)
	if s, ok := disabledSuffixOf(base); ok {
		return filepath.ToSlash(filepath.Join(filepath.Dir(native), base[:len(base)-len(s)]))
	}
	return filepath.ToSlash(native)
}

func sanitizeTag(raw string) string {
	raw = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, raw)
	raw = strings.Join(strings.Fields(raw), " ")
	runes := []rune(raw)
	if len(runes) > maxTagLength {
		raw = string(runes[:maxTagLength])
	}
	return raw
}

// AddTag stores one label for a mod.
func (m *ModsService) AddTag(id, tag string) error {
	key := modTagKey(id)
	tag = sanitizeTag(tag)
	if key == "" || tag == "" {
		return nil
	}
	cfg := m.cfg.Get()
	if cfg.Tags == nil {
		cfg.Tags = map[string][]string{}
	}
	list := cfg.Tags[key]
	for _, t := range list {
		if strings.EqualFold(t, tag) {
			return nil
		}
	}
	if len(list) >= maxTagsPerMod {
		return errors.New("a mod holds 12 tags")
	}
	cfg.Tags[key] = append(list, tag)
	return m.cfg.save(cfg)
}

// RemoveTag drops one label from a mod.
func (m *ModsService) RemoveTag(id, tag string) error {
	key := modTagKey(id)
	cfg := m.cfg.Get()
	list := cfg.Tags[key]
	out := make([]string, 0, len(list))
	for _, t := range list {
		if !strings.EqualFold(t, tag) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		delete(cfg.Tags, key)
	} else {
		cfg.Tags[key] = out
	}
	return m.cfg.save(cfg)
}
