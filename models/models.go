package models

// Kind separates plain mods from custom content so installs land in the right place.
type Kind string

const (
	KindMod Kind = "mod"
	KindCC  Kind = "cc"
)

// Config is the whole on-disk settings file. Missing file means defaults.
type Config struct {
	GameRoot string `json:"gameRoot"`
	KeepLast int    `json:"keepLast"`
}

// GameFolder is the result of validating a folder the user picked.
type GameFolder struct {
	Root    string `json:"root"`
	Valid   bool   `json:"valid"`
	Saves   int    `json:"saves"`
	Tray    int    `json:"tray"`
	Mods    int    `json:"mods"`
	Problem string `json:"problem"`
}

// Mod is one installable unit: a folder under Mods, or a loose file.
type Mod struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Creator  string   `json:"creator"`
	Kind     Kind     `json:"kind"`
	Enabled  bool     `json:"enabled"`
	Path     string   `json:"path"`
	Size     int64    `json:"size"`
	Modified string   `json:"modified"`
	Files    []string `json:"files"`
}

// Backup is one snapshot of saves + Tray.
type Backup struct {
	ID      string `json:"id"`
	Created string `json:"created"`
	Files   int    `json:"files"`
	Size    int64  `json:"size"`
}

// Storage is what the app's own folders cost on disk. Partial counts
// half-written snapshots left by a crash, which are never listed.
type Storage struct {
	Snapshots      int   `json:"snapshots"`
	SnapshotSize   int64 `json:"snapshotSize"`
	Partial        int   `json:"partial"`
	Quarantine     int   `json:"quarantine"`
	QuarantineSize int64 `json:"quarantineSize"`
}

// AppInfo is the build's version and where to point the user for releases.
type AppInfo struct {
	Version string `json:"version"`
	Repo    string `json:"repo"`
}

// UpdateInfo is the result of asking GitHub for the newest stable release.
type UpdateInfo struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"`
}

// Progress is emitted on the "progress" event while long file work runs.
type Progress struct {
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Label string `json:"label"`
}

// Exception is the last script error the game wrote out, with the mods the
// traceback points at already pulled out.
type Exception struct {
	Found    bool      `json:"found"`
	File     string    `json:"file"`
	When     string    `json:"when"`
	Build    string    `json:"build"`
	Message  string    `json:"message"`
	Trace    []string  `json:"trace"`
	Suspects []Suspect `json:"suspects"`
}

// Suspect is one mod a traceback line points at. ID stays empty when the path
// belongs to something no longer installed.
type Suspect struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Why   string `json:"why"`
}

// TidyMove is one loose file that moved into its own folder.
type TidyMove struct {
	File string `json:"file"`
	Into string `json:"into"`
}

// TidyResult is what the tidy pass did, so the screen can say it out loud.
type TidyResult struct {
	Moved   []TidyMove `json:"moved"`
	Folders int        `json:"folders"`
	Skipped []string   `json:"skipped"`
}

// InstallRequest describes a file the user dropped in.
type InstallRequest struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Creator string `json:"creator"`
	Kind    Kind   `json:"kind"`
}
