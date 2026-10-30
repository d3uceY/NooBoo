package services

import (
	"os"
	"path/filepath"
	"testing"

	"nooboo/models"
)

// newTestConfig builds a ConfigService rooted in a throwaway folder.
func newTestConfig(t *testing.T) (*ConfigService, string) {
	t.Helper()
	cfg := &ConfigService{base: t.TempDir()}
	return cfg, cfg.base
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndToggle(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	mods := filepath.Join(game, "Mods")

	write(t, filepath.Join(mods, "MCCC", "MCCC.package"), "a")
	write(t, filepath.Join(mods, "CreatorA", "Hair 01", "hair.package"), "b")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")

	if g := cfg.SetGameRoot(game); !g.Valid {
		t.Fatalf("folder rejected: %q", g.Problem)
	}

	svc := NewModsService(cfg)
	list, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 installs, got %d: %+v", len(list), list)
	}

	byID := map[string]bool{}
	for _, m := range list {
		byID[m.ID] = m.Enabled
		if !m.Enabled {
			t.Fatalf("%s should start enabled", m.ID)
		}
	}
	if _, ok := byID["MCCC"]; !ok {
		t.Fatalf("mod id missing: %v", byID)
	}

	// Disabling renames the file, leaving the folder in place.
	if err := svc.SetEnabled([]string{"MCCC"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mods, "MCCC", "MCCC.package.off")); err != nil {
		t.Fatalf("expected .off file: %v", err)
	}

	list, err = svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.ID == "MCCC" && m.Enabled {
			t.Fatal("MCCC still reads as enabled")
		}
		if m.ID != "MCCC" && !m.Enabled {
			t.Fatalf("%s changed unexpectedly", m.ID)
		}
	}

	// Enabling restores the real extension.
	if err := svc.SetEnabled([]string{"MCCC"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mods, "MCCC", "MCCC.package")); err != nil {
		t.Fatalf("expected .package back: %v", err)
	}
}

func TestInstallPutsModsInFolders(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "game")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")
	cfg.SetGameRoot(game)

	src := filepath.Join(base, "downloads", "cool-hair.package")
	write(t, src, "payload")

	svc := NewModsService(cfg)
	mod, err := svc.Install(models.InstallRequest{
		Source:  src,
		Name:    "Cool Hair",
		Creator: "CreatorA",
		Kind:    models.KindCC,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(game, "Mods", "CreatorA", "Cool Hair", "cool-hair.package")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("want %s: %v", want, err)
	}
	if mod.Creator != "CreatorA" || !mod.Enabled {
		t.Fatalf("unexpected mod: %+v", mod)
	}
}

func TestBackupAndRestore(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "game")
	saveFile := filepath.Join(game, "saves", "Slot_00000001.save")

	write(t, saveFile, "v1")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")
	cfg.SetGameRoot(game)

	svc := NewBackupService(cfg)
	snap, err := svc.BackupNow()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files == 0 {
		t.Fatal("snapshot counted no files")
	}

	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("want 1 snapshot, got %d (%v)", len(list), err)
	}

	// Overwrite, then roll back.
	write(t, saveFile, "v2")
	if err := svc.Restore(snap.ID); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(saveFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("restore returned %q, want v1", got)
	}

	// The newer save must still exist somewhere.
	found := false
	_ = filepath.WalkDir(svc.quarantineDir(), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if body, err := os.ReadFile(p); err == nil && string(body) == "v2" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("previous save was not kept in quarantine")
	}
}

func TestRestoreRejectsPathTraversal(t *testing.T) {
	cfg, _ := newTestConfig(t)
	svc := NewBackupService(cfg)
	if err := svc.Delete("../escape"); err == nil {
		t.Fatal("expected delete to reject a traversal id")
	}
}
