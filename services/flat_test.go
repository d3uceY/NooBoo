package services

import (
	"path/filepath"
	"testing"

	"nooboo/models"
)

// TestScanFlatModsFolder pins down what a user gets when they drop every file
// straight into Mods with no folders at all.
func TestScanFlatModsFolder(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	mods := filepath.Join(game, "Mods")

	write(t, filepath.Join(mods, "MCCC.package"), "a")
	write(t, filepath.Join(mods, "WickedWhims.ts4script"), "b")
	write(t, filepath.Join(mods, "[Harrie] Cottage Garden.package"), "c")
	write(t, filepath.Join(mods, "tool.package.off"), "d")
	write(t, filepath.Join(mods, "Resource.cfg"), "e")
	write(t, filepath.Join(mods, "readme.txt"), "f")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	if g := cfg.SetGameRoot(game); !g.Valid {
		t.Fatalf("folder rejected: %q", g.Problem)
	}

	got, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}

	// One row per file, and nothing for Resource.cfg or a stray readme.
	if len(got) != 4 {
		t.Fatalf("want 4 rows, got %d: %+v", len(got), got)
	}

	byID := map[string]models.Mod{}
	for _, m := range got {
		byID[m.ID] = m
	}

	for _, id := range []string{"MCCC.package", "WickedWhims.ts4script", "[Harrie] Cottage Garden.package"} {
		m, ok := byID[id]
		if !ok {
			t.Fatalf("missing row for %q, got %+v", id, byID)
		}
		if !m.Enabled {
			t.Errorf("%q should read as enabled", id)
		}
		// A loose file has no creator to learn, so it always lands in Mods.
		if m.Creator != "" || m.Kind != models.KindMod {
			t.Errorf("%q = creator %q kind %q, want empty creator and mod kind", id, m.Creator, m.Kind)
		}
	}

	if n := byID["MCCC.package"].Name; n != "MCCC" {
		t.Errorf("name = %q, want MCCC", n)
	}
	if n := byID["[Harrie] Cottage Garden.package"].Name; n != "[Harrie] Cottage Garden" {
		t.Errorf("name = %q", n)
	}

	off, ok := byID["tool.package.off"]
	if !ok {
		t.Fatalf("missing the switched-off file, got %+v", byID)
	}
	if off.Enabled {
		t.Error("a .package.off file should read as off")
	}
	if off.Name != "tool" {
		t.Errorf("name = %q, want tool", off.Name)
	}
}

// TestFlatFilesToggleIndependently records the cost of a flat folder: the two
// halves of one script mod are separate rows with separate switches.
func TestFlatFilesToggleIndependently(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	mods := filepath.Join(game, "Mods")

	write(t, filepath.Join(mods, "WickedWhims.package"), "a")
	write(t, filepath.Join(mods, "WickedWhims.ts4script"), "b")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	_ = cfg.SetGameRoot(game)
	svc := NewModsService(cfg)

	if err := svc.SetEnabled([]string{"WickedWhims.package"}, false); err != nil {
		t.Fatal(err)
	}

	again, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var packageOff, scriptOn bool
	for _, m := range again {
		switch m.ID {
		case "WickedWhims.package.off":
			packageOff = !m.Enabled
		case "WickedWhims.ts4script":
			scriptOn = m.Enabled
		}
	}
	if !packageOff || !scriptOn {
		t.Fatalf("want one file off and the other still on, got %+v", again)
	}
}
