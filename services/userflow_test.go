package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nooboo/models"
)

// makeGame lays out the folders a real Sims 4 user folder has.
func makeGame(t *testing.T, root string) string {
	t.Helper()
	write(t, filepath.Join(root, "saves", "Slot_00000001.save"), "save")
	write(t, filepath.Join(root, "Tray", "household.bpi"), "tray")
	write(t, filepath.Join(root, "Options.ini"), "[Options]")
	return root
}

func makeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func modReq(src string) models.InstallRequest {
	base := filepath.Base(src)
	return models.InstallRequest{
		Source: src,
		Name:   strings.TrimSuffix(base, filepath.Ext(base)),
		Kind:   models.KindMod,
	}
}

func ccReq(src, name, creator string) models.InstallRequest {
	return models.InstallRequest{Source: src, Name: name, Creator: creator, Kind: models.KindCC}
}

// A user browses to "Electronic Arts" instead of "The Sims 4".
func TestPickingTheParentFolderStillWorks(t *testing.T) {
	cfg, base := newTestConfig(t)
	ea := filepath.Join(base, "Documents", "Electronic Arts")
	makeGame(t, filepath.Join(ea, "The Sims 4"))

	got := cfg.SetGameRoot(ea)
	if !got.Valid {
		t.Fatalf("expected the parent pick to resolve, got %q", got.Problem)
	}
	if got.Root != filepath.Join(ea, "The Sims 4") {
		t.Fatalf("resolved to %s", got.Root)
	}
}

// Picking Documents is the other common mistake.
func TestPickingDocumentsStillWorks(t *testing.T) {
	cfg, base := newTestConfig(t)
	docs := filepath.Join(base, "Documents")
	makeGame(t, filepath.Join(docs, "Electronic Arts", "The Sims 4"))

	if got := cfg.SetGameRoot(docs); !got.Valid {
		t.Fatalf("expected Documents to resolve, got %q", got.Problem)
	}
}

// Any other folder must be rejected and must not be saved.
func TestPickingWrongFolderIsRejectedAndNotSaved(t *testing.T) {
	cfg, base := newTestConfig(t)
	random := filepath.Join(base, "Downloads")
	if err := os.MkdirAll(random, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := cfg.SetGameRoot(random); got.Valid {
		t.Fatal("a random folder was accepted")
	}
	if cfg.Get().GameRoot != "" {
		t.Fatal("a rejected folder was persisted")
	}
}

// The game folder gets moved or renamed after it was chosen.
func TestFolderThatDisappearsReadsAsInvalid(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	if err := os.RemoveAll(game); err != nil {
		t.Fatal(err)
	}
	if cfg.Current().Valid {
		t.Fatal("a deleted folder still validated")
	}
}

// Someone drops a file called "Mods" where the folder should be.
func TestModsPathThatIsAFileDoesNotCrash(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods"), "not a folder")
	cfg.SetGameRoot(game)

	if _, err := NewModsService(cfg).Scan(); err != nil {
		t.Fatalf("scan should tolerate a bogus Mods path, got %v", err)
	}
}

// A brand new player has no Mods folder at all.
func TestInstallCreatesModsFolder(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "MCCC.package")
	write(t, src, "payload")

	if _, err := NewModsService(cfg).Install(modReq(src)); err != nil {
		t.Fatalf("install into a missing Mods folder failed: %v", err)
	}
	if !fileExists(filepath.Join(game, "Mods", "MCCC", "MCCC.package")) {
		t.Fatal("mod did not land in Mods/MCCC")
	}
}

// Download names routinely carry characters Windows refuses.
func TestInstallSanitizesWindowsIllegalNames(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "hair.package")
	write(t, src, "payload")

	mod, err := NewModsService(cfg).Install(ccReq(src, `What: "Even" <is> this? | *`, "Cre:ator?"))
	if err != nil {
		t.Fatalf("install with illegal characters failed: %v", err)
	}
	for _, bad := range []string{`\`, "/", ":", "*", "?", `"`, "<", ">", "|"} {
		if strings.Contains(mod.Name, bad) || strings.Contains(mod.Creator, bad) {
			t.Fatalf("illegal character %q survived in %q / %q", bad, mod.Name, mod.Creator)
		}
	}
	if !fileExists(mod.Path) {
		t.Fatalf("sanitized folder is not on disk: %s", mod.Path)
	}
}

// .rar and .7z need a real extractor, so the app must say so plainly.
func TestInstallRejectsRar(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "mod.rar")
	write(t, src, "rar")

	if _, err := NewModsService(cfg).Install(modReq(src)); err == nil {
		t.Fatal("a .rar was accepted")
	}
}

// Zips from creators usually wrap the packages in extra folders.
func TestInstallZipFlattensNestedFolders(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "pack.zip")
	makeZip(t, src, map[string]string{
		"pack/readme.txt":          "hello",
		"pack/cc/hair.package":     "a",
		"pack/cc/eyes.package":     "b",
		"pack/deeper/skin.package": "c",
	})

	mod, err := NewModsService(cfg).Install(ccReq(src, "Pack", "Creator"))
	if err != nil {
		t.Fatalf("zip install failed: %v", err)
	}

	entries, err := os.ReadDir(mod.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 packages flattened into one folder, got %d", len(entries))
	}
	if fileExists(filepath.Join(mod.Path, "readme.txt")) {
		t.Fatal("non-mod files were extracted")
	}
}

// A zip with nothing usable should fail loudly, not create an empty folder.
func TestInstallZipWithNoModsFails(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "junk.zip")
	makeZip(t, src, map[string]string{"notes.txt": "nothing here"})

	if _, err := NewModsService(cfg).Install(modReq(src)); err == nil {
		t.Fatal("a zip with no packages was accepted")
	}
}

// Windows users rename by hand, so the suffix arrives in any case.
func TestToggleHandlesUppercaseDisabledFile(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods", "Old", "OLD.PACKAGE.OFF"), "x")
	cfg.SetGameRoot(game)

	svc := NewModsService(cfg)
	list, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("uppercase .OFF should read as disabled: %+v", list)
	}

	if err := svc.SetEnabled([]string{list[0].ID}, true); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(game, "Mods", "Old", "OLD.PACKAGE")) {
		entries, _ := os.ReadDir(filepath.Join(game, "Mods", "Old"))
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("file was not re-enabled, still: %v", names)
	}
}

// The UI can ask for a mod that was deleted in another window.
func TestToggleUnknownIDIsIgnored(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	if err := os.MkdirAll(filepath.Join(game, "Mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewModsService(cfg).SetEnabled([]string{"Ghost/Mod"}, false); err != nil {
		t.Fatalf("unknown id should be skipped, got %v", err)
	}
}

// Ids are paths, so they must not escape the Mods folder.
func TestToggleRejectsTraversal(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	write(t, filepath.Join(base, "Outside", "secret.package"), "x")

	if err := NewModsService(cfg).SetEnabled([]string{"../Outside"}, false); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if !fileExists(filepath.Join(base, "Outside", "secret.package")) {
		t.Fatal("a file outside Mods was renamed")
	}
}

// Clicking "Back up now" twice in a row is normal; both must survive.
func TestTwoBackupsInTheSameSecond(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	first, err := svc.BackupNow()
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.BackupNow()
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("second backup reused the id %s", first.ID)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(list))
	}
}

// A player who deleted their saves folder should still be able to back up.
func TestBackupWithoutSavesStillSucceeds(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	cfg.SetGameRoot(game)

	if _, err := NewBackupService(cfg).BackupNow(); err != nil {
		t.Fatalf("backup with no saves folder failed: %v", err)
	}
}

// Restoring twice in the same second must not collide in quarantine.
func TestTwoRestoresInTheSameSecond(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	snap, err := svc.BackupNow()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(snap.ID); err != nil {
		t.Fatalf("first restore failed: %v", err)
	}
	if err := svc.Restore(snap.ID); err != nil {
		t.Fatalf("second restore failed: %v", err)
	}
}

// The settings file gets truncated by a crash or hand-editing.
func TestCorruptConfigFallsBackToDefaults(t *testing.T) {
	cfg, base := newTestConfig(t)
	write(t, filepath.Join(base, "config.json"), "{ not json")

	if cfg.Get().GameRoot != "" {
		t.Fatal("a corrupt config produced a game root")
	}
}

// Nothing is detected on a machine that has never run the game.
func TestNoGameFolderIsReportedCleanly(t *testing.T) {
	cfg, _ := newTestConfig(t)
	if cfg.Current().Valid {
		t.Fatal("a fresh install reported a valid folder")
	}
	if list, err := NewModsService(cfg).Scan(); err != nil || len(list) != 0 {
		t.Fatalf("scan without a folder should be empty, got %v / %v", list, err)
	}
}

// Script mods are a single .ts4script and toggle the same way.
func TestScriptModToggles(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.ts4script"), "script")
	cfg.SetGameRoot(game)

	svc := NewModsService(cfg)
	list, err := svc.Scan()
	if err != nil || len(list) != 1 || !list[0].Enabled {
		t.Fatalf("script mod not found or not enabled: %+v / %v", list, err)
	}

	if err := svc.SetEnabled([]string{list[0].ID}, false); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(game, "Mods", "MCCC", "MCCC.ts4script.off")) {
		t.Fatal("script mod was not renamed")
	}
}

// Users reinstall to update a mod; the folder must be reused, not duplicated.
func TestReinstallUpdatesInPlace(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewModsService(cfg)
	oldSrc := filepath.Join(base, "dl", "v1", "MCCC.package")
	write(t, oldSrc, "old")
	if _, err := svc.Install(modReq(oldSrc)); err != nil {
		t.Fatal(err)
	}

	newSrc := filepath.Join(base, "dl", "v2", "MCCC.package")
	write(t, newSrc, "new")
	if _, err := svc.Install(modReq(newSrc)); err != nil {
		t.Fatal(err)
	}

	list, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("reinstall duplicated the mod: %+v", list)
	}
	body, err := os.ReadFile(filepath.Join(game, "Mods", "MCCC", "MCCC.package"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "new" {
		t.Fatalf("file was not replaced, got %q", body)
	}
}

// The game's own Resource.cfg and stray OS files are not mods.
func TestScanIgnoresGameFiles(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	modsDir := filepath.Join(game, "Mods")
	write(t, filepath.Join(modsDir, "Resource.cfg"), "cfg")
	write(t, filepath.Join(modsDir, ".DS_Store"), "junk")
	if err := os.MkdirAll(filepath.Join(modsDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(modsDir, "Real", "real.package"), "x")
	cfg.SetGameRoot(game)

	list, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Real" {
		t.Fatalf("game files leaked into the list: %+v", list)
	}
}

// Clearing the name field must still produce a usable folder.
func TestInstallWithoutNameFallsBack(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "dl", "thing.package")
	write(t, src, "x")

	mod, err := NewModsService(cfg).Install(ccReq(src, "   ", ""))
	if err != nil {
		t.Fatalf("blank name should still install: %v", err)
	}
	if mod.Name == "" || strings.ContainsAny(mod.Name, `/\:`) {
		t.Fatalf("bad fallback folder name %q", mod.Name)
	}
}

// A restore that was already deleted should say so, not crash.
func TestRestoreMissingSnapshotReports(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	snap, err := svc.BackupNow()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(snap.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(snap.ID); err == nil {
		t.Fatal("restoring a deleted snapshot reported success")
	}
}

// Removing a mod must be recoverable, not a shred.
func TestDeleteModSetsItAside(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.package"), "payload")
	cfg.SetGameRoot(game)

	svc := NewModsService(cfg)
	list, err := svc.Scan()
	if err != nil || len(list) != 1 {
		t.Fatalf("setup failed: %+v / %v", list, err)
	}

	if err := svc.Delete(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(game, "Mods", "MCCC")) {
		t.Fatal("mod folder still in Mods")
	}
	if remaining, _ := svc.Scan(); len(remaining) != 0 {
		t.Fatalf("scan still reports the mod: %+v", remaining)
	}

	// The files must survive somewhere the player can reach.
	found := false
	_ = filepath.WalkDir(filepath.Join(cfg.AppData(), "quarantine"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "MCCC.package" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("deleted mod left no recoverable copy")
	}
}

func TestDeleteModRejectsTraversal(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)
	write(t, filepath.Join(base, "Outside", "keep.package"), "x")

	if err := NewModsService(cfg).Delete("../Outside"); err == nil {
		t.Fatal("traversal id was accepted")
	}
	if !fileExists(filepath.Join(base, "Outside", "keep.package")) {
		t.Fatal("a folder outside Mods was moved")
	}
}

func TestDeleteUnknownModReports(t *testing.T) {
	cfg, base := newTestConfig(t)
	cfg.SetGameRoot(makeGame(t, filepath.Join(base, "The Sims 4")))
	if err := os.MkdirAll(filepath.Join(base, "The Sims 4", "Mods"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewModsService(cfg).Delete("Ghost"); err == nil {
		t.Fatal("deleting a missing mod reported success")
	}
}
