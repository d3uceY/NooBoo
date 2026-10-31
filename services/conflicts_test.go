package services

import (
	"path/filepath"
	"strconv"
	"testing"

	"nooboo/models"
)

func conflictsOf(t *testing.T, cfg *ConfigService, id string) []models.ConflictRef {
	t.Helper()
	list, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range list {
		if mod.ID == id {
			return mod.Conflicts
		}
	}
	t.Fatalf("mod %q not in scan", id)
	return nil
}

func hasReason(refs []models.ConflictRef, reason string) bool {
	for _, ref := range refs {
		if ref.Reason == reason {
			return true
		}
	}
	return false
}

func TestConflictSameName(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "CreatorA", "Hair", "a.package"), "a")
	write(t, filepath.Join(mods, "CreatorB", "Hair", "b.package"), "bb")

	for _, id := range []string{"CreatorA/Hair", "CreatorB/Hair"} {
		refs := conflictsOf(t, cfg, id)
		if !hasReason(refs, conflictSameName) {
			t.Errorf("%s conflicts = %+v, want same name", id, refs)
		}
	}
}

func TestConflictVersionPair(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Hair v1", "a.package"), "a")
	write(t, filepath.Join(mods, "Hair v2", "b.package"), "bb")

	for _, id := range []string{"Hair v1", "Hair v2"} {
		if refs := conflictsOf(t, cfg, id); !hasReason(refs, conflictVersion) {
			t.Errorf("%s conflicts = %+v, want another version", id, refs)
		}
	}
}

func TestConflictResourceOverlap(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	shared := []testRes{{typ: 0x034AEECB, group: 7, inst: 99, data: []byte("x")}}
	writeDbpf(t, filepath.Join(mods, "One", "a.package"), shared)
	writeDbpf(t, filepath.Join(mods, "Two", "b.package"), shared)

	for _, id := range []string{"One", "Two"} {
		if refs := conflictsOf(t, cfg, id); !hasReason(refs, conflictResource) {
			t.Errorf("%s conflicts = %+v, want shared resources", id, refs)
		}
	}
}

func TestConflictIgnoredPair(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "CreatorA", "Hair", "a.package"), "a")
	write(t, filepath.Join(mods, "CreatorB", "Hair", "b.package"), "bb")

	svc := NewModsService(cfg)
	if err := svc.IgnoreConflict("CreatorA/Hair", "CreatorB/Hair"); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range list {
		if mod.Name != "Hair" {
			continue
		}
		if hasReason(mod.Conflicts, conflictSameName) {
			t.Fatalf("%s still reports %+v", mod.ID, mod.Conflicts)
		}
	}
}

func TestConflictPartnerCap(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	for i := 0; i < 10; i++ {
		creator := "Creator" + strconv.Itoa(i)
		write(t, filepath.Join(mods, creator, "Hair", "x.package"), "x"+creator)
	}
	list, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, mod := range list {
		if mod.Name == "Hair" && len(mod.Conflicts) > maxConflictPartners {
			t.Errorf("%s has %d partners", mod.ID, len(mod.Conflicts))
		}
	}
}

func TestVersionToken(t *testing.T) {
	cases := map[string]string{
		"Hair v1":        "1",
		"hair_v2":        "2",
		"hair 1.2":       "1.2",
		"UICheats_v1.36": "1.36",
		"Lot 2":          "",
		"MCCC":           "",
	}
	for name, want := range cases {
		if got := versionToken(name); got != want {
			t.Errorf("versionToken(%q) = %q, want %q", name, got, want)
		}
	}
}
