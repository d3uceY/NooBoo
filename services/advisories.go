package services

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"nooboo/models"
)

//go:embed advisories.json
var embeddedAdvisories []byte

type advisoryEntry struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Since      string   `json:"since"`
	Note       string   `json:"note"`
	URL        string   `json:"url"`
	Identities []string `json:"identities"`
	Versions   []string `json:"versions"`
}

type advisoryFile struct {
	Version int                        `json:"version"`
	Games   map[string][]advisoryEntry `json:"games"`
}

// loadAdvisories reads the shipped list, or a local one when present.
func loadAdvisories(cfg *ConfigService) []advisoryEntry {
	raw := embeddedAdvisories
	if b, err := os.ReadFile(filepath.Join(cfg.AppData(), "advisories.json")); err == nil {
		raw = b
	}
	var file advisoryFile
	if json.Unmarshal(raw, &file) != nil {
		return nil
	}

	out := make([]advisoryEntry, 0, len(file.Games["sims4"]))
	for _, e := range file.Games["sims4"] {
		if e.Title == "" || advisoryRank(e.Status) == 99 {
			continue
		}
		ids := make([]string, 0, len(e.Identities))
		for _, id := range e.Identities {
			if id != "" {
				ids = append(ids, modIdentity(id))
			}
		}
		if len(ids) == 0 {
			continue
		}
		e.Identities = ids
		if !strings.HasPrefix(e.URL, "https://") {
			e.URL = ""
		}
		out = append(out, e)
	}
	return out
}

func advisoryRank(status string) int {
	switch status {
	case "broken":
		return 0
	case "outdated":
		return 1
	case "caution":
		return 2
	}
	return 99
}

// modIdentity folds a file or folder name the way the advisory list writes it.
func modIdentity(name string) string {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if m := versionSuffixRe.FindStringSubmatch(stem); m != nil {
		stem = m[1]
	}
	stem = strings.NewReplacer("_", " ", "-", " ", "+", " ", ".", " ").Replace(stem)
	return strings.Join(strings.Fields(stem), " ") + ext
}

func modIdentities(mod models.Mod) map[string]bool {
	out := map[string]bool{}
	if mod.Name != "" {
		out[modIdentity(mod.Name)] = true
	}
	for _, f := range mod.Files {
		out[modIdentity(f)] = true
	}
	return out
}

func matchAdvisory(mod models.Mod, entries []advisoryEntry) *models.AdvisoryRef {
	if len(entries) == 0 {
		return nil
	}
	ids := modIdentities(mod)
	tokens := map[string]bool{}
	if token := versionToken(mod.Name); token != "" {
		tokens[token] = true
	}
	for _, f := range mod.Files {
		if token := versionToken(f); token != "" {
			tokens[token] = true
		}
	}
	var best *models.AdvisoryRef
	bestRank := 99
	for _, e := range entries {
		if len(e.Versions) > 0 {
			found := false
			for _, v := range e.Versions {
				if tokens[v] {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		matched := false
		for _, id := range e.Identities {
			if ids[id] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if rank := advisoryRank(e.Status); rank < bestRank {
			bestRank = rank
			best = &models.AdvisoryRef{
				Title:  e.Title,
				Status: e.Status,
				Since:  e.Since,
				Note:   e.Note,
				URL:    e.URL,
			}
		}
	}
	return best
}
