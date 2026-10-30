package services

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nooboo/models"
)

// releaseAPI is a var so tests can point it at a local server.
var releaseAPI = "https://api.github.com/repos/" + RepoOwner + "/" + RepoName + "/releases/latest"

// UpdateService asks GitHub for the newest stable release.
type UpdateService struct{}

func NewUpdateService() *UpdateService { return &UpdateService{} }

// Check compares the running version with the latest stable release.
// Failures are not errors the UI should show: no network means "no update".
func (u *UpdateService) Check() (models.UpdateInfo, error) {
	info := models.UpdateInfo{Current: Version}
	if Version == "dev" {
		return info, nil // a source build has nothing to compare against
	}

	req, err := http.NewRequest(http.MethodGet, releaseAPI, nil)
	if err != nil {
		return info, nil
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", RepoName+"/"+Version)

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return info, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info, nil
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		Draft       bool   `json:"draft"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return info, nil
	}
	// /releases/latest already excludes these, but a draft can slip through
	// on repos that publish manually.
	if payload.Prerelease || payload.Draft {
		return info, nil
	}

	info.Latest = payload.TagName
	info.URL = payload.HTMLURL
	info.PublishedAt = payload.PublishedAt
	info.Available = isNewer(Version, payload.TagName)
	return info, nil
}

// isNewer reports whether latest is a stable version above current.
// Anything with a pre-release suffix, and anything unparseable, never counts.
func isNewer(current, latest string) bool {
	c, okC := parseVersion(current)
	l, okL := parseVersion(latest)
	if !okC || !okL {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseVersion accepts 1.2.3 and v1.2.3, rejects 1.2.3-beta and 1.2.3+build.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if v == "" || strings.ContainsAny(v, "-+") {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
