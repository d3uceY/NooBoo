package services

import (
	"path/filepath"
	"strings"
	"testing"
)

// sampleLog is the shape the game actually writes: one escaped traceback blob.
const sampleLog = `<?xml version="1.0" ?><root>
<report><version>2</version><sessionid>7c1f</sessionid><type>desync</type>
<sku>ea.maxis.sims4_64.15.pc</sku>
<createtime>2026-10-29 23:00:00</createtime>
<buildsignature>Local.Unknown.Unknown.1.110.315.1030-1.1</buildsignature>
<categoryid>AttributeError</categoryid>
<desyncdata>Traceback (most recent call last):&#13;&#10;
  File "T:\InGame\Gameplay\Scripts\Server\interactions\base\interaction.py", line 5123, in _trigger_interaction_start_event&#13;&#10;
  File "C:\\Users\\me\\Documents\\Electronic Arts\\The Sims 4\\Mods\\MCCC\\mc_cmd_center.py", line 87, in on_interaction&#13;&#10;
  File "<string>", line 1, in <module>&#13;&#10;
AttributeError: 'NoneType' object has no attribute 'get_current_outfit'</desyncdata>
</report>
</root>`

func TestParseLogReadsTheBlob(t *testing.T) {
	got, frames := parseLog(sampleLog)

	if got.Build != "Local.Unknown.Unknown.1.110.315.1030-1.1" {
		t.Errorf("build = %q", got.Build)
	}
	if got.When != "2026-10-29T23:00:00Z" {
		t.Errorf("when = %q, want the log's own createtime", got.When)
	}
	if !strings.Contains(got.Message, "get_current_outfit") {
		t.Errorf("message = %q", got.Message)
	}
	if len(frames) != 5 {
		t.Fatalf("want 5 lines, got %d: %v", len(frames), got.Trace)
	}
	if !strings.HasPrefix(got.Trace[1], `File "Server\`) {
		t.Errorf("game frame not shortened: %q", got.Trace[1])
	}
}

func TestFindSuspects(t *testing.T) {
	root := filepath.Join(`C:\Users\me\Documents\Electronic Arts\The Sims 4`, "Mods")
	_, frames := parseLog(sampleLog)
	suspects := findSuspects(frames, root)

	if len(suspects) != 1 {
		t.Fatalf("want 1 suspect, got %d: %+v", len(suspects), suspects)
	}
	if suspects[0].ID != "MCCC" {
		t.Errorf("id = %q, want MCCC", suspects[0].ID)
	}
	if suspects[0].Line != 87 {
		t.Errorf("line = %d, want 87", suspects[0].Line)
	}
}

func TestFindSuspectsIgnoresGameAndGeneratedCode(t *testing.T) {
	frames := []frame{
		toFrame(`File "T:\InGame\Gameplay\Scripts\Server\objects\component.py", line 20, in _start`),
		toFrame(`File "<string>", line 1, in <module>`),
		toFrame(`File "C:\Program Files\EA Games\The Sims 4\Game\Bin\Python\python27.zip\abc.py", line 9, in run`),
	}
	if got := findSuspects(frames, ""); len(got) != 0 {
		t.Fatalf("want no suspects, got %+v", got)
	}
}

func TestFindSuspectsFlagsUnknownScript(t *testing.T) {
	frames := []frame{toFrame(`File "E:\Downloads\some_mod\loose_script.py", line 4, in patch`)}
	got := findSuspects(frames, `C:\Games\Mods`)
	if len(got) != 1 || got[0].ID != "" || got[0].Label != "loose_script.py" {
		t.Fatalf("want one mention by file name, got %+v", got)
	}
}

func TestFindSuspectsHandlesDoubledSlashes(t *testing.T) {
	root := `C:\Users\me\Documents\The Sims 4\Mods`
	frames := []frame{toFrame(`File "C:\\Users\\me\\Documents\\The Sims 4\\Mods\\MCCC\\mccc.py", line 12, in go`)}
	got := findSuspects(frames, root)
	if len(got) != 1 || got[0].ID != "MCCC" {
		t.Fatalf("want MCCC, got %+v", got)
	}
}

func TestLatestPrefersNewestLog(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")

	// A game folder needs two markers before it is accepted.
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	write(t, filepath.Join(game, "lastException.txt"), sampleLog)
	write(t, filepath.Join(game, "Mods", "MCCC", "MCCC.package"), "m")
	if g := cfg.SetGameRoot(game); !g.Valid {
		t.Fatalf("folder rejected: %q", g.Problem)
	}

	ex, err := NewExceptionService(cfg).Latest()
	if err != nil {
		t.Fatal(err)
	}
	if !ex.Found || ex.When == "" {
		t.Fatalf("expected a found log with a time, got %+v", ex)
	}
	if len(ex.Suspects) != 1 || ex.Suspects[0].ID != "MCCC" {
		t.Fatalf("suspects = %+v", ex.Suspects)
	}
}

func TestLatestWithoutLogIsEmpty(t *testing.T) {
	cfg, base := newTestConfig(t)
	game := filepath.Join(base, "The Sims 4")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")
	_ = cfg.SetGameRoot(game)

	ex, err := NewExceptionService(cfg).Latest()
	if err != nil {
		t.Fatal(err)
	}
	if ex.Found || len(ex.Trace) != 0 || len(ex.Suspects) != 0 {
		t.Fatalf("a clean game should report nothing, got %+v", ex)
	}
}

func TestPlainCrashDumpStillReads(t *testing.T) {
	ex, _ := parseLog("boom\r\nTraceback (most recent call last):\r\nValueError: bad tuning\n")
	if !strings.Contains(ex.Message, "bad tuning") {
		t.Fatalf("message = %q", ex.Message)
	}
}

// realLog is trimmed from a real file: a BOM, two reports, an unclosed
// categoryid holding a desync:// URL, and a j[mal|g] summary line.
const realLog = "\ufeff<?xml version=\"1.0\" ?><root>\n" +
	"<report><version>2</version><sessionid>280e24f061adc16d6744b83d</sessionid><type>desync</type>" +
	"<sku>ea.maxis.sims4_64.15.macarm64</sku><createtime>2024-11-26 01:47:53</createtime>" +
	"<buildsignature>Local.Unknown.Unknown.1.110.311.1220-1.300.199.Release</buildsignature>" +
	"<categoryid>desync://280e24f061adc16d6744b83d/</desync://stentsysconfig/</xsheet>" +
	"<desyncdata>j[mal|g] Error invoking TunableModelWrapper. (AttributeError: 'NoneType' object has no attribute 'get_model')&#13;&#10;&#13;&#10;" +
	"Traceback (most recent call last):&#13;&#10;" +
	"  File \"T:\\InGame\\Gameplay\\Scripts\\Core\\sims4\\core_services.py\", line 184, in start_service_tick&#13;&#10;" +
	"  File \"T:\\InGame\\Gameplay\\Scripts\\Core\\sims4\\tuning\\instance_manager.py\", line 567, in load_data_into_class_instances&#13;&#10;" +
	"  File \"T:\\InGame\\Gameplay\\Scripts\\Server\\tunable_utils\\tunable_model.py\", line 71, in get_model&#13;&#10;" +
	"AttributeError: 'NoneType' object has no attribute 'get_model'</desyncdata></report>" +
	"<report><version>2</version><sessionid>280e24f061adc16d6744b83d</sessionid><type>desync</type>" +
	"<sku>ea.maxis.sims4_64.15.macarm64</sku><createtime>2024-11-26 01:47:54</createtime>" +
	"<buildsignature>Local.Unknown.Unknown.1.110.311.1220-1.300.199.Release</buildsignature>" +
	"<categoryid>desync://280e24f061adc16d6744b83d/</desync://stentsysconfig/</xsheet>" +
	"<desyncdata>j[mal|g] Error while parsing Tuning in Instance 99016419052317039314 (Smoink Object Food Tarts, Type: SObject). (AttributeError: 'NoneType' object has no attribute 'get_model')&#13;&#10;&#13;&#10;" +
	"Traceback (most recent call last):&#13;&#10;" +
	"  File \"T:\\InGame\\Gameplay\\Scripts\\Core\\sims4\\tuning\\serialization.py\", line 802, in load_tunable&#13;&#10;" +
	"  File \"T:\\InGame\\Gameplay\\Scripts\\Core\\sims4\\tuning\\tunable.py\", line 1205, in load_etree_node&#13;&#10;" +
	"AttributeError: 'NoneType' object has no attribute 'get_model'</desyncdata></report>\n</root>"

func TestRealLogUsesNewestReport(t *testing.T) {
	ex, _ := parseLog(realLog)

	if ex.When != "2024-11-26T01:47:54Z" {
		t.Errorf("when = %q, want the second report", ex.When)
	}
	if !strings.Contains(ex.Message, "Error while parsing Tuning") {
		t.Errorf("message = %q, want the engine's own summary", ex.Message)
	}
	if strings.Contains(ex.Message, "Traceback") {
		t.Errorf("message picked the traceback header: %q", ex.Message)
	}
	if !strings.Contains(ex.Build, "1.110.311.1220") {
		t.Errorf("build = %q", ex.Build)
	}
}

func TestRealLogBlamesNoMod(t *testing.T) {
	_, frames := parseLog(realLog)
	if got := findSuspects(frames, `C:\Users\me\Documents\Electronic Arts\The Sims 4\Mods`); len(got) != 0 {
		t.Fatalf("an all-game stack should blame nothing, got %+v", got)
	}
	if len(frames) != 5 {
		t.Fatalf("want 5 lines across the newest report, got %d", len(frames))
	}
}
