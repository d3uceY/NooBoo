package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicateFindsIdenticalBytes(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "A", "one.package"), "same")
	write(t, filepath.Join(mods, "B", "two.package"), "same")

	sets, err := NewModsService(cfg).FindDuplicates()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || len(sets[0].Files) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	if sets[0].Wasted != 4 {
		t.Errorf("wasted = %d, want 4", sets[0].Wasted)
	}
}

func TestDuplicateSkipsEmptyAndDifferentBytes(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "A", "one.package"), "")
	write(t, filepath.Join(mods, "B", "two.package"), "")
	write(t, filepath.Join(mods, "C", "three.package"), "abcd")
	write(t, filepath.Join(mods, "D", "four.package"), "wxyz")

	sets, err := NewModsService(cfg).FindDuplicates()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 0 {
		t.Fatalf("sets = %+v", sets)
	}
}

func TestDuplicateKeeperOrder(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "A", "one.package"), "same")
	write(t, filepath.Join(mods, "B", "two.package.off"), "same")

	sets, err := NewModsService(cfg).FindDuplicates()
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].Files[0].File != "A/one.package" {
		t.Fatalf("sets = %+v", sets)
	}
}

func TestSetAsideFileMovesAndPrunes(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Pack", "one.package"), "a")
	write(t, filepath.Join(mods, "Pack", "two.package"), "bb")

	svc := NewModsService(cfg)
	if err := svc.SetAsideFile("Pack/one.package"); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(mods, "Pack", "one.package")) {
		t.Fatal("the file is still in Mods")
	}
	if !fileExists(filepath.Join(mods, "Pack", "two.package")) {
		t.Fatal("the other file moved too")
	}
	if err := svc.SetAsideFile("Pack/two.package"); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(mods, "Pack")) {
		t.Fatal("the empty folder was left behind")
	}

	found := 0
	_ = filepath.WalkDir(filepath.Join(cfg.AppData(), "quarantine"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found++
		}
		return nil
	})
	if found != 2 {
		t.Fatalf("quarantine holds %d files, want 2", found)
	}
}

func TestSetAsideRejectsTraversal(t *testing.T) {
	cfg, _, _ := gameWithMods(t)
	if err := NewModsService(cfg).SetAsideFile("../outside.package"); err == nil {
		t.Fatal("traversal was accepted")
	}
}

func TestRestoreQuarantinePutsSetAsideFileBack(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Pack", "one.package"), "a")

	if err := NewModsService(cfg).SetAsideFile("Pack/one.package"); err != nil {
		t.Fatal(err)
	}
	backup := NewBackupService(cfg)
	list, err := backup.Quarantine()
	if err != nil || len(list) != 1 {
		t.Fatalf("quarantine = %+v / %v", list, err)
	}
	if err := backup.RestoreQuarantine(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "Pack", "one.package")) {
		t.Fatal("the file did not come back")
	}
}
