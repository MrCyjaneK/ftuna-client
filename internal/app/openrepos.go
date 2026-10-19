package app

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/cache"
	"github.com/mrcyjanek/ftuna/client/internal/catalog"
	"github.com/mrcyjanek/ftuna/client/internal/install"
)

// OpenReposUser is one publisher repository from the OpenRepos index.
type OpenReposUser struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// OpenReposUsers lists every publisher in the OpenRepos index.
// refresh downloads the index. Otherwise the cached index is used, and a missing cache is downloaded.
// Enabled is true when SSU already has that publisher repository turned on.
func (a *App) OpenReposUsers(refresh bool) ([]OpenReposUser, error) {
	pref, err := a.openReposPref()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(a.cacheDir, "catalogs", sanitizeID(pref.ID))
	var idx cache.Index
	if refresh {
		idx, err = cache.FetchIndex(a.client, a.indexFor(pref), dir, cache.ReadETag(dir), func(int) {})
	} else {
		idx, err = cache.ReadIndex(dir)
		if err != nil {
			idx, err = cache.FetchIndex(a.client, a.indexFor(pref), dir, cache.ReadETag(dir), func(int) {})
		}
	}
	if err != nil {
		return nil, err
	}
	raw, err := decodeXZ(idx.Body)
	if err != nil {
		return nil, err
	}
	doc, err := catalog.Parse(raw)
	if err != nil {
		return nil, err
	}
	enabled, err := a.SystemRepos()
	if err != nil {
		return nil, err
	}
	return openReposUsers(doc.Repositories, enabled), nil
}

func (a *App) openReposPref() (RepoPref, error) {
	prefs, err := a.Settings()
	if err != nil {
		return RepoPref{}, err
	}
	for _, pref := range prefs.Repositories {
		if pref.ID == "openrepos" {
			return pref, nil
		}
	}
	return RepoPref{}, fmt.Errorf("openrepos is not configured")
}

func openReposUsers(repos []catalog.Repo, enabled []install.Repo) []OpenReposUser {
	out := []OpenReposUser{}
	seen := map[string]bool{}
	for _, repo := range repos {
		title, raw, ok := openReposPublisher(repo.URL)
		if !ok {
			continue
		}
		key := strings.TrimRight(raw, "/")
		if seen[key] {
			continue
		}
		seen[key] = true
		id := repo.ID
		if id == "" {
			id = title
		}
		out = append(out, OpenReposUser{
			ID:      id,
			Title:   title,
			URL:     raw,
			Enabled: install.RepoEnabled(enabled, raw),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func openReposPublisher(raw string) (title, repoURL string, ok bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "repo-openrepos.ftuna.eu") {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], raw, true
}
