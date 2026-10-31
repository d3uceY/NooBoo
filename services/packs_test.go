package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackKindFromCode(t *testing.T) {
	cases := map[string]string{
		"EP01": "expansion",
		"GP04": "game pack",
		"FP01": "free",
		"SP18": "stuff pack",
		"SP46": "stuff pack",
		"SP24": "kit",
		"SP82": "kit",
	}
	for code, want := range cases {
		if got := packKind(code); got != want {
			t.Errorf("packKind(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestPacksReadSkipLine(t *testing.T) {
	ini := "[usersetting]\npackstoskipmount = EP01, gp04 ,, SP24\n"
	got := readPacksToSkip(ini)
	for _, code := range []string{"EP01", "GP04", "SP24"} {
		if !got[code] {
			t.Errorf("%s missing from %v", code, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("parsed %d codes: %v", len(got), got)
	}
	if len(readPacksToSkip("nothing here")) != 0 {
		t.Error("a file without the line produced codes")
	}
}

func TestPacksWritePreservesRest(t *testing.T) {
	ini := "[usersetting]\r\nfoo = 1\r\npackstoskipmount = EP01\r\n[other]\r\nbar = 2\r\n"
	next := applyPackSkip(ini, "GP04", false)
	if !strings.Contains(next, "packstoskipmount = EP01,GP04\r\n") {
		t.Fatalf("line = %q", next)
	}
	if !strings.Contains(next, "[other]\r\nbar = 2\r\n") {
		t.Fatal("other sections were rewritten")
	}
	if !strings.Contains(next, "foo = 1\r\n") {
		t.Fatal("other keys were rewritten")
	}

	next = applyPackSkip(next, "EP01", true)
	next = applyPackSkip(next, "GP04", true)
	if !strings.Contains(next, "packstoskipmount = \r\n") {
		t.Fatalf("empty value not written: %q", next)
	}
}

func TestPacksAddsUserSettingSection(t *testing.T) {
	next := applyPackSkip("foo = 1\n", "EP01", false)
	if !strings.HasSuffix(next, "[usersetting]\npackstoskipmount = EP01\n") {
		t.Fatalf("got %q", next)
	}
	if strings.Count(next, "[usersetting]") != 1 {
		t.Fatal("section inserted more than once")
	}
}

func packFixture(t *testing.T) (*ConfigService, string) {
	t.Helper()
	cfg, base := newTestConfig(t)
	game := makeGame(t, filepath.Join(base, "The Sims 4"))
	cfg.SetGameRoot(game)

	install := filepath.Join(base, "install")
	for _, d := range []string{"Game", "Data"} {
		if err := os.MkdirAll(filepath.Join(install, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"EP01", "GP04", "SP24", "EP99", "readme"} {
		if err := os.MkdirAll(filepath.Join(install, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c := cfg.Get()
	c.InstallRoot = install
	if err := cfg.save(c); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(game, "UserSetting.ini"), "[usersetting]\npackstoskipmount = GP04\n")
	return cfg, game
}

func TestPacksListFromInstallRoot(t *testing.T) {
	cfg, _ := packFixture(t)
	packs, err := NewPackService(cfg).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 4 {
		t.Fatalf("got %d packs: %+v", len(packs), packs)
	}
	byCode := map[string]bool{}
	names := map[string]string{}
	for _, p := range packs {
		byCode[p.Code] = p.Enabled
		names[p.Code] = p.Name
		if p.Code == "GP04" && p.Name != "Vampires" {
			t.Errorf("GP04 name = %q", p.Name)
		}
	}
	if names["EP99"] != "EP99" {
		t.Errorf("unknown code name = %q, want EP99", names["EP99"])
	}
	if byCode["GP04"] || !byCode["EP01"] || !byCode["SP24"] {
		t.Fatalf("states = %+v", byCode)
	}
}

func TestSetEnabledWritesSkipLine(t *testing.T) {
	cfg, game := packFixture(t)
	svc := NewPackService(cfg)
	if err := svc.SetEnabled("EP01", false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(game, "UserSetting.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "packstoskipmount = EP01,GP04") {
		t.Fatalf("ini = %q", body)
	}
	if err := svc.SetEnabled("EP01", true); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(game, "UserSetting.ini"))
	if !strings.Contains(string(body), "packstoskipmount = GP04") {
		t.Fatalf("ini = %q", body)
	}
}
