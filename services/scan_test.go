package services

import (
	"path/filepath"
	"testing"

	"nooboo/models"
)

// One walk has to answer everything the row shows.
func TestReadModCollectsEnabledSizeAndNewest(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.package"), "abc")
	write(t, filepath.Join(root, "b.package.off"), "de")
	write(t, filepath.Join(root, "sub", "c.ts4script"), "fghij")

	enabled, size, files, newest := readMod(root)
	if !enabled {
		t.Fatal("a folder with a live .package reads as off")
	}
	if size != 10 {
		t.Errorf("size = %d, want 10", size)
	}
	if len(files) != 3 {
		t.Errorf("files = %v, want three", files)
	}
	if newest.IsZero() {
		t.Error("newest mtime is zero")
	}
}

// Another manager wrote .disabled; the file must still read as off and turn on.
func TestDisabledSuffixIsUnderstood(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	write(t, filepath.Join(game, "Mods", "Other", "Hair.package.disabled"), "x")
	cfg.SetGameRoot(game)

	svc := NewModsService(cfg)
	list, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("a .disabled file read as %+v", list)
	}

	if err := svc.SetEnabled([]string{list[0].ID}, true); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(game, "Mods", "Other", "Hair.package")) {
		t.Fatal("enabling did not strip .disabled")
	}
}

// A script in a subfolder is still one mod, not a creator folder.
func TestScanTreatsNestedScriptAsOneMod(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Scripted", "Scripts", "main.ts4script"), "x")

	m, ok := scanAll(t, cfg)["Scripted"]
	if !ok {
		t.Fatalf("want one mod named Scripted, got %+v", scanAll(t, cfg))
	}
	if m.Kind != models.KindMod {
		t.Errorf("kind = %q, want mod", m.Kind)
	}
}

func TestTagsPersistAcrossToggle(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "MCCC", "MCCC.package"), "x")

	svc := NewModsService(cfg)
	if err := svc.AddTag("MCCC", "keep"); err != nil {
		t.Fatal(err)
	}
	if got := scanAll(t, cfg)["MCCC"]; len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Fatalf("tags = %v", got.Tags)
	}

	if err := svc.SetEnabled([]string{"MCCC"}, false); err != nil {
		t.Fatal(err)
	}
	if got := scanAll(t, cfg)["MCCC"]; len(got.Tags) != 1 {
		t.Fatalf("tag lost after toggle: %v", got.Tags)
	}

	if err := svc.RemoveTag("MCCC", "keep"); err != nil {
		t.Fatal(err)
	}
	if got := scanAll(t, cfg)["MCCC"]; len(got.Tags) != 0 {
		t.Fatalf("tag removal failed: %v", got.Tags)
	}
}

func TestTagSanitize(t *testing.T) {
	if got := sanitizeTag("  alpha   cc \n"); got != "alpha cc" {
		t.Errorf("sanitize = %q", got)
	}
	long := sanitizeTag("123456789012345678901234567890")
	if len([]rune(long)) != maxTagLength {
		t.Errorf("length = %d", len([]rune(long)))
	}
}

// A switched-off package still knows what it is.
func TestDisabledPackageKeepsItsKind(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	writeDbpf(t, filepath.Join(mods, "Off", "x.package.off"),
		[]testRes{{typ: 0x034AEECB, inst: 1, data: []byte("x")}})

	m := scanAll(t, cfg)["Off"]
	if m.Category != "CAS" {
		t.Fatalf("category = %q, want CAS", m.Category)
	}
	if m.Enabled {
		t.Fatal("a .package.off file read as enabled")
	}
}

// A script deep inside a creator folder is an item, not a signal that the
// whole creator is one mod.
func TestScanKeepsDeepScriptAsCC(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Creator", "Item", "Scripts", "x.ts4script"), "x")

	m, ok := scanAll(t, cfg)["Creator/Item"]
	if !ok {
		t.Fatalf("want Creator/Item, got %+v", scanAll(t, cfg))
	}
	if m.Kind != models.KindCC || m.Creator != "Creator" {
		t.Errorf("read as %+v", m)
	}
}
