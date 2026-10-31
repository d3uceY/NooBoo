package services

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nooboo/models"
)

// Conflict reasons, sharpest first.
const (
	conflictSameName     = "same name"
	conflictVersion      = "another version"
	conflictResource     = "shares resources"
	maxConflictPartners  = 8
	maxConflictGroupSize = 9
	maxResourceGroup     = 64
)

func reasonRank(reason string) int {
	switch reason {
	case conflictSameName:
		return 0
	case conflictVersion:
		return 1
	case conflictResource:
		return 2
	}
	return 3
}

// versionSuffixRe matches "Hair v1", "hair_v2" and "hair 1.2", but not "Lot 2".
var versionSuffixRe = regexp.MustCompile(`(?i)^(.*?)[\s_\-+](?:v(\d+(?:\.\d+)*)|(\d+\.\d+))$`)

func versionToken(name string) string {
	m := versionSuffixRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return ""
	}
	if m[2] != "" {
		return m[2]
	}
	return m[3]
}

func versionBase(name string) string {
	m := versionSuffixRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return ""
	}
	base := strings.ToLower(m[1])
	base = strings.NewReplacer("_", " ", "-", " ", "+", " ").Replace(base)
	return strings.Join(strings.Fields(base), " ")
}

func conflictKey(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

type conflictPair struct {
	a, b   string
	reason string
}

// findConflicts pairs enabled mods by name, version and shared resource keys.
func findConflicts(
	mods []models.Mod,
	keys map[string][]resourceKey,
	ignored map[string]bool,
) map[string][]models.ConflictRef {
	enabled := make([]models.Mod, 0, len(mods))
	for _, mod := range mods {
		if mod.Enabled {
			enabled = append(enabled, mod)
		}
	}

	pairs := map[string]conflictPair{}
	add := func(a, b, reason string) {
		if a == "" || a == b {
			return
		}
		key := conflictKey(a, b)
		if ignored[key] {
			return
		}
		if old, ok := pairs[key]; ok {
			if reasonRank(reason) < reasonRank(old.reason) {
				old.reason = reason
				pairs[key] = old
			}
			return
		}
		pairs[key] = conflictPair{a: a, b: b, reason: reason}
	}
	addGroup := func(ids []string, reason string) {
		if len(ids) > maxConflictGroupSize {
			ids = ids[:maxConflictGroupSize]
		}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				add(ids[i], ids[j], reason)
			}
		}
	}

	byName := map[string][]string{}
	byBase := map[string]map[string]string{}
	for _, mod := range enabled {
		name := strings.ToLower(strings.TrimSpace(mod.Name))
		if name != "" {
			byName[name] = append(byName[name], mod.ID)
		}
		if token := versionToken(mod.Name); token != "" {
			if base := versionBase(mod.Name); base != "" {
				if byBase[base] == nil {
					byBase[base] = map[string]string{}
				}
				if _, ok := byBase[base][token]; !ok {
					byBase[base][token] = mod.ID
				}
			}
		}
	}
	for _, ids := range byName {
		if len(ids) > 1 {
			addGroup(ids, conflictSameName)
		}
	}
	for _, tokens := range byBase {
		if len(tokens) < 2 {
			continue
		}
		ids := make([]string, 0, len(tokens))
		for _, id := range tokens {
			ids = append(ids, id)
		}
		addGroup(ids, conflictVersion)
	}

	byKey := map[resourceKey][]string{}
	for id, list := range keys {
		for _, k := range list {
			byKey[k] = append(byKey[k], id)
		}
	}
	for _, ids := range byKey {
		if len(ids) > 1 && len(ids) <= maxResourceGroup {
			addGroup(ids, conflictResource)
		}
	}

	refs := map[string][]models.ConflictRef{}
	for _, p := range pairs {
		refs[p.a] = append(refs[p.a], models.ConflictRef{ID: p.b, Reason: p.reason})
		refs[p.b] = append(refs[p.b], models.ConflictRef{ID: p.a, Reason: p.reason})
	}
	for id, list := range refs {
		sort.Slice(list, func(i, j int) bool {
			ri, rj := reasonRank(list[i].Reason), reasonRank(list[j].Reason)
			if ri != rj {
				return ri < rj
			}
			return list[i].ID < list[j].ID
		})
		if len(list) > maxConflictPartners {
			list = list[:maxConflictPartners]
		}
		refs[id] = list
	}
	return refs
}

func (m *ModsService) resourceKeys(mods []models.Mod) map[string][]resourceKey {
	out := map[string][]resourceKey{}
	for _, mod := range mods {
		if !mod.Enabled {
			continue
		}
		m.mu.Lock()
		ref, ok := m.mods[mod.ID]
		m.mu.Unlock()
		if !ok {
			continue
		}
		if ins := m.insightForPath(ref.path, ref.size, ref.newest); ins != nil && len(ins.Keys) > 0 {
			out[mod.ID] = ins.Keys
		}
	}
	return out
}

func (m *ModsService) ignoredPairs() map[string]bool {
	out := map[string]bool{}
	for _, k := range m.cfg.Get().IgnoredConflicts {
		out[k] = true
	}
	return out
}

// IgnoreConflict remembers one pair so a deliberate override stops warning.
func (m *ModsService) IgnoreConflict(a, b string) error {
	if a == "" || b == "" || a == b {
		return nil
	}
	key := conflictKey(a, b)
	cfg := m.cfg.Get()
	for _, k := range cfg.IgnoredConflicts {
		if k == key {
			return nil
		}
	}
	cfg.IgnoredConflicts = append(cfg.IgnoredConflicts, key)
	return m.cfg.save(cfg)
}

func (m *ModsService) attachHints(mods []models.Mod) {
	refs := findConflicts(mods, m.resourceKeys(mods), m.ignoredPairs())
	advisories := loadAdvisories(m.cfg)
	for i := range mods {
		mods[i].Conflicts = refs[mods[i].ID]
		if mods[i].Conflicts == nil {
			mods[i].Conflicts = []models.ConflictRef{}
		}
		mods[i].Advisory = matchAdvisory(mods[i], advisories)
	}
}

// modsDir is only here so a relative path can be resolved in one place.
func modsDir(dir, rel string) string {
	return filepath.Join(dir, filepath.FromSlash(rel))
}
