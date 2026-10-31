package services

import (
	"path/filepath"
	"testing"
)

// A Steam Deck keeps the Windows user folder inside the Proton prefix.
func TestDetectFindsProtonPrefix(t *testing.T) {
	home := t.TempDir()
	game := filepath.Join(home, ".steam", "steam", "steamapps", "compatdata",
		"1222670", "pfx", "drive_c", "users", "steamuser", "Documents",
		"Electronic Arts", "The Sims 4")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Options.ini"), "[Options]")

	if got := gameRootUnder(home); got != game {
		t.Fatalf("got %q, want %q", got, game)
	}
}

// A library on another drive is only discoverable through libraryfolders.vdf.
func TestDetectReadsSteamLibraryFolders(t *testing.T) {
	home := t.TempDir()
	steam := filepath.Join(home, ".local", "share", "Steam")
	library := filepath.Join(home, "games", "SteamLibrary")
	vdf := `"libraryfolders" { "1" { "path" "` + library + `" } }`
	write(t, filepath.Join(steam, "config", "libraryfolders.vdf"), vdf)

	game := filepath.Join(library, "steamapps", "compatdata", "1222670", "pfx",
		"drive_c", "users", "steamuser", "Documents", "Electronic Arts", "The Sims 4")
	write(t, filepath.Join(game, "saves", "Slot.save"), "s")
	write(t, filepath.Join(game, "Tray", "item.bpi"), "t")

	if got := gameRootUnder(home); got != game {
		t.Fatalf("got %q, want %q", got, game)
	}
}

// A folder that merely exists is not a Sims 4 folder.
func TestDetectIgnoresUnrelatedFolders(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "Documents", "Electronic Arts", "The Sims 4", "notes.txt"), "x")

	if got := gameRootUnder(home); got != "" {
		t.Fatalf("got %q, want no match", got)
	}
}
