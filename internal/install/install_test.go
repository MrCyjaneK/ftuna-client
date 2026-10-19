package install

import (
	"strings"
	"testing"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

func TestAlias(t *testing.T) {
	got := Alias("https://repo-oss.ftuna.eu/5.2.0.17/aarch64/")
	want := "ftuna-repo-oss.ftuna.eu-5.2.0.17-aarch64"
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestEnsureRepoAddsOnce(t *testing.T) {
	raw := "https://repo-oss.ftuna.eu/5.2.0.17/aarch64/"
	fake := &fakeSys{}
	alias, added, err := EnsureRepo(fake, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !added || alias != "ftuna-repo-oss.ftuna.eu-5.2.0.17-aarch64" || len(fake.added) != 1 || fake.added[0].URL != raw {
		t.Fatalf("%s added %v %+v", alias, added, fake.added)
	}
	if _, added, err := EnsureRepo(fake, raw); err != nil || added {
		t.Fatal(err, added)
	}
	if len(fake.added) != 1 {
		t.Fatalf("added again: %+v", fake.added)
	}
}

func TestPickPrefersRepoOverNewerInstalled(t *testing.T) {
	alias := "ftuna-repo-oss.ftuna.eu-5.1.0.11-aarch64"
	found := []Found{
		{ID: "harbour-advanced-camera;0.9.8-1.4.1.bso;aarch64;installed", Name: "harbour-advanced-camera", Version: "0.9.8-1.4.1.bso", Data: "installed"},
		{ID: "harbour-advanced-camera;0:0.9.5-1;aarch64;" + alias, Name: "harbour-advanced-camera", Version: "0:0.9.5-1", Data: alias},
	}
	id, ok := pick(found, catalog.RPM{Name: "harbour-advanced-camera", Version: "0.9.5", Release: "1"}, alias)
	if !ok || id != found[1].ID {
		t.Fatalf("ok %v id %s", ok, id)
	}
}

func TestPackagesInstallsResolvedID(t *testing.T) {
	repo := "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"
	alias := Alias(repo)
	fake := &fakeSys{resolved: []Found{{
		ID:      "harbour-advanced-camera;0.9.5-1;aarch64;" + alias,
		Name:    "harbour-advanced-camera",
		Version: "0.9.5-1",
		Data:    alias,
	}}}
	err := Packages(fake, repo, []catalog.RPM{{
		Name: "harbour-advanced-camera", Version: "0.9.5", Release: "1", Arch: "aarch64", Install: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.installed) != 1 || fake.installed[0] != fake.resolved[0].ID {
		t.Fatalf("%v", fake.installed)
	}
	if !fake.refreshed {
		t.Fatal("cache was not refreshed")
	}
}

func TestUnversionedSkipsNewerInstalled(t *testing.T) {
	repo := "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"
	alias := Alias(repo)
	fake := &fakeSys{added: []Repo{{Alias: alias, URL: repo}}}
	fake.resolves = [][]Found{
		{{ID: "sailfish-components-gallery-qt5;1.3.2-1.6.1.jolla;aarch64;installed", Name: "sailfish-components-gallery-qt5", Version: "1.3.2-1.6.1.jolla", Data: "installed"}},
		{
			{ID: "sailfish-components-gallery-qt5;1.3.2-1.6.1.jolla;aarch64;installed", Name: "sailfish-components-gallery-qt5", Version: "1.3.2-1.6.1.jolla", Data: "installed"},
			{ID: "sailfish-components-gallery-qt5;1.3.0-1;aarch64;" + alias, Name: "sailfish-components-gallery-qt5", Version: "1.3.0-1", Data: alias},
		},
	}
	_, err := PackageIDs(fake, repo, []catalog.RPM{{Name: "sailfish-components-gallery-qt5", Arch: "aarch64", Install: true}})
	if err == nil || !strings.Contains(err.Error(), "already") || !strings.Contains(err.Error(), "installed") {
		t.Fatal(err)
	}
	if !fake.refreshed {
		t.Fatal("cache was not refreshed")
	}
}

func TestChangesKeepsSystemEffects(t *testing.T) {
	got := Changes([]Found{
		{Name: "screenrecorder", Version: "0:0.5.4-1.3.1.bso", Info: 3, Summary: "recorder"},
		{Name: "screenrecorder", Version: "0.5.4-1.2.1.bso", Info: 4},
		{Name: "libunused", Version: "1", Info: 6},
	})
	if len(got) != 2 || got[0].Action != "Install" || got[0].Label != "screenrecorder 0.5.4-1.3.1.bso" || got[0].Summary != "recorder" || got[1].Action != "Remove" {
		t.Fatalf("%+v", got)
	}
	fallback := ChangesFromIDs([]string{"screenrecorder;0.5.4-1.3.1.bso;aarch64;sailfishos-chum"})
	if len(fallback) != 1 || fallback[0].Action != "Install" || fallback[0].Label != "screenrecorder 0.5.4-1.3.1.bso" {
		t.Fatalf("%+v", fallback)
	}
}

func TestRefreshThenInstallsCatalogBuild(t *testing.T) {
	repo := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.0.0.68_aarch64/"
	alias := "sailfishos-chum"
	oldID := "screenrecorder;0.5.4-1.2.1.bso;aarch64;"
	nextID := "screenrecorder;0.5.4-1.3.1.bso;aarch64;" + alias
	stale := []Found{
		{ID: oldID + "installed", Name: "screenrecorder", Version: "0.5.4-1.2.1.bso", Data: "installed"},
		{ID: oldID + alias, Name: "screenrecorder", Version: "0.5.4-1.2.1.bso", Data: alias},
	}
	fresh := []Found{
		stale[0],
		{ID: nextID, Name: "screenrecorder", Version: "0.5.4-1.3.1.bso", Data: alias},
	}
	fake := &fakeSys{added: []Repo{{Alias: alias, URL: repo}}, resolves: [][]Found{stale, fresh}}
	ids, err := PackageIDs(fake, repo, []catalog.RPM{{
		Name: "screenrecorder", Version: "0.5.4", Release: "1.3.1.bso",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !fake.refreshed || len(ids) != 1 || ids[0] != nextID {
		t.Fatalf("refreshed %v ids %v", fake.refreshed, ids)
	}
}

func TestMissingBuildNamesWhatTheRepositoryHas(t *testing.T) {
	repo := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.0.0.68_aarch64/"
	alias := "sailfishos-chum"
	have := []Found{
		{ID: "screenrecorder;0.5.4-1.2.1.bso;aarch64;installed", Name: "screenrecorder", Version: "0.5.4-1.2.1.bso", Data: "installed"},
		{ID: "screenrecorder;0.5.4-1.2.1.bso;aarch64;" + alias, Name: "screenrecorder", Version: "0.5.4-1.2.1.bso", Data: alias},
	}
	fake := &fakeSys{added: []Repo{{Alias: alias, URL: repo}}, resolves: [][]Found{have, have}}
	_, err := PackageIDs(fake, repo, []catalog.RPM{{
		Name: "screenrecorder", Version: "0.5.4", Release: "1.3.1.bso",
	}})
	if err == nil || !strings.Contains(err.Error(), "0.5.4-1.2.1.bso") || !strings.Contains(err.Error(), alias) {
		t.Fatal(err)
	}
	if !fake.refreshed {
		t.Fatal("cache was not refreshed")
	}
}

func TestUnversionedInstallsNewerRepoBuild(t *testing.T) {
	repo := "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"
	alias := Alias(repo)
	repoID := "sailfish-components-gallery-qt5;1.4.0-1;aarch64;" + alias
	fake := &fakeSys{
		added: []Repo{{Alias: alias, URL: repo}},
		resolved: []Found{
			{ID: "sailfish-components-gallery-qt5;1.3.2-1.6.1.jolla;aarch64;installed", Name: "sailfish-components-gallery-qt5", Version: "1.3.2-1.6.1.jolla", Data: "installed"},
			{ID: repoID, Name: "sailfish-components-gallery-qt5", Version: "1.4.0-1", Data: alias},
		},
	}
	ids, err := PackageIDs(fake, repo, []catalog.RPM{{Name: "sailfish-components-gallery-qt5"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != repoID {
		t.Fatalf("%v", ids)
	}
	if fake.refreshed {
		t.Fatal("refreshed despite a visible repo package")
	}
}

type fakeSys struct {
	added     []Repo
	resolved  []Found
	resolves  [][]Found
	installed []string
	refreshed bool
}

func (f *fakeSys) ListRepos() ([]Repo, error) { return append([]Repo{}, f.added...), nil }
func (f *fakeSys) AddRepo(alias, raw string) error {
	f.added = append(f.added, Repo{Alias: alias, URL: raw})
	return nil
}
func (f *fakeSys) EnableRepo(alias string) error { return nil }
func (f *fakeSys) DisableRepo(alias string) error {
	kept := make([]Repo, 0, len(f.added))
	for _, repo := range f.added {
		if repo.Alias != alias {
			kept = append(kept, repo)
		}
	}
	f.added = kept
	return nil
}

func TestChumAppRepoIsReused(t *testing.T) {
	raw := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/"
	app := Repo{Alias: "sailfishos-chum", URL: "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/"}
	fake := &fakeSys{added: []Repo{app}}
	alias, added, err := EnsureRepo(fake, raw)
	if err != nil || added || alias != "sailfishos-chum" || len(fake.added) != 1 {
		t.Fatalf("alias %s added %v err %v %+v", alias, added, err, fake.added)
	}
	if err := SetRepoEnabled(fake, raw, true); err != nil || len(fake.added) != 1 {
		t.Fatalf("enable %v %+v", err, fake.added)
	}
	err = SetRepoEnabled(fake, raw, false)
	if err == nil || !strings.Contains(err.Error(), "SailfishOS:Chum") || len(fake.added) != 1 {
		t.Fatalf("disable %v %+v", err, fake.added)
	}
}

func TestChumAppOtherReleaseIsNotDuplicated(t *testing.T) {
	raw := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.0_aarch64/"
	fake := &fakeSys{added: []Repo{{
		Alias: "sailfishos-chum",
		URL:   "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/",
	}}}
	alias, added, err := EnsureRepo(fake, raw)
	if err != nil || added || alias != "sailfishos-chum" || len(fake.added) != 1 {
		t.Fatalf("alias %s added %v err %v %+v", alias, added, err, fake.added)
	}
}

func TestDisableDropsOnlyFtunaAlias(t *testing.T) {
	raw := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/"
	fake := &fakeSys{added: []Repo{
		{Alias: "sailfishos-chum", URL: raw},
		{Alias: Alias(raw), URL: raw},
	}}
	err := SetRepoEnabled(fake, raw, false)
	if err == nil || !strings.Contains(err.Error(), "SailfishOS:Chum") || len(fake.added) != 1 || fake.added[0].Alias != "sailfishos-chum" {
		t.Fatalf("%v %+v", err, fake.added)
	}
}

func TestChumTestingBlocksStableDuplicate(t *testing.T) {
	raw := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/"
	fake := &fakeSys{added: []Repo{{
		Alias: "sailfishos-chum-testing",
		URL:   "https://repo.sailfishos.org/obs/sailfishos:/chum:/testing/5.1_aarch64/",
	}}}
	alias, added, err := EnsureRepo(fake, raw)
	if err != nil || added || alias != "sailfishos-chum-testing" || len(fake.added) != 1 {
		t.Fatalf("alias %s added %v err %v %+v", alias, added, err, fake.added)
	}
}

func TestChumWithoutAppAddsFtunaAlias(t *testing.T) {
	raw := "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1_aarch64/"
	fake := &fakeSys{}
	if err := SetRepoEnabled(fake, raw, true); err != nil {
		t.Fatal(err)
	}
	if len(fake.added) != 1 || fake.added[0].Alias != Alias(raw) || fake.added[0].URL != raw {
		t.Fatalf("%+v", fake.added)
	}
	if err := SetRepoEnabled(fake, raw, false); err != nil || len(fake.added) != 0 {
		t.Fatalf("%v %+v", err, fake.added)
	}
}

func TestSetRepoEnabledTouchesOne(t *testing.T) {
	fake := &fakeSys{added: []Repo{{Alias: "openrepos-other", URL: "https://repo-openrepos.ftuna.eu/other/personal/main/"}}}
	raw := "https://repo-openrepos.ftuna.eu/piggz/personal/main/"
	if err := SetRepoEnabled(fake, raw, true); err != nil {
		t.Fatal(err)
	}
	if len(fake.added) != 2 || fake.added[1].Alias != Alias(raw) {
		t.Fatalf("%+v", fake.added)
	}
	if err := SetRepoEnabled(fake, raw, false); err != nil {
		t.Fatal(err)
	}
	if len(fake.added) != 1 || fake.added[0].Alias != "openrepos-other" {
		t.Fatalf("%+v", fake.added)
	}
}

func TestRepoEnabledSkipsUninstalledPublisher(t *testing.T) {
	repos := []Repo{
		{Alias: "sailfishos-chum", URL: "https://repo.sailfishos.org/obs/sailfishos:/chum/5.1.0.11_aarch64/"},
		{Alias: "ftuna-oss", URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"},
	}
	if !RepoEnabled(repos, "https://repo.sailfishos.org/obs/sailfishos:/chum/5.2.0.18_aarch64/") {
		t.Fatal("chum channel should count as enabled")
	}
	if !RepoEnabled(repos, "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/") {
		t.Fatal("oss repo should count as enabled")
	}
	if !RepoEnabled([]Repo{{URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"}}, "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/") {
		t.Fatal("a matching url counts even when the alias is blank")
	}
	if RepoEnabled(repos, "https://repo-openrepos.ftuna.eu/nobodyinperson/personal/main/") {
		t.Fatal("another publisher is not installed")
	}
}
func (f *fakeSys) Refresh() error { f.refreshed = true; return nil }
func (f *fakeSys) Resolve(names []string) ([]Found, error) {
	if len(f.resolves) > 0 {
		out := f.resolves[0]
		f.resolves = f.resolves[1:]
		return out, nil
	}
	return f.resolved, nil
}
func (f *fakeSys) Installed(names []string) ([]Found, error) { return nil, nil }
func (f *fakeSys) Simulate(ids []string) ([]Found, error) {
	return nil, nil
}
func (f *fakeSys) Install(ids []string) error {
	f.installed = ids
	return nil
}
