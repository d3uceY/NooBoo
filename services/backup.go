package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nooboo/models"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	return os.RemoveAll(b.quarantineDir())
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

	aside := uniqueDir(filepath.Join(b.quarantineDir(), time.Now().Format("2006-01-02_15-04-05")))
	moved := false
	for _, part := range backupParts {
		live := filepath.Join(root, part)
		if !fileExists(live) {
			continue
		}
		if err := os.MkdirAll(aside, 0o755); err != nil {
			return err
		}
		if err := os.Rename(live, filepath.Join(aside, part)); err != nil {
			// Rename fails across volumes; copy then clear instead.
			if _, _, cerr := copyTree(live, filepath.Join(aside, part), nil); cerr != nil {
				return cerr
			}
			if err := os.RemoveAll(live); err != nil {
				return err
			}
		}
		moved = true
	}
	if !moved {
		_ = os.Remove(aside) // nothing to park, drop the empty folder
	}

	_, _, err := b.copyParts(src, root, "Restoring")
	emit("done", 1, 1, "Restored")
	return err
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
