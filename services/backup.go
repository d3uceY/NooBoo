package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const manifestName = "backup.json"

// backupParts are the folders a snapshot covers.
var backupParts = []string{"saves", "Tray"}

// BackupService snapshots and restores the player's saves and library.
type BackupService struct {
	cfg *ConfigService
	// sweepAfter spares snapshots that are still being written. Zero sweeps
	// everything, which is what the tests want.
	sweepAfter time.Duration
	// setAside is the one quarantine window, reused instead of piling up.
	setAside *application.WebviewWindow
}

func NewBackupService(cfg *ConfigService) *BackupService {
	return &BackupService{cfg: cfg, sweepAfter: 10 * time.Minute}
}

func (b *BackupService) backupsDir() string    { return filepath.Join(b.cfg.AppData(), "backups") }
func (b *BackupService) quarantineDir() string { return filepath.Join(b.cfg.AppData(), "quarantine") }

// BackupNow snapshots saves and Tray. Safe to call before any install.
func (b *BackupService) BackupNow() (models.Backup, error) {
	root := b.cfg.Get().GameRoot
	if root == "" {
		return models.Backup{}, errors.New("set your Sims 4 folder first")
	}

	// Sweep anything a previous crash left half-written.
	_, _ = b.Cleanup()

	id := time.Now().Format("2006-01-02_15-04-05")
	dest := uniqueDir(filepath.Join(b.backupsDir(), id))
	id = filepath.Base(dest)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return models.Backup{}, err
	}

	files, size, err := b.copyParts(root, dest, "Backing up")
	if err != nil {
		// A half-copied snapshot looks restorable but is not.
		_ = os.RemoveAll(dest)
		return models.Backup{}, err
	}

	info := models.Backup{ID: id, Created: time.Now().Format(time.RFC3339), Files: files, Size: size}
	if err := writeManifest(dest, info); err != nil {
		_ = os.RemoveAll(dest)
		return models.Backup{}, err
	}
	emit("done", files, files, "Backed up")
	_, _ = b.prune(b.cfg.KeepLast())
	return info, nil
}

// prune drops the oldest snapshots beyond keep.
func (b *BackupService) prune(keep int) (int, error) {
	list, err := b.List()
	if err != nil || len(list) <= keep {
		return 0, err
	}
	removed := 0
	for _, old := range list[keep:] {
		if err := os.RemoveAll(filepath.Join(b.backupsDir(), old.ID)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// List returns snapshots newest first.
func (b *BackupService) List() ([]models.Backup, error) {
	entries, err := os.ReadDir(b.backupsDir())
	if err != nil {
		return []models.Backup{}, nil
	}
	out := []models.Backup{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if info, err := readManifest(filepath.Join(b.backupsDir(), e.Name())); err == nil {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := createdAt(out[i]), createdAt(out[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// createdAt parses the manifest stamp. Unparseable entries sort oldest.
func createdAt(b models.Backup) time.Time {
	if t, err := time.Parse(time.RFC3339, b.Created); err == nil {
		return t
	}
	return time.Time{}
}

// Cleanup deletes snapshots a crash left without a manifest. They never show
// up in List, so without this they would sit on disk forever.
func (b *BackupService) Cleanup() (int, error) {
	entries, err := os.ReadDir(b.backupsDir())
	if err != nil {
		return 0, nil
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(b.backupsDir(), e.Name())
		if _, err := readManifest(dir); err == nil {
			continue
		}
		// A folder written seconds ago is a copy in flight, not debris.
		if b.sweepAfter > 0 {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) < b.sweepAfter {
				continue
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Storage reports what the app's folders cost, so the player can see growth.
func (b *BackupService) Storage() models.Storage {
	var s models.Storage
	if entries, err := os.ReadDir(b.backupsDir()); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(b.backupsDir(), e.Name())
			_, size := countTree(dir)
			if _, err := readManifest(dir); err != nil {
				s.Partial++
				continue
			}
			s.Snapshots++
			s.SnapshotSize += size
		}
	}
	if entries, err := os.ReadDir(b.quarantineDir()); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			_, size := countTree(filepath.Join(b.quarantineDir(), e.Name()))
			s.Quarantine++
			s.QuarantineSize += size
		}
	}
	return s
}

// EmptyQuarantine drops the folders held back by previous restores.
func (b *BackupService) EmptyQuarantine() error {
	err := os.RemoveAll(b.quarantineDir())
	emitQuarantine()
	return err
}

// Restore puts a snapshot back. Live folders are moved aside first, never deleted,
// so a restore taken by mistake is still recoverable.
func (b *BackupService) Restore(id string) error {
	root := b.cfg.Get().GameRoot
	if root == "" {
		return errors.New("set your Sims 4 folder first")
	}
	src := filepath.Join(b.backupsDir(), id)
	if !fileExists(src) {
		return errors.New("backup not found")
	}
	// Moving live saves aside for an empty snapshot would lose them.
	if files, _ := b.countParts(src); files == 0 {
		return errors.New("snapshot is empty, nothing to restore")
	}

	if _, err := b.park(root, backupParts); err != nil {
		return err
	}

	_, _, err := b.copyParts(src, root, "Restoring")
	emit("done", 1, 1, "Restored")
	return err
}

// park moves live parts into one timestamped quarantine folder and returns it, or
// "" when nothing moved. Nothing is deleted.
func (b *BackupService) park(root string, parts []string) (string, error) {
	aside := uniqueDir(filepath.Join(b.quarantineDir(), time.Now().Format("2006-01-02_15-04-05")))
	moved := false
	for _, part := range parts {
		live := filepath.Join(root, part)
		if !fileExists(live) {
			continue
		}
		if err := os.MkdirAll(aside, 0o755); err != nil {
			return "", err
		}
		if err := movePath(live, filepath.Join(aside, part)); err != nil {
			return "", err
		}
		moved = true
	}
	if !moved {
		_ = os.Remove(aside) // nothing to park, drop the empty folder
		return "", nil
	}
	return aside, nil
}

// isBackupPart reports whether a parked name is saves/Tray, not a removed mod.
func isBackupPart(name string) bool {
	for _, part := range backupParts {
		if strings.EqualFold(part, name) {
			return true
		}
	}
	return false
}

// Quarantine lists what restores and mod removals parked, newest first.
func (b *BackupService) Quarantine() ([]models.Quarantine, error) {
	entries, err := os.ReadDir(b.quarantineDir())
	if err != nil {
		return []models.Quarantine{}, nil
	}
	out := []models.Quarantine{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(b.quarantineDir(), e.Name())
		subs, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		q := models.Quarantine{ID: e.Name(), Parts: []string{}}
		for _, s := range subs {
			q.Parts = append(q.Parts, s.Name())
			if s.IsDir() {
				files, size := countTree(filepath.Join(dir, s.Name()))
				q.Files += files
				q.Size += size
				continue
			}
			q.Files++
			if info, err := s.Info(); err == nil {
				q.Size += info.Size()
			}
		}
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// RestoreQuarantine puts one parked folder back. The live files it replaces are
// parked first, then the entry it came from is deleted.
func (b *BackupService) RestoreQuarantine(id string) error {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return errors.New("bad quarantine id")
	}
	root := b.cfg.Get().GameRoot
	if root == "" {
		return errors.New("set your Sims 4 folder first")
	}
	src := filepath.Join(b.quarantineDir(), id)
	subs, err := os.ReadDir(src)
	if err != nil || len(subs) == 0 {
		return errors.New("nothing set aside under that name")
	}

	// Sort first, refuse before touching anything: a mod already in Mods wins.
	// Loose files are skipped, since neither saves nor Mods has a slot for one.
	saves, mods := []string{}, []string{}
	for _, s := range subs {
		if !s.IsDir() {
			continue
		}
		if isBackupPart(s.Name()) {
			saves = append(saves, s.Name())
			continue
		}
		if fileExists(filepath.Join(root, "Mods", s.Name())) {
			return fmt.Errorf("%s is already in Mods", s.Name())
		}
		mods = append(mods, s.Name())
	}
	if len(saves)+len(mods) == 0 {
		return errors.New("nothing to put back")
	}

	if len(saves) > 0 {
		if _, err := b.park(root, saves); err != nil {
			return err
		}
	}

	for _, part := range saves {
		if err := movePath(filepath.Join(src, part), filepath.Join(root, part)); err != nil {
			return err
		}
	}
	for _, mod := range mods {
		if err := movePath(filepath.Join(src, mod), filepath.Join(root, "Mods", mod)); err != nil {
			return err
		}
	}
	// Spent: the contents are back and what they replaced is parked.
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	emit("done", 1, 1, "Put back")
	emitQuarantine()
	return nil
}

// OpenQuarantineWindow opens the set-aside list in its own window. A second
// call focuses that window.
func (b *BackupService) OpenQuarantineWindow() error {
	app := application.Get()
	if app == nil {
		return errors.New("no window available")
	}
	if b.setAside != nil {
		b.setAside.Restore()
		b.setAside.Focus()
		return nil
	}
	w := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                 "NooBoo - Set aside",
		Width:                 560,
		Height:                560,
		MinWidth:              420,
		MinHeight:             320,
		MaxWidth:              560,
		MaxHeight:             620,
		MaximiseButtonState:   application.ButtonDisabled,
		FullscreenButtonState: application.ButtonDisabled,
		BackgroundColour: application.NewRGB(226, 241, 250),
		URL:              "/?view=quarantine",
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		b.setAside = nil
	})
	b.setAside = w
	return nil
}

// emitQuarantine tells open windows the set-aside list changed.
func emitQuarantine() {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("quarantine", struct{}{})
}

// RevealQuarantine opens the set-aside folder.
func (b *BackupService) RevealQuarantine() error {
	if err := os.MkdirAll(b.quarantineDir(), 0o755); err != nil {
		return err
	}
	return openInExplorer(b.quarantineDir())
}

// Delete removes one snapshot.
func (b *BackupService) Delete(id string) error {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return errors.New("bad backup id")
	}
	return os.RemoveAll(filepath.Join(b.backupsDir(), id))
}

// Reveal opens the backups folder.
func (b *BackupService) Reveal() error {
	if err := os.MkdirAll(b.backupsDir(), 0o755); err != nil {
		return err
	}
	return openInExplorer(b.backupsDir())
}

// countParts totals the files inside a snapshot's part folders.
func (b *BackupService) countParts(dir string) (int, int64) {
	total := 0
	var size int64
	for _, part := range backupParts {
		files, bytes := countTree(filepath.Join(dir, part))
		total += files
		size += bytes
	}
	return total, size
}

// copyParts copies each part folder, reporting progress across all of them.
func (b *BackupService) copyParts(src, dst, phase string) (int, int64, error) {
	total := 0
	for _, part := range backupParts {
		files, _ := countTree(filepath.Join(src, part))
		total += files
	}
	if total == 0 {
		total = 1
	}

	done := 0
	var bytes int64
	for _, part := range backupParts {
		from := filepath.Join(src, part)
		if !fileExists(from) {
			continue
		}
		_, size, err := copyTree(from, filepath.Join(dst, part), func(string) {
			done++
			emit(phase, done, total, part)
		})
		if err != nil {
			return done, bytes, err
		}
		bytes += size
	}
	return done, bytes, nil
}

func writeManifest(dir string, info models.Backup) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), b, 0o644)
}

func readManifest(dir string) (models.Backup, error) {
	var info models.Backup
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(b, &info)
	return info, err
}

// emit pushes progress to the frontend. No-op outside a running app.
func emit(phase string, done, total int, label string) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit("progress", models.Progress{Phase: phase, Done: done, Total: total, Label: label})
}
