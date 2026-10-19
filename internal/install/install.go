// Package install adds a native rpm repository through SSU and installs
// the chosen packages through PackageKit.
package install

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

// Alias is the SSU repository name: ftuna- plus the URL, without the scheme,
// with every character outside [A-Za-z0-9._-] replaced by '-'.
func Alias(raw string) string {
	u, err := url.Parse(raw)
	name := raw
	if err == nil && u.Host != "" {
		name = u.Host + u.Path
	}
	name = strings.Trim(name, "/")
	var b strings.Builder
	b.WriteString("ftuna-")
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// Repo is one SSU repository.
type Repo struct {
	Alias string `json:"alias"`
	URL   string `json:"url"`
}

// Found is one package PackageKit reported.
type Found struct {
	ID      string
	Name    string
	Version string
	Arch    string
	Data    string
	Info    uint32
	Summary string
}

// Change is one package a simulated install would add, upgrade, or remove.
type Change struct {
	Action  string `json:"action"`
	Label   string `json:"label"`
	Summary string `json:"summary,omitempty"`
}

// Plan is the package ids to install and the system changes PackageKit reported.
type Plan struct {
	IDs     []string `json:"ids"`
	Changes []Change `json:"changes"`
}

// System is the D-Bus surface used to add a repo and install packages.
type System interface {
	ListRepos() ([]Repo, error)
	AddRepo(alias, url string) error
	Refresh() error
	Resolve(names []string) ([]Found, error)
	Installed(names []string) ([]Found, error)
	Simulate(ids []string) ([]Found, error)
	Install(ids []string) error
}

// RepoSwitch turns one SSU repository on or off. ListRepos returns enabled repositories.
type RepoSwitch interface {
	System
	EnableRepo(alias string) error
	DisableRepo(alias string) error
}

// SetRepoEnabled adds and enables one repository, or disables that one alias.
// Other repositories are left alone. An upstream Chum URL that is already
// served by another alias, including the Chum app's sailfishos-chum, is not
// added again and is not disabled.
func SetRepoEnabled(sys RepoSwitch, raw string, on bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("no repository url")
	}
	repos, err := sys.ListRepos()
	if err != nil {
		return err
	}
	own := Alias(raw)
	if !on {
		if chumChannel(raw) != "" {
			for _, repo := range repos {
				if repo.Alias == own {
					if err := sys.DisableRepo(own); err != nil {
						return err
					}
					break
				}
			}
			if foreignChum(repos, raw) {
				return fmt.Errorf("SailfishOS:Chum manages this repository")
			}
			return nil
		}
		return sys.DisableRepo(own)
	}
	if alias, found := resolveAlias(repos, raw); found {
		if foreignChum(repos, raw) && alias != own {
			return nil
		}
		return sys.EnableRepo(alias)
	}
	if err := sys.AddRepo(own, raw); err != nil {
		if err2 := sys.EnableRepo(own); err2 != nil {
			return err
		}
		return nil
	}
	return sys.EnableRepo(own)
}

// SameRepo reports whether two repository URLs name the same path, ignoring a trailing slash.
func SameRepo(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

// EnsureRepo calls addRepo when that repository is not already listed.
// A Chum URL already served by the Chum app is reused, so a second alias
// is not created. The URL passed to addRepo is the native repository URL unchanged.
func EnsureRepo(sys System, repoURL string) (alias string, added bool, err error) {
	repos, err := sys.ListRepos()
	if err != nil {
		return "", false, err
	}
	if alias, found := resolveAlias(repos, repoURL); found {
		return alias, false, nil
	}
	alias = Alias(repoURL)
	if err := sys.AddRepo(alias, repoURL); err != nil {
		return alias, false, err
	}
	return alias, true, nil
}

// resolveAlias finds an enabled repository for raw. A Chum repository the
// Chum app already added is used instead of FTuna's own alias.
func resolveAlias(repos []Repo, raw string) (string, bool) {
	own := Alias(raw)
	var ownAlias, sameAlias, channelAlias string
	same := false
	for _, repo := range repos {
		if repo.Alias == own {
			ownAlias = repo.Alias
		}
		if !same && SameRepo(repo.URL, raw) {
			sameAlias = repo.Alias
			same = true
		}
		if channelAlias == "" && repo.Alias != own && chumCovers(repo, raw) {
			channelAlias = repo.Alias
		}
	}
	if channelAlias != "" {
		return channelAlias, true
	}
	if same {
		return sameAlias, true
	}
	if ownAlias != "" {
		return ownAlias, true
	}
	return "", false
}

// RepoEnabled reports whether raw is already enabled. A Chum URL is enabled
// when the Chum app's alias serves that channel, even if the release differs.
func RepoEnabled(repos []Repo, raw string) bool {
	_, ok := resolveAlias(repos, raw)
	return ok
}

func foreignChum(repos []Repo, raw string) bool {
	own := Alias(raw)
	for _, repo := range repos {
		if repo.Alias != own && chumCovers(repo, raw) {
			return true
		}
	}
	return false
}

func chumCovers(repo Repo, raw string) bool {
	want := chumChannel(raw)
	got := chumChannel(repo.URL)
	if want == "" || got == "" {
		return false
	}
	if got == want {
		return true
	}
	// The Chum app keeps a single alias. A second channel would be a second repository.
	return repo.Alias == "sailfishos-chum" || repo.Alias == "sailfishos-chum-testing"
}

func chumChannel(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/") + "/"
	const testing = "https://repo.sailfishos.org/obs/sailfishos:/chum:/testing/"
	const stable = "https://repo.sailfishos.org/obs/sailfishos:/chum/"
	if strings.HasPrefix(raw, testing) {
		return "testing"
	}
	if strings.HasPrefix(raw, stable) {
		return "stable"
	}
	return ""
}

// PackageIDs resolves the checked RPM names to PackageKit package ids.
// The caller installs those ids with PackageKit::Daemon::installPackages.
//
// The id comes from the repository PackageKit can see. A catalog build that
// is missing from the cache is refreshed once. When that build is present it
// is installed. Otherwise the newest build in the repository is installed
// when it is newer than what is already on the phone.
func PackageIDs(sys System, repoURL string, chosen []catalog.RPM) ([]string, error) {
	if len(chosen) == 0 {
		return nil, fmt.Errorf("no packages selected")
	}
	alias, added, err := EnsureRepo(sys, repoURL)
	if err != nil {
		return nil, err
	}
	if added {
		if err := sys.Refresh(); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(chosen))
	seenName := map[string]bool{}
	for _, rpm := range chosen {
		if rpm.Name == "" || seenName[rpm.Name] {
			continue
		}
		seenName[rpm.Name] = true
		names = append(names, rpm.Name)
	}
	found, err := sys.Resolve(names)
	if err != nil {
		return nil, err
	}
	if !added && cacheBehind(found, chosen, alias) {
		if err := sys.Refresh(); err != nil {
			return nil, err
		}
		found, err = sys.Resolve(names)
		if err != nil {
			return nil, err
		}
	}
	ids, missing := selectIDs(found, chosen, alias)
	if missing.Name != "" {
		return nil, fmt.Errorf("%s is not in %s", missing.Name, alias)
	}
	if len(ids) == 0 {
		if msg := absentBuilds(found, chosen, alias); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("The packages are already all installed")
	}
	return ids, nil
}

// cacheBehind reports whether Resolve is missing a build the caller asked for.
// A name with no version only needs some build in the repository.
func cacheBehind(found []Found, chosen []catalog.RPM, alias string) bool {
	for _, rpm := range chosen {
		if rpm.Version == "" && rpm.Release == "" {
			if newest(found, rpm.Name, alias, true) == nil {
				return true
			}
			continue
		}
		if _, ok := pick(found, rpm, alias); !ok {
			return true
		}
	}
	return false
}

func inRepo(row Found, alias string) bool {
	return strings.Contains(row.Data, alias) || strings.Contains(row.ID, alias)
}

func selectIDs(found []Found, chosen []catalog.RPM, alias string) (ids []string, missing catalog.RPM) {
	seen := map[string]bool{}
	for _, rpm := range chosen {
		if rpm.Name == "" || seen[rpm.Name] {
			continue
		}
		seen[rpm.Name] = true
		id, skip, ok := choose(found, rpm, alias)
		if !ok {
			return nil, rpm
		}
		if !skip {
			ids = append(ids, id)
		}
	}
	return ids, catalog.RPM{}
}

// choose returns the package id to install. skip means the phone already has
// a build at least as new as the repository. ok is false when the repository
// has no build of this name.
func choose(found []Found, rpm catalog.RPM, alias string) (id string, skip bool, ok bool) {
	if rpm.Version != "" || rpm.Release != "" {
		if id, ok = pick(found, rpm, alias); ok {
			return id, false, true
		}
	}
	repo := newest(found, rpm.Name, alias, true)
	if repo == nil {
		return "", false, false
	}
	inst := newest(found, rpm.Name, alias, false)
	if inst != nil && catalog.RPMVerCmp(stripEpoch(repo.Version), stripEpoch(inst.Version)) <= 0 {
		return "", true, true
	}
	return repo.ID, false, true
}

func newest(found []Found, name, alias string, repo bool) *Found {
	var best *Found
	for i := range found {
		row := &found[i]
		if row.Name != name || inRepo(*row, alias) != repo {
			continue
		}
		if best == nil || catalog.RPMVerCmp(stripEpoch(row.Version), stripEpoch(best.Version)) > 0 {
			best = row
		}
	}
	return best
}

func absentBuilds(found []Found, chosen []catalog.RPM, alias string) string {
	var parts []string
	for _, rpm := range chosen {
		if rpm.Version == "" && rpm.Release == "" {
			continue
		}
		if _, ok := pick(found, rpm, alias); ok {
			continue
		}
		have := "no build of it"
		if repo := newest(found, rpm.Name, alias, true); repo != nil {
			have = stripEpoch(repo.Version)
		}
		parts = append(parts, fmt.Sprintf("%s %s-%s is not in %s. The repository has %s.", rpm.Name, rpm.Version, rpm.Release, alias, have))
	}
	return strings.Join(parts, " ")
}

// Packages resolves the checked RPM names and installs those package ids.
func Packages(sys System, repoURL string, chosen []catalog.RPM) error {
	ids, err := PackageIDs(sys, repoURL, chosen)
	if err != nil {
		return err
	}
	return sys.Install(ids)
}

func pick(found []Found, rpm catalog.RPM, alias string) (string, bool) {
	for _, row := range found {
		if row.Name != rpm.Name || !inRepo(row, alias) || !versionMatches(row.Version, rpm.Version, rpm.Release) {
			continue
		}
		return row.ID, true
	}
	return "", false
}

func versionMatches(got, version, release string) bool {
	got = stripEpoch(got)
	return got == version+"-"+release || got == version
}

func stripEpoch(v string) string {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		return v[i+1:]
	}
	return v
}

// Changes lists the install, update, and remove lines from a simulation.
// Info values are PackageKit's PkInfoEnum codes.
func Changes(found []Found) []Change {
	var out []Change
	seen := map[string]bool{}
	for _, row := range found {
		action := simulateAction(row.Info)
		if action == "" || row.Name == "" {
			continue
		}
		label := packageLabel(row.Name, row.Version)
		key := action + "\t" + label
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Change{Action: action, Label: label, Summary: strings.TrimSpace(row.Summary)})
	}
	if out == nil {
		out = []Change{}
	}
	return out
}

// ChangesFromIDs is the fallback when a simulation returns no package lines.
func ChangesFromIDs(ids []string) []Change {
	var out []Change
	for _, id := range ids {
		row, ok := parsePackageID(id)
		if !ok {
			continue
		}
		out = append(out, Change{Action: "Install", Label: packageLabel(row.Name, row.Version)})
	}
	if out == nil {
		out = []Change{}
	}
	return out
}

func packageLabel(name, version string) string {
	version = stripEpoch(version)
	if version == "" {
		return name
	}
	return name + " " + version
}

func simulateAction(info uint32) string {
	switch info {
	case 3:
		return "Install"
	case 4, 9:
		return "Remove"
	case 5:
		return "Update"
	case 13:
		return "Reinstall"
	case 14:
		return "Downgrade"
	default:
		return ""
	}
}
