package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nooboo/models"
)

// seedSnapshot writes a snapshot folder with a manifest, as BackupNow would.
func seedSnapshot(t *testing.T, svc *BackupService, id string, created time.Time, withData bool) {
	t.Helper()
	dir := filepath.Join(svc.backupsDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withData {
		write(t, filepath.Join(dir, "saves", "Slot_00000001.save"), "snapshot")
		write(t, filepath.Join(dir, "Tray", "item.bpi"), "snapshot")
	}
	if err := writeManifest(dir, seedBackup(id, created)); err != nil {
		t.Fatal(err)
	}
}

// A crash mid-copy leaves a folder with no manifest.
func TestPartialBackupIsHiddenFromList(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	partial := filepath.Join(svc.backupsDir(), "2026-01-01_00-00-00")
	write(t, filepath.Join(partial, "saves", "half-copied.save"), "x")

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("a manifest-less folder was listed: %+v", list)
	}
}

// Restoring a snapshot that has no content must not touch live saves.
func TestRestoreRefusesEmptySnapshot(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	seedSnapshot(t, svc, "2026-01-01_00-00-00", time.Now(), false)

	err := svc.Restore("2026-01-01_00-00-00")
	if err == nil {
		t.Fatal("restoring an empty snapshot reported success")
	}
	if !fileExists(filepath.Join(game, "saves", "Slot_00000001.save")) {
		t.Fatal("live saves were moved away by a failed restore")
	}
}

// Same-second suffixes (-2, -10) must sort by time, not by string.
func TestListSortsNewestFirstAcrossSuffixes(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{
		"2026-01-01_00-00-00",
		"2026-01-01_00-00-00-2",
		"2026-01-01_00-00-00-10",
	}
	for i, id := range ids {
		seedSnapshot(t, svc, id, at.Add(time.Duration(i)*time.Second), true)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 snapshots, got %d", len(list))
	}
	if list[0].ID != "2026-01-01_00-00-00-10" {
		t.Fatalf("newest snapshot should sort first, got %q", list[0].ID)
	}
}

func seedBackup(id string, created time.Time) models.Backup {
	return models.Backup{ID: id, Created: created.UTC().Format(time.RFC3339), Files: 2, Size: 16}
}

// sweeper builds a service with no age guard so fresh debris is fair game.
func sweeper(cfg *ConfigService) *BackupService {
	return &BackupService{cfg: cfg}
}

func TestCleanupRemovesOnlyPartials(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := sweeper(cfg)
	seedSnapshot(t, svc, "2026-01-01_00-00-00", time.Now(), true)
	write(t, filepath.Join(svc.backupsDir(), "2026-01-02_00-00-00", "saves", "half.save"), "x")
	write(t, filepath.Join(svc.backupsDir(), "2026-01-03_00-00-00", "Tray", "half.bpi"), "x")

	removed, err := svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 partials removed, got %d", removed)
	}
	if !fileExists(filepath.Join(svc.backupsDir(), "2026-01-01_00-00-00", "backup.json")) {
		t.Fatal("cleanup ate a real snapshot")
	}
	if list, _ := svc.List(); len(list) != 1 {
		t.Fatalf("expected 1 snapshot to survive, got %d", len(list))
	}
}

// Production keeps a guard so a copy in flight is never swept.
func TestCleanupSparesFreshWorkInProgress(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := NewBackupService(cfg) // 10 minute guard
	inFlight := filepath.Join(svc.backupsDir(), "2026-01-01_00-00-00")
	write(t, filepath.Join(inFlight, "saves", "copying.save"), "x")

	removed, err := svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("swept a snapshot that was still being written (%d)", removed)
	}
	if !fileExists(filepath.Join(inFlight, "saves", "copying.save")) {
		t.Fatal("in-flight folder was deleted")
	}
}

func TestBackupSweepsOldPartialsFirst(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	write(t, filepath.Join(svc.backupsDir(), "2026-01-01_00-00-00", "saves", "junk.save"), "x")

	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(svc.backupsDir(), "2026-01-01_00-00-00")) {
		t.Fatal("stale partial survived a new backup")
	}
	if list, _ := svc.List(); len(list) != 1 {
		t.Fatalf("expected exactly 1 snapshot, got %d", len(list))
	}
}

func TestStorageCountsSnapshotsQuarantineAndPartials(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore((mustList(t, svc))[0].ID); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(svc.backupsDir(), "2026-02-02_00-00-00", "saves", "junk.save"), "x")

	st := svc.Storage()
	if st.Snapshots != 1 {
		t.Fatalf("want 1 snapshot, got %d", st.Snapshots)
	}
	if st.SnapshotSize <= 0 {
		t.Fatal("snapshot size was not measured")
	}
	if st.Partial != 1 {
		t.Fatalf("want 1 partial, got %d", st.Partial)
	}
	if st.Quarantine != 1 || st.QuarantineSize <= 0 {
		t.Fatalf("quarantine not reported: %+v", st)
	}
}

func TestEmptyQuarantineKeepsSnapshots(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore((mustList(t, svc))[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := svc.EmptyQuarantine(); err != nil {
		t.Fatal(err)
	}
	if st := svc.Storage(); st.Quarantine != 0 {
		t.Fatalf("quarantine not emptied: %+v", st)
	}
	if list, _ := svc.List(); len(list) != 1 {
		t.Fatalf("emptying quarantine touched snapshots: %d", len(list))
	}
}

func TestDeleteLeavesOtherSnapshotsAlone(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	seedSnapshot(t, svc, "2026-01-01_00-00-00", time.Now(), true)
	seedSnapshot(t, svc, "2026-01-02_00-00-00", time.Now().Add(time.Hour), true)

	if err := svc.Delete("2026-01-01_00-00-00"); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.List()
	if len(list) != 1 || list[0].ID != "2026-01-02_00-00-00" {
		t.Fatalf("delete removed the wrong snapshot: %+v", list)
	}
}

func TestCleanupWithNoBackupsFolder(t *testing.T) {
	cfg, _ := newTestConfig(t)
	removed, err := sweeper(cfg).Cleanup()
	if err != nil || removed != 0 {
		t.Fatalf("cleanup on a fresh install: %d / %v", removed, err)
	}
}

func mustList(t *testing.T, svc *BackupService) []models.Backup {
	t.Helper()
	list, err := svc.List()
	if err != nil || len(list) == 0 {
		t.Fatalf("no snapshots: %v", err)
	}
	return list
}

// The view button needs a name, a size and what is inside, or it shows a number.
func TestQuarantineListsWhatRestoreParked(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(mustList(t, svc)[0].ID); err != nil {
		t.Fatal(err)
	}

	list, err := svc.Quarantine()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected the parked saves, got %+v", list)
	}
	if list[0].Files < 2 || list[0].Size <= 0 {
		t.Fatalf("parked folder reads as empty: %+v", list[0])
	}
	if strings.Join(list[0].Parts, ",") != "Tray,saves" {
		t.Fatalf("parts wrong: %v", list[0].Parts)
	}
}

// Putting back the saves a restore parked must bring the old files back, and
// must park the ones it replaces rather than deleting them.
func TestRestoreQuarantinePutsParkedSavesBack(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := sweeper(cfg)
	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(mustList(t, svc)[0].ID); err != nil {
		t.Fatal(err)
	}
	parked, err := svc.Quarantine()
	if err != nil || len(parked) != 1 {
		t.Fatalf("expected one parked folder: %+v %v", parked, err)
	}

	if err := svc.RestoreQuarantine(parked[0].ID); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(game, "saves", "Slot_00000001.save"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "save" {
		t.Fatalf("live saves hold %q, want the parked copy", body)
	}
	// The entry that was put back is deleted; only the copy it replaced remains.
	after, err := svc.Quarantine()
	if err != nil || len(after) != 1 {
		t.Fatalf("the replaced saves were not parked: %+v %v", after, err)
	}
	if after[0].ID == parked[0].ID {
		t.Fatalf("the put-back entry %q is still listed", parked[0].ID)
	}
	if fileExists(filepath.Join(svc.quarantineDir(), parked[0].ID)) {
		t.Fatal("the put-back entry was left on disk")
	}
}

// A stray file in an entry has nowhere to go, so nothing is moved or deleted.
func TestRestoreQuarantineSkipsLooseFiles(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := sweeper(cfg)
	aside := filepath.Join(svc.quarantineDir(), "2026-01-01_00-00-00")
	write(t, filepath.Join(aside, "notes.txt"), "stray")
	write(t, filepath.Join(aside, "saves", "Slot_00000001.save"), "parked")

	if err := svc.RestoreQuarantine("2026-01-01_00-00-00"); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(game, "Mods", "notes.txt")) {
		t.Fatal("a loose file was dropped into Mods")
	}
	if fileExists(aside) {
		t.Fatal("the spent entry was left on disk")
	}
}

// An entry holding nothing puttable back is refused instead of quietly deleted.
func TestRestoreQuarantineRefusesEmptyEntry(t *testing.T) {
	cfg, _ := newTestConfig(t)
	svc := sweeper(cfg)
	aside := filepath.Join(svc.quarantineDir(), "2026-01-01_00-00-00")
	write(t, filepath.Join(aside, "notes.txt"), "stray")

	if err := svc.RestoreQuarantine("2026-01-01_00-00-00"); err == nil {
		t.Fatal("an entry with nothing to put back reported success")
	}
	if !fileExists(filepath.Join(aside, "notes.txt")) {
		t.Fatal("the refused entry was deleted")
	}
}

// A mod is never overwritten: the entry stays put and the call says why.
func TestRestoreQuarantineRefusesModAlreadyInMods(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.package"), "live")

	svc := sweeper(cfg)
	aside := filepath.Join(svc.quarantineDir(), "2026-01-01_00-00-00")
	write(t, filepath.Join(aside, "MCCC", "MCCC.package"), "parked")

	if err := svc.RestoreQuarantine("2026-01-01_00-00-00"); err == nil {
		t.Fatal("a mod already in Mods was overwritten")
	}
	if !fileExists(filepath.Join(aside, "MCCC", "MCCC.package")) {
		t.Fatal("the refused entry was consumed")
	}
}

func TestRestoreQuarantineRejectsPathTraversal(t *testing.T) {
	cfg, _ := newTestConfig(t)
	svc := sweeper(cfg)
	if err := svc.RestoreQuarantine("../backups"); err == nil {
		t.Fatal("a traversal id was accepted")
	}
}

// Snapshots must not pile up forever; the oldest go once the cap is reached.
func TestBackupPrunesToKeepLast(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))
	if err := cfg.SetKeepLast(3); err != nil {
		t.Fatal(err)
	}

	svc := sweeper(cfg)
	for i := 0; i < 5; i++ {
		if _, err := svc.BackupNow(); err != nil {
			t.Fatalf("backup %d failed: %v", i, err)
		}
	}

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("keep-last 3 left %d snapshots", len(list))
	}
}

func TestBackupUnderLimitKeepsThemAll(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))

	svc := sweeper(cfg)
	for i := 0; i < 2; i++ {
		if _, err := svc.BackupNow(); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := svc.List(); len(list) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(list))
	}
}

func TestKeepLastDefaultsAndClamps(t *testing.T) {
	cfg, _ := newTestConfig(t)
	if got := cfg.KeepLast(); got != defaultKeepLast {
		t.Fatalf("default should be %d, got %d", defaultKeepLast, got)
	}
	if err := cfg.SetKeepLast(0); err != nil {
		t.Fatal(err)
	}
	if got := cfg.KeepLast(); got != 1 {
		t.Fatalf("zero should clamp to 1, got %d", got)
	}
	if err := cfg.SetKeepLast(9999); err != nil {
		t.Fatal(err)
	}
	if got := cfg.KeepLast(); got != 200 {
		t.Fatalf("huge value should clamp to 200, got %d", got)
	}
}

// The row detail needs real file names, not just a count.
func TestScanReportsFilesAndModified(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.package"), "a")
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.ts4script"), "b")
	cfg.SetGameRoot(game)

	list, err := NewModsService(cfg).Scan()
	if err != nil || len(list) != 1 {
		t.Fatalf("scan: %+v / %v", list, err)
	}
	mod := list[0]
	if len(mod.Files) != 2 {
		t.Fatalf("expected 2 file names, got %v", mod.Files)
	}
	if mod.Modified == "" {
		t.Fatal("modified time was not reported")
	}
	if _, err := time.Parse(time.RFC3339, mod.Modified); err != nil {
		t.Fatalf("modified is not RFC3339: %q", mod.Modified)
	}
}
