package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// RepoPref is one catalog toggle stored on the phone.
type RepoPref struct {
	ID      string `json:"id"`
	Host    string `json:"host"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Color   string `json:"color"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
}

// SeenFallback is an older-release notice the phone has already shown.
type SeenFallback struct {
	Title         string `json:"title"`
	DeviceRelease string `json:"device_release"`
	ChosenRelease string `json:"chosen_release"`
	Arch          string `json:"arch"`
}

// PublisherName is a display name for a scratchpad publisher.
// The id stays the publisher hex. Title is only what the phone shows.
type PublisherName struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Settings is the onboarding and repository page.
type Settings struct {
	Onboarded      bool            `json:"onboarded"`
	Repositories   []RepoPref      `json:"repositories"`
	SeenFallbacks  []SeenFallback  `json:"seen_fallbacks"`
	PublisherNames []PublisherName `json:"publisher_names,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		Repositories: []RepoPref{
			{ID: "oss", Host: "repo-oss.ftuna.eu", URL: "https://repo-oss.ftuna.eu/{version}/{arch}/", Enabled: true, Color: "green", Title: "FTuna OSS", Detail: "Official catalog. Open source."},
			{ID: "chum", Host: "repo.sailfishos.org", URL: "https://repo.sailfishos.org/obs/sailfishos:/chum/{version}_{arch}/", Enabled: true, Color: "green", Title: "Chum", Detail: "Upstream SailfishOS:Chum. Open source."},
			{ID: "openrepos", Host: "repo-openrepos.ftuna.eu", URL: "https://repo-openrepos.ftuna.eu/", Enabled: false, Color: "orange", Title: "OpenRepos", Detail: "User-published packages, may not be open source."},
		},
	}
}

func (a *App) Settings() (Settings, error) {
	a.mu.Lock()
	if a.haveSettings {
		s := a.settings
		a.mu.Unlock()
		return s, nil
	}
	a.mu.Unlock()
	s, err := a.readSettings()
	if err != nil {
		return Settings{}, err
	}
	a.mu.Lock()
	if !a.haveSettings {
		a.settings = s
		a.haveSettings = true
	} else {
		s = a.settings
	}
	a.mu.Unlock()
	return s, nil
}

func (a *App) readSettings() (Settings, error) {
	path := filepath.Join(a.configDir, "settings.json")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return normalizeSettings(DefaultSettings(), nil), nil
	}
	if err != nil {
		return Settings{}, err
	}
	var got Settings
	if err := json.Unmarshal(body, &got); err != nil {
		return Settings{}, err
	}
	next := mergeSettings(got)
	next.PublisherNames = cleanPublisherNames(got.PublisherNames)
	return normalizeSettings(next, got.SeenFallbacks), nil
}

// SaveSettings keeps the canonical repository copy and stores enabled and onboarded.
// A payload that omits seen_fallbacks leaves the remembered notices in place.
func (a *App) SaveSettings(raw []byte) (Settings, error) {
	var probe struct {
		Seen  json.RawMessage `json:"seen_fallbacks"`
		Names json.RawMessage `json:"publisher_names"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Settings{}, err
	}
	var got Settings
	if err := json.Unmarshal(raw, &got); err != nil {
		return Settings{}, err
	}
	next := mergeSettings(got)
	if probe.Seen == nil || probe.Names == nil {
		prev, err := a.Settings()
		if err != nil {
			return Settings{}, err
		}
		if probe.Seen == nil {
			next.SeenFallbacks = prev.SeenFallbacks
		} else {
			next.SeenFallbacks = got.SeenFallbacks
		}
		if probe.Names == nil {
			next.PublisherNames = prev.PublisherNames
		} else {
			next.PublisherNames = cleanPublisherNames(got.PublisherNames)
		}
	} else {
		next.SeenFallbacks = got.SeenFallbacks
		next.PublisherNames = cleanPublisherNames(got.PublisherNames)
	}
	next = normalizeSettings(next, next.SeenFallbacks)
	return a.writeSettings(next)
}

// RememberFallbacks records older-release notices so they are not shown again
// until a refresh resolves a different release.
func (a *App) RememberFallbacks(raw []byte) (Settings, error) {
	var notes []SeenFallback
	if err := json.Unmarshal(raw, &notes); err != nil {
		return Settings{}, err
	}
	current, err := a.Settings()
	if err != nil {
		return Settings{}, err
	}
	for _, note := range notes {
		if strings.TrimSpace(note.Title) == "" || note.ChosenRelease == "" {
			continue
		}
		if fallbackSeen(current.SeenFallbacks, note) {
			continue
		}
		current.SeenFallbacks = append(current.SeenFallbacks, note)
	}
	current = normalizeSettings(current, current.SeenFallbacks)
	return a.writeSettings(current)
}

func (a *App) writeSettings(next Settings) (Settings, error) {
	if err := os.MkdirAll(a.configDir, 0755); err != nil {
		return Settings{}, err
	}
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(a.configDir, ".settings-*.json")
	if err != nil {
		return Settings{}, err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return Settings{}, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return Settings{}, err
	}
	if err := os.Rename(name, filepath.Join(a.configDir, "settings.json")); err != nil {
		os.Remove(name)
		return Settings{}, err
	}
	a.mu.Lock()
	a.settings = next
	a.haveSettings = true
	a.mu.Unlock()
	return next, nil
}

func mergeSettings(got Settings) Settings {
	defaults := DefaultSettings()
	defaults.Onboarded = got.Onboarded
	if len(got.Repositories) == 0 {
		return defaults
	}
	baseByID := map[string]RepoPref{}
	for _, repo := range defaults.Repositories {
		baseByID[repo.ID] = repo
	}
	repos := make([]RepoPref, 0, len(got.Repositories)+len(defaults.Repositories))
	seen := map[string]bool{}
	for _, repo := range got.Repositories {
		if def, ok := baseByID[repo.ID]; ok {
			def.Enabled = repo.Enabled
			// A saved mirror URL is dropped so the upstream default stays.
			if !legacyChumMirror(repo.URL) {
				if raw, host, ok := normalizeRepoURL(repo.URL); ok {
					def.URL = raw
					def.Host = host
				}
			}
			if title := strings.TrimSpace(repo.Title); title != "" {
				def.Title = title
			}
			repos = append(repos, def)
			seen[repo.ID] = true
			continue
		}
		cleaned, ok := cleanCustom(repo)
		if !ok || seen[cleaned.ID] {
			continue
		}
		seen[cleaned.ID] = true
		repos = append(repos, cleaned)
	}
	for _, def := range defaults.Repositories {
		if !seen[def.ID] {
			repos = append(repos, def)
		}
	}
	defaults.Repositories = repos
	return defaults
}

func cleanCustom(repo RepoPref) (RepoPref, bool) {
	raw, host, ok := normalizeRepoURL(repo.URL)
	if !ok {
		return RepoPref{}, false
	}
	repo.URL = raw
	repo.Host = host
	if repo.ID == "" || builtinRepo(repo.ID) {
		repo.ID = customRepoID(raw, host)
	}
	repo.ID = sanitizeID(repo.ID)
	if repo.ID == "" || builtinRepo(repo.ID) {
		return RepoPref{}, false
	}
	if strings.TrimSpace(repo.Title) == "" {
		repo.Title = host
	}
	repo.Title = strings.TrimSpace(repo.Title)
	if repo.Color == "" {
		repo.Color = "green"
	}
	if repo.Detail == "" {
		repo.Detail = "Added on this phone."
	}
	return repo, true
}

// normalizeRepoURL keeps {version} and {arch} as written. url.URL would encode the braces.
func normalizeRepoURL(raw string) (urlStr, host string, ok bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "info.json.xz")
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if !strings.HasPrefix(raw, "https://") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "https://")
	if rest == "" || strings.ContainsAny(rest, " @") {
		return "", "", false
	}
	host, path, found := strings.Cut(rest, "/")
	if host == "" {
		return "", "", false
	}
	if !found {
		return "https://" + host + "/", host, true
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "https://" + host + "/", host, true
	}
	return "https://" + host + "/" + path + "/", host, true
}

func legacyChumMirror(raw string) bool {
	return strings.Contains(strings.TrimSpace(raw), "://repo-chum.ftuna.eu/")
}

func customRepoID(raw, host string) string {
	path := strings.TrimPrefix(raw, "https://"+host)
	path = strings.Trim(path, "/")
	token := host
	if path != "" {
		token += "-" + path
	}
	return "custom-" + sanitizeID(token)
}

func builtinRepo(id string) bool {
	return id == "oss" || id == "chum" || id == "openrepos"
}

func sanitizeID(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func cleanPublisherNames(in []PublisherName) []PublisherName {
	seen := map[string]int{}
	out := []PublisherName{}
	for _, name := range in {
		id := strings.TrimSpace(name.ID)
		title := strings.TrimSpace(name.Title)
		if title == "" || !publisherID(id) {
			continue
		}
		if i, ok := seen[id]; ok {
			out[i].Title = title
			continue
		}
		seen[id] = len(out)
		out = append(out, PublisherName{ID: id, Title: title})
	}
	return out
}

func publisherID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeSettings(s Settings, seen []SeenFallback) Settings {
	if seen == nil {
		seen = []SeenFallback{}
	}
	s.SeenFallbacks = seen
	if s.Repositories == nil {
		s.Repositories = []RepoPref{}
	}
	if s.PublisherNames == nil {
		s.PublisherNames = []PublisherName{}
	}
	return s
}

func fallbackSeen(seen []SeenFallback, note SeenFallback) bool {
	for _, got := range seen {
		if got.Title == note.Title && got.DeviceRelease == note.DeviceRelease && got.ChosenRelease == note.ChosenRelease && got.Arch == note.Arch {
			return true
		}
	}
	return false
}

func indexURL(pref RepoPref, ossIndex string) string {
	if pref.ID == "oss" && ossIndex != "" {
		return ossIndex
	}
	raw := strings.TrimRight(pref.URL, "/")
	if strings.HasSuffix(raw, "info.json.xz") {
		return raw
	}
	return raw + "/info.json.xz"
}
