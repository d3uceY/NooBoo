package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ConfigService owns settings and the game folder choice.
type ConfigService struct {
	base string
}

func NewConfigService() *ConfigService {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return &ConfigService{base: filepath.Join(dir, "NooBoo")}
}

// AppData is the app's own folder: config, backups and quarantine live here.
func (c *ConfigService) AppData() string { return c.base }

// About reports the build version and the project home for the release link.
func (c *ConfigService) About() models.AppInfo {
	return models.AppInfo{Version: Version, Repo: RepoURL}
}

// OpenRepo opens the releases page in the user's browser.
func (c *ConfigService) OpenRepo() error {
	return c.OpenRelease("")
}

// OpenRelease opens a release page. Empty means the latest one. Anything
// outside this repo is refused, since the URL comes back over the bridge.
func (c *ConfigService) OpenRelease(url string) error {
	if url == "" {
		url = RepoURL + "/releases/latest"
	}
	if !strings.HasPrefix(url, RepoURL) {
		return errors.New("refusing to open an unknown link")
	}
	return application.Get().Browser.OpenURL(url)
}

func (c *ConfigService) configPath() string { return filepath.Join(c.base, "config.json") }

// Get returns settings, falling back to defaults when the file is absent or corrupt.
func (c *ConfigService) Get() models.Config {
	var cfg models.Config
	b, err := os.ReadFile(c.configPath())
	if err != nil {
		return cfg
	}
	if json.Unmarshal(b, &cfg) != nil {
		return models.Config{}
	}
	return cfg
}

func (c *ConfigService) save(cfg models.Config) error {
	if err := os.MkdirAll(c.base, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath(), b, 0o644)
}

// DetectGameRoot returns the default Sims 4 folder, or "" when it is not there.
// Documents is often redirected into OneDrive, so that path is checked too.
func (c *ConfigService) DetectGameRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, documents := range []string{
		filepath.Join(home, "Documents"),
		filepath.Join(home, "OneDrive", "Documents"),
	} {
		p := filepath.Join(documents, "Electronic Arts", "The Sims 4")
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

// GameFolder inspects a folder and reports whether it is a Sims 4 user folder.
// Picking Documents or the Electronic Arts parent is common, so it descends.
func (c *ConfigService) GameFolder(root string) models.GameFolder {
	if root == "" {
		return models.GameFolder{Problem: "No folder selected"}
	}
	for _, candidate := range []string{
		root,
		filepath.Join(root, "The Sims 4"),
		filepath.Join(root, "Electronic Arts", "The Sims 4"),
	} {
		if g := inspect(candidate); g.Valid {
			return g
		}
	}
	g := inspect(root)
	if g.Problem == "" {
		g.Problem = "Not a Sims 4 folder"
	}
	return g
}

// defaultKeepLast caps how many snapshots pile up before the oldest is dropped.
const defaultKeepLast = 10

// KeepLast is how many snapshots to retain after each backup.
func (c *ConfigService) KeepLast() int {
	n := c.Get().KeepLast
	if n <= 0 {
		return defaultKeepLast
	}
	return n
}

// SetKeepLast stores the retention count. The next backup prunes to match.
func (c *ConfigService) SetKeepLast(n int) error {
	if n < 1 {
		n = 1
	}
	if n > 200 {
		n = 200
	}
	cfg := c.Get()
	cfg.KeepLast = n
	return c.save(cfg)
}

// SetGameRoot validates and remembers the folder. An invalid folder is not saved.
func (c *ConfigService) SetGameRoot(root string) models.GameFolder {
	g := c.GameFolder(root)
	if !g.Valid {
		return g
	}
	cfg := c.Get()
	cfg.GameRoot = g.Root
	_ = c.save(cfg)
	return g
}

// PickGameFolder opens the native folder picker.
func (c *ConfigService) PickGameFolder() (models.GameFolder, error) {
	dir, err := application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Select your Sims 4 folder").
		PromptForSingleSelection()
	if err != nil {
		return models.GameFolder{}, err
	}
	if dir == "" {
		return models.GameFolder{Problem: "No folder selected"}, nil
	}
	return c.SetGameRoot(dir), nil
}

// Current returns the saved folder, revalidated so a moved folder reads as invalid.
func (c *ConfigService) Current() models.GameFolder {
	root := c.Get().GameRoot
	if root == "" {
		return models.GameFolder{}
	}
	return c.GameFolder(root)
}

// Reveal opens a folder in File Explorer.
func (c *ConfigService) Reveal(path string) error {
	if path == "" {
		return errors.New("nothing to open")
	}
	return openInExplorer(path)
}

// markers that only exist in a real Sims 4 user folder.
func inspect(root string) models.GameFolder {
	g := models.GameFolder{Root: root}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		g.Problem = "Folder not found"
		return g
	}

	found := 0
	for _, name := range []string{"saves", "Tray", "Mods", "Options.ini", "GameVersion.txt", "UserSetting.ini"} {
		if fileExists(filepath.Join(root, name)) {
			found++
		}
	}
	g.Saves = countEntries(filepath.Join(root, "saves"))
	g.Tray = countEntries(filepath.Join(root, "Tray"))
	g.Mods = countLoadable(filepath.Join(root, "Mods"))

	// Two markers is enough: a fresh install has no Mods folder yet.
	g.Valid = found >= 2
	if !g.Valid {
		g.Problem = "Not a Sims 4 folder"
	}
	return g
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func countEntries(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

// countLoadable counts package/script files, enabled or not.
func countLoadable(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if isLoadable(d.Name()) {
			n++
		}
		return nil
	})
	return n
}

func isLoadable(name string) bool {
	return hasAnySuffix(name, ".package", ".ts4script")
}

func hasAnySuffix(name string, suffixes ...string) bool {
	lower := strings.ToLower(name)
	for _, s := range suffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}
