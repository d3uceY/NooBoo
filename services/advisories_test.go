package services

import (
	"path/filepath"
	"testing"
)

func TestModIdentity(t *testing.T) {
	cases := map[string]string{
		"MC_Command_Center.package":         "mc command center.package",
		"UICheatsExtension_v1.36.ts4script": "uicheatsextension.ts4script",
		"[Harrie] Cottage Garden.package":   "[harrie] cottage garden.package",
		"Cool-Hair_v2.package":              "cool hair.package",
	}
	for name, want := range cases {
		if got := modIdentity(name); got != want {
			t.Errorf("modIdentity(%q) = %q, want %q", name, got, want)
		}
	}
}

func writeAdvisories(t *testing.T, cfg *ConfigService, body string) {
	t.Helper()
	write(t, filepath.Join(cfg.AppData(), "advisories.json"), body)
}

func TestAdvisoryMatchesIdentity(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "MC_Command_Center.package"), "x")
	writeAdvisories(t, cfg, `{"version":1,"games":{"sims4":[{
		"id":"mccc","title":"MCCC is broken","status":"broken",
		"since":"the 1.117 patch","identities":["MC Command Center.package"]}]}}`)

	list, err := NewModsService(cfg).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Advisory == nil {
		t.Fatalf("advisory = %+v", list)
	}
	if list[0].Advisory.Title != "MCCC is broken" {
		t.Fatalf("title = %q", list[0].Advisory.Title)
	}
}

func TestAdvisoryVersionFilter(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "UICheatsExtension_v1.36.ts4script"), "x")
	writeAdvisories(t, cfg, `{"version":1,"games":{"sims4":[{
		"id":"ui","title":"UI Cheats","status":"outdated",
		"identities":["UICheatsExtension.ts4script"],"versions":["1.35"]}]}}`)

	svc := NewModsService(cfg)
	list, _ := svc.Scan()
	if len(list) != 1 || list[0].Advisory != nil {
		t.Fatalf("1.35 should not match 1.36: %+v", list)
	}

	writeAdvisories(t, cfg, `{"version":1,"games":{"sims4":[{
		"id":"ui","title":"UI Cheats","status":"outdated",
		"identities":["UICheatsExtension.ts4script"],"versions":["1.36"]}]}}`)
	list, _ = svc.Scan()
	if len(list) != 1 || list[0].Advisory == nil {
		t.Fatal("1.36 should match")
	}
}

func TestAdvisorySkipsUnknownStatus(t *testing.T) {
	cfg, _, mods := gameWithMods(t)
	write(t, filepath.Join(mods, "Thing.package"), "x")
	writeAdvisories(t, cfg, `{"version":1,"games":{"sims4":[{
		"id":"x","title":"Thing","status":"weird","identities":["Thing.package"]}]}}`)

	list, _ := NewModsService(cfg).Scan()
	if len(list) != 1 || list[0].Advisory != nil {
		t.Fatalf("unknown status matched: %+v", list)
	}
}
