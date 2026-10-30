package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nooboo/models"
)

// gameWithMods builds a throwaway game folder and returns the Mods path.
func gameWithMods(t *testing.T) (*ConfigService, string, string) {
	t.Helper()
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	mods := filepath.Join(game, "Mods")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	if g := cfg.SetGameRoot(game); !g.Valid {
		t.Fatalf("folder rejected: %q", g.Problem)
	}
	return cfg, game, mods
}

func scanAll(t *testing.T, cfg *ConfigService) map[string]models.Mod {
	t.Helper()
	got, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]models.Mod{}
	for _, m := range got {
		out[m.ID] = m
	}
	return out
}

func TestLooseFileDetailListsItsOwnName(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "MCCC.package"), "abc")

	got := scanAll(t, cfg)["MCCC.package"]
	if len(got.Files) != 1 || got.Files[0] != "MCCC.package" {
		t.Fatalf("files = %v, want the file name rather than \".\"", got.Files)
	}
	if got.Size != 3 {
		t.Errorf("size = %d, want 3", got.Size)
	}
}

func TestScanHandlesShoutingExtensions(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "LOUD.PACKAGE"), "a")
	write(t, filepath.Join(mods, "SCRIPT.TS4SCRIPT.OFF"), "b")
	write(t, filepath.Join(mods, "resource.CFG"), "c")

	all := scanAll(t, cfg)
	if m, ok := all["LOUD.PACKAGE"]; !ok || !m.Enabled || m.Name != "LOUD" {
		t.Fatalf("upper-case package read as %+v", m)
	}
	if m, ok := all["SCRIPT.TS4SCRIPT.OFF"]; !ok || m.Enabled || m.Name != "SCRIPT" {
		t.Fatalf("upper-case off file read as %+v", m)
	}
	if _, ok := all["resource.CFG"]; ok {
		t.Error("Resource.cfg should never be listed, whatever its case")
	}
}

func TestScanTreatsTooDeepAsOneMod(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Downloaded", "Pretty Hair", "Sub", "hair.package"), "a")

	all := scanAll(t, cfg)
	m, ok := all["Downloaded/Pretty Hair"]
	if !ok {
		t.Fatalf("want creator/child id, got %v", all)
	}
	if m.Creator != "Downloaded" || m.Kind != models.KindCC || m.Name != "Pretty Hair" {
		t.Fatalf("read as %+v", m)
	}
}

func TestScanFolderWithFilesAndSubfolders(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Mixed", "a.package"), "a")
	write(t, filepath.Join(mods, "Mixed", "extras", "b.package"), "b")

	m := scanAll(t, cfg)["Mixed"]
	if m.Kind != models.KindMod {
		t.Errorf("kind = %q, want mod", m.Kind)
	}
	if len(m.Files) != 2 {
		t.Errorf("files = %v, want both", m.Files)
	}
}

func TestInstallFromAFolderTakesEveryModFile(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "Some Mod Pack")
	write(t, filepath.Join(src, "main.package"), "a")
	write(t, filepath.Join(src, "scripts", "behaviour.ts4script"), "b")
	write(t, filepath.Join(src, "scripts", "helper.py"), "not loadable")
	write(t, filepath.Join(src, "readme.txt"), "ignore me")
	write(t, filepath.Join(src, ".git", "config"), "ignore me too")

	got, err := NewModsService(cfg).Install(models.InstallRequest{
		Source: src,
		Name:   "Some Mod Pack",
		Kind:   models.KindMod,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Flattened, so the game loads it from one folder deep.
	if !fileExists(filepath.Join(mods, "Some Mod Pack", "main.package")) ||
		!fileExists(filepath.Join(mods, "Some Mod Pack", "behaviour.ts4script")) {
		t.Fatal("a loadable file was not copied")
	}
	if fileExists(filepath.Join(mods, "Some Mod Pack", "helper.py")) ||
		fileExists(filepath.Join(mods, "Some Mod Pack", "readme.txt")) {
		t.Error("a file the game cannot load was copied")
	}
	if got.Name != "Some Mod Pack" || len(got.Files) != 2 {
		t.Errorf("described as %+v", got)
	}
}

func TestInstallFromAFolderKeepsSwitchedOffFiles(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "Half Off")
	write(t, filepath.Join(src, "on.package"), "a")
	write(t, filepath.Join(src, "off.package.off"), "b")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Half Off", Kind: models.KindMod}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "Half Off", "off.package.off")) {
		t.Fatal("a .off file lost its suffix on the way in")
	}
}

func TestInstallFromAFolderForCCUsesCreator(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "Garden Pack")
	write(t, filepath.Join(src, "garden.package"), "a")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{
		Source: src, Name: "Garden Pack", Creator: "Harrie", Kind: models.KindCC,
	}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "Harrie", "Garden Pack", "garden.package")) {
		t.Fatal("CC from a folder did not land under the creator")
	}
}

func TestInstallFromAFolderWithNoModsFails(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "Docs Only")
	write(t, filepath.Join(src, "readme.txt"), "nothing here")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Docs Only", Kind: models.KindMod}); err == nil {
		t.Fatal("want an error for a folder with no mod files")
	}
	if fileExists(filepath.Join(mods, "Docs Only")) {
		t.Error("the empty folder was left behind")
	}
}

func TestInstallFromAFolderKeepsOneOfEachName(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "Dupes")
	write(t, filepath.Join(src, "same.package"), "first")
	write(t, filepath.Join(src, "sub", "same.package"), "second")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Dupes", Kind: models.KindMod}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(mods, "Dupes", "same.package"))
	if err != nil {
		t.Fatal(err)
	}
	// filepath.WalkDir is lexical, so the top-level file wins.
	if string(body) != "first" {
		t.Errorf("kept %q, want the shallowest copy", body)
	}
}

func TestInstallRejectsAnUnknownFileType(t *testing.T) {
	cfg, game, _ := gameWithMods(t)
	src := filepath.Join(game, "downloads", "archive.rar")
	write(t, src, "x")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Rar", Kind: models.KindMod}); err == nil {
		t.Fatal("want an error for a .rar")
	}
}

func TestInstallKeepsPriorityAndBracketPrefixes(t *testing.T) {	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "!MCCC.package")
	write(t, src, "x")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "!MCCC", Kind: models.KindMod}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "!MCCC", "!MCCC.package")) {
		t.Fatal("the ! prefix was not kept")
	}

	bracket := filepath.Join(game, "downloads", "[Harrie] Kitchen.package")
	write(t, bracket, "y")
	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: bracket, Name: "[Harrie] Kitchen", Kind: models.KindMod}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "[Harrie] Kitchen", "[Harrie] Kitchen.package")) {
		t.Fatal("the [bracket] prefix was not kept")
	}
}

func TestFolderSafeKeepsBracketsAndDropsIllegal(t *testing.T) {
	if got := folderSafe("[Harrie] Kitchen"); got != "[Harrie] Kitchen" {
		t.Errorf("brackets changed: %q", got)
	}
	if got := folderSafe(`a:b|c?`); got != "a-b-c" {
		t.Errorf("illegal characters kept: %q", got)
	}
	if got := folderSafe("trailing. "); got != "trailing" {
		t.Errorf("trailing dot and space kept: %q", got)
	}
}

func TestInstallZipCannotEscapeTheModFolder(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "evil.zip")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"../escaped.package", "deep/er/still/deep.package"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("x"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Evil", Kind: models.KindMod}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "Evil", "escaped.package")) {
		t.Error("a zip entry with ../ should still land inside the mod folder")
	}
	if fileExists(filepath.Join(mods, "escaped.package")) {
		t.Error("a zip entry escaped into Mods")
	}
	if fileExists(filepath.Join(game, "escaped.package")) {
		t.Error("a zip entry escaped outside Mods")
	}
}

func TestInstallThatFindsNoModsLeavesNothingBehind(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "empty.zip")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("readme.txt")
	_, _ = w.Write([]byte("nothing useful"))
	_ = zw.Close()
	_ = f.Close()

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Nothing", Kind: models.KindMod}); err == nil {
		t.Fatal("want an error for a zip with no mods")
	}
	if fileExists(filepath.Join(mods, "Nothing")) {
		t.Error("the empty folder was left behind")
	}
}

func TestCCWithoutCreatorLandsAsAMod(t *testing.T) {
	cfg, game, mods := gameWithMods(t)
	src := filepath.Join(game, "downloads", "hair.package")
	write(t, src, "x")

	if _, err := NewModsService(cfg).Install(models.InstallRequest{Source: src, Name: "Hair", Kind: models.KindCC}); err != nil {
		t.Fatal(err)
	}
	// Nothing on disk says "creator", so the next scan cannot tell this is CC.
	// Pinned here so a change to that rule is a deliberate one.
	if !fileExists(filepath.Join(mods, "Hair", "hair.package")) {
		t.Fatal("expected Mods/Hair/hair.package")
	}
	if m := scanAll(t, cfg)["Hair"]; m.Kind != models.KindMod {
		t.Errorf("kind = %q, want mod", m.Kind)
	}
}

func TestTidyGroupsAPackageAndItsScript(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "WickedWhims.package"), "a")
	write(t, filepath.Join(mods, "WickedWhims.ts4script"), "b")

	svc := NewModsService(cfg)
	res, err := svc.Tidy()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 2 || res.Folders != 1 {
		t.Fatalf("moved %d into %d folders, want 2 into 1: %+v", len(res.Moved), res.Folders, res)
	}
	if !fileExists(filepath.Join(mods, "WickedWhims", "WickedWhims.package")) ||
		!fileExists(filepath.Join(mods, "WickedWhims", "WickedWhims.ts4script")) {
		t.Fatal("the pair is not together")
	}

	// The payoff: one row, one switch.
	all := scanAll(t, cfg)
	if len(all) != 1 {
		t.Fatalf("want one mod after tidying, got %+v", all)
	}
	if m := all["WickedWhims"]; len(m.Files) != 2 {
		t.Errorf("files = %v", m.Files)
	}
}

func TestTidyKeepsModulesOffAndPrefixes(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "tool.package.off"), "a")
	write(t, filepath.Join(mods, "!MCCC.package"), "b")
	write(t, filepath.Join(mods, "[Harrie] Kitchen.package"), "c")

	if _, err := NewModsService(cfg).Tidy(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "tool", "tool.package.off")) {
		t.Error("the switched-off file lost its suffix or its folder")
	}
	if !fileExists(filepath.Join(mods, "!MCCC", "!MCCC.package")) {
		t.Error("the priority prefix was not kept in the folder name")
	}
	if !fileExists(filepath.Join(mods, "[Harrie] Kitchen", "[Harrie] Kitchen.package")) {
		t.Error("the bracket prefix was not kept in the folder name")
	}

	all := scanAll(t, cfg)
	if m := all["tool"]; m.Enabled {
		t.Error("tidy should not switch a mod back on")
	}
}

func TestTidyMergesIntoAnExistingModFolder(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "WickedWhims", "WickedWhims.package"), "a")
	write(t, filepath.Join(mods, "WickedWhims.ts4script"), "b")

	if _, err := NewModsService(cfg).Tidy(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "WickedWhims", "WickedWhims.ts4script")) {
		t.Fatal("the loose script did not join the existing folder")
	}
	if m := scanAll(t, cfg)["WickedWhims"]; len(m.Files) != 2 {
		t.Errorf("files = %v", m.Files)
	}
}

func TestTidyLeavesCreatorFoldersAlone(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Harrie", "Cottage Garden", "garden.package"), "a")
	write(t, filepath.Join(mods, "Harrie.package"), "b")

	res, err := NewModsService(cfg).Tidy()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("want the file left alone, got %+v", res)
	}
	if !fileExists(filepath.Join(mods, "Harrie.package")) {
		t.Fatal("the file was moved into the creator folder")
	}
	if m := scanAll(t, cfg)["Harrie/Cottage Garden"]; m.Name != "Cottage Garden" {
		t.Error("the creator folder changed shape")
	}
}

func TestTidySkipsACollision(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "MCCC", "MCCC.package"), "keep")
	write(t, filepath.Join(mods, "MCCC.package"), "loose")

	res, err := NewModsService(cfg).Tidy()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("want a skip, got %+v", res)
	}
	if !fileExists(filepath.Join(mods, "MCCC.package")) {
		t.Fatal("the loose file disappeared")
	}
}

func TestTidyIgnoresEverythingElseAndIsRepeatable(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "readme.txt"), "a")
	write(t, filepath.Join(mods, "Resource.cfg"), "b")
	write(t, filepath.Join(mods, ".hidden.package"), "c")
	write(t, filepath.Join(mods, "MCCC.package"), "d")

	svc := NewModsService(cfg)
	first, err := svc.Tidy()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Moved) != 1 {
		t.Fatalf("want one move, got %+v", first)
	}
	if !fileExists(filepath.Join(mods, "readme.txt")) || !fileExists(filepath.Join(mods, "Resource.cfg")) ||
		!fileExists(filepath.Join(mods, ".hidden.package")) {
		t.Fatal("tidy touched a file it should ignore")
	}

	second, err := svc.Tidy()
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Moved) != 0 || len(second.Skipped) != 0 {
		t.Fatalf("second run did something: %+v", second)
	}
}

func TestTidyNeedsAGameFolder(t *testing.T) {
	cfg, _ := newTestConfig(t)
	if _, err := NewModsService(cfg).Tidy(); err == nil {
		t.Fatal("want an error when no folder is set")
	}
}

func TestTidyOnMissingModsFolderIsQuiet(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	_ = cfg.SetGameRoot(game)

	res, err := NewModsService(cfg).Tidy()
	if err != nil {
		t.Fatalf("a missing Mods folder is not an error: %v", err)
	}
	if len(res.Moved) != 0 {
		t.Fatalf("moved something out of nothing: %+v", res)
	}
}

func TestTidyLeavesTheFolderShapeOfNamesWithSpaces(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Cottage Garden Extra.package"), "a")

	if _, err := NewModsService(cfg).Tidy(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(mods, "Cottage Garden Extra", "Cottage Garden Extra.package")) {
		t.Fatal("spaces in the name broke the folder")
	}
	if !strings.Contains(scanAll(t, cfg)["Cottage Garden Extra"].Name, "Cottage") {
		t.Error("the mod name changed")
	}
}
