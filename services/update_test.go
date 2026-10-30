package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.1.0", "v0.1.1", true},
		{"v0.9.0", "v1.0.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.2.0", "v0.1.9", false},
		{"1.0.0", "1.0.1", true},
		{"v1.0.0", "1.0.1", true},
		{"dev", "v9.9.9", false},
		{"v0.1.0", "v0.2.0-beta", false},
		{"v0.1.0", "v0.2.0-alpha.1", false},
		{"v0.1.0", "v0.2.0+sha", false},
		{"v0.1.0", "nightly", false},
	}
	for _, c := range cases {
		if got := isNewer(c.current, c.latest); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

// serveRelease points the checker at a stub GitHub response.
func serveRelease(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(func() { srv.Close() })
	original := releaseAPI
	releaseAPI = srv.URL
	t.Cleanup(func() { releaseAPI = original })
}

func TestCheckFindsAStableUpdate(t *testing.T) {
	original := Version
	Version = "v0.1.0"
	t.Cleanup(func() { Version = original })

	serveRelease(t, http.StatusOK, `{"tag_name":"v0.2.0","html_url":"https://github.com/d3uceY/NooBoo/releases/tag/v0.2.0","published_at":"2026-11-01T10:00:00Z"}`)

	info, err := NewUpdateService().Check()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available {
		t.Fatalf("update not detected: %+v", info)
	}
	if info.Latest != "v0.2.0" || info.URL == "" {
		t.Fatalf("bad payload mapping: %+v", info)
	}
}

// A pre-release must never prompt, even if its version is higher.
func TestCheckIgnoresPreReleases(t *testing.T) {
	original := Version
	Version = "v0.1.0"
	t.Cleanup(func() { Version = original })

	serveRelease(t, http.StatusOK, `{"tag_name":"v0.2.0","prerelease":true,"html_url":"https://github.com/d3uceY/NooBoo/releases/tag/v0.2.0"}`)

	info, _ := NewUpdateService().Check()
	if info.Available {
		t.Fatalf("pre-release prompted for an update: %+v", info)
	}
}

func TestCheckTreatsOfflineAsNoUpdate(t *testing.T) {
	original := Version
	Version = "v0.1.0"
	t.Cleanup(func() { Version = original })

	serveRelease(t, http.StatusForbidden, `{"message":"rate limited"}`)

	info, err := NewUpdateService().Check()
	if err != nil {
		t.Fatalf("check should not fail the caller: %v", err)
	}
	if info.Available || info.Latest != "" {
		t.Fatalf("rate limit produced an update: %+v", info)
	}
}

// A source build has no version to compare, so it never checks.
func TestCheckSkipsDevBuilds(t *testing.T) {
	original := Version
	Version = "dev"
	t.Cleanup(func() { Version = original })

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { srv.Close() })
	originalAPI := releaseAPI
	releaseAPI = srv.URL
	t.Cleanup(func() { releaseAPI = originalAPI })

	info, _ := NewUpdateService().Check()
	if called {
		t.Fatal("a dev build called the release API")
	}
	if info.Available {
		t.Fatal("dev build reported an update")
	}
}

func TestCheckHandlesGarbageJSON(t *testing.T) {
	original := Version
	Version = "v0.1.0"
	t.Cleanup(func() { Version = original })

	serveRelease(t, http.StatusOK, `not json`)

	info, err := NewUpdateService().Check()
	if err != nil {
		t.Fatal(err)
	}
	if info.Available {
		t.Fatal("garbage produced an update")
	}
}
