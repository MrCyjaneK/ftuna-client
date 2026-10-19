package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mrcyjanek/ftuna/client/internal/analysis"
	"github.com/mrcyjanek/ftuna/client/internal/catalog"
	"github.com/mrcyjanek/ftuna/client/internal/install"
	"github.com/ulikunitz/xz"
)

func TestRefreshFallbackAndCache(t *testing.T) {
	var icon string
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/icon.svg" {
			w.Write([]byte("<svg/>"))
			return
		}
		w.Header().Set("ETag", `"1"`)
		if r.Method == http.MethodHead {
			return
		}
		w.Write(mustXZ(t, sampleDoc(t, icon)))
	}))
	defer srv.Close()
	icon = srv.URL + "/icon.svg"

	dir := t.TempDir()
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, IndexURL: srv.URL + "/info.json.xz",
		Release: "5.2.0.17", Arch: "aarch64",
		System: &emptyBus{}, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"oss","enabled":true},{"id":"chum","enabled":false},{"id":"openrepos","enabled":false}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := application.Refresh(); err != nil {
		t.Fatal(err)
	}
	raw, err := application.State()
	if err != nil {
		t.Fatal(err)
	}
	var body state
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !body.Fallback || body.ChosenRelease != "5.1.0.11" {
		t.Fatalf("%+v", body.View)
	}
	if len(body.Fallbacks) != 1 || body.Fallbacks[0].Title != "FTuna OSS" || body.Fallbacks[0].URL == "" {
		t.Fatalf("fallbacks %+v", body.Fallbacks)
	}
	if len(body.Packages) != 1 || body.Packages[0].IconPath == "" || body.Packages[0].RepoURL == "" {
		t.Fatalf("packages %+v", body.Packages)
	}
	if body.Packages[0].Source != "FTuna OSS" || !body.Packages[0].SourceEnabled {
		t.Fatalf("source %+v", body.Packages[0])
	}
	if len(body.Packages[0].Sources) != 1 || body.Packages[0].Sources[0].Title != "FTuna OSS" || len(body.Packages[0].Sources[0].RPMs) == 0 {
		t.Fatalf("sources %+v", body.Packages[0].Sources)
	}
	if len(body.Updates) != 1 {
		t.Fatalf("updates %+v err %s", body.Updates, body.UpdateError)
	}
	found, err := application.Search("camera")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(found, []byte("harbour-advanced-camera")) {
		t.Fatalf("%s", found)
	}
	deps := application.Dependencies("harbour-advanced-camera", "", []string{"harbour-advanced-camera"})
	if len(deps) != 1 || deps[0] != "qt5-qtcore" {
		t.Fatalf("%v", deps)
	}
	listed, err := application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(listed, []byte("description")) || bytes.Contains(listed, []byte("sources")) {
		t.Fatalf("listing should stay short: %s", listed)
	}
	if !bytes.Contains(listed, []byte(`"added":"2026-09-28T12:00:00Z"`)) || !bytes.Contains(listed, []byte(`"repo":"FTuna OSS"`)) || !bytes.Contains(listed, []byte(`"color":"gray"`)) {
		t.Fatalf("listing row %s", listed)
	}
	if !bytes.Contains(listed, []byte(`"installed":"0.9.0-1"`)) || !bytes.Contains(listed, []byte(`"version":"0.9.5-1"`)) || !bytes.Contains(listed, []byte(`"harbour-advanced-camera\t0.9.5\t1"`)) {
		t.Fatalf("update row %s", listed)
	}
	full, err := application.Package("harbour-advanced-camera")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full, []byte("qt5-qtcore")) {
		t.Fatalf("%s", full)
	}
}

func TestLoadCachedBeforeNetwork(t *testing.T) {
	var hits int
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("ETag", `"1"`)
		if r.Method == http.MethodHead {
			return
		}
		w.Write(mustXZ(t, sampleDoc(t, "")))
	}))
	defer srv.Close()

	dir := t.TempDir()
	settings := `{"onboarded":true,"repositories":[{"id":"oss","enabled":true},{"id":"chum","enabled":false},{"id":"openrepos","enabled":false}]}`
	first, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, IndexURL: srv.URL + "/info.json.xz",
		Release: "5.2.0.17", Arch: "aarch64",
		System: &emptyBus{}, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.SaveSettings([]byte(settings)); err != nil {
		t.Fatal(err)
	}
	if err := first.Refresh(); err != nil {
		t.Fatal(err)
	}
	if hits == 0 {
		t.Fatal("expected a catalog download")
	}
	downloaded := hits

	offline, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, IndexURL: srv.URL + "/info.json.xz",
		Release: "5.2.0.17", Arch: "aarch64",
		System: &emptyBus{}, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := offline.LoadCached()
	if err != nil || !published {
		t.Fatalf("published %v %v", published, err)
	}
	if hits != downloaded {
		t.Fatalf("saved catalog used the network: %d requests", hits-downloaded)
	}
	listed, err := offline.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte("harbour-advanced-camera")) {
		t.Fatalf("listing %s", listed)
	}
}

type emptyBus struct{}

func (emptyBus) ListRepos() ([]install.Repo, error) {
	return []install.Repo{{Alias: "ftuna-oss", URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"}}, nil
}
func (emptyBus) AddRepo(alias, raw string) error { return nil }
func (emptyBus) Refresh() error                  { return nil }
func (emptyBus) Resolve(names []string) ([]install.Found, error) {
	return nil, nil
}
func (emptyBus) Installed(names []string) ([]install.Found, error) {
	return []install.Found{{Name: "harbour-advanced-camera", Version: "0.9.0-1"}}, nil
}
func (emptyBus) Install(ids []string) error                     { return nil }
func (emptyBus) Simulate(ids []string) ([]install.Found, error) { return nil, nil }

func sampleDoc(t *testing.T, icon string) []byte {
	t.Helper()
	doc := catalog.Doc{
		Version:    1,
		Updated:    "2026-09-28T12:00:00Z",
		Categories: []string{"application", "library"},
		Repositories: []catalog.Repo{{
			ID:  "ftuna-oss-5.1.0.11-aarch64",
			URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/",
		}},
		Packages: []catalog.Pkg{{
			ID: "harbour-advanced-camera", Category: "application", Updated: "2026-09-28T12:00:00Z",
			Summary: "Advanced camera", Description: "Better camera application",
			Icon: icon, Screenshots: []string{}, Repo: "ftuna-oss-5.1.0.11-aarch64",
			Status: catalog.Status{Sailjail: "unknown", Setuid: []catalog.Setuid{}, Permissions: []string{}, RedFlags: []catalog.Flag{}},
			RPMs: []catalog.RPM{{
				Name: "harbour-advanced-camera", Version: "0.9.5", Release: "1", Arch: "aarch64", Install: true, Requires: []string{"qt5-qtcore"},
			}},
		}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestScratchRepoJoinsExistingPackage(t *testing.T) {
	primary := gzipBytes(t, []byte(`
<metadata>
  <package type="rpm">
    <name>harbour-advanced-camera</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="2.0.0" rel="1"/>
    <summary>Advanced camera</summary>
    <description>Scratch build</description>
    <format><rpm:sourcerpm>harbour-advanced-camera-2.0.0-1.src.rpm</rpm:sourcerpm></format>
  </package>
  <package type="rpm">
    <name>harbour-mullvad</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="0.1" rel="2"/>
    <summary>Mullvad</summary>
    <description>VPN</description>
    <format><rpm:sourcerpm>harbour-mullvad-0.1-2.src.rpm</rpm:sourcerpm></format>
  </package>
</metadata>`))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info.json.xz":
			w.Header().Set("ETag", `"1"`)
			if r.Method == http.MethodHead {
				return
			}
			w.Write(mustXZ(t, sampleDoc(t, "")))
		case "/acct/mullvad-repo/info.json.xz":
			http.NotFound(w, r)
		case "/acct/mullvad-repo/aarch64/repodata/repomd.xml":
			w.Write([]byte(`<repomd><data type="primary"><location href="repodata/primary.xml.gz"/></data></repomd>`))
		case "/acct/mullvad-repo/aarch64/repodata/primary.xml.gz":
			w.Write(primary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, IndexURL: srv.URL + "/info.json.xz",
		Release: "5.2.0.17", Arch: "aarch64",
		System: &emptyBus{}, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"onboarded":true,"repositories":[` +
		`{"id":"oss","enabled":true},{"id":"chum","enabled":false},{"id":"openrepos","enabled":false},` +
		`{"id":"scratch-mullvad","title":"Scratch","enabled":true,"url":"` + srv.URL + `/acct/mullvad-repo/"}` +
		`]}`
	if _, err := application.SaveSettings([]byte(settings)); err != nil {
		t.Fatal(err)
	}
	if err := application.Refresh(); err != nil {
		t.Fatal(err)
	}
	raw, err := application.Package("harbour-advanced-camera")
	if err != nil {
		t.Fatal(err)
	}
	var camera shown
	if err := json.Unmarshal(raw, &camera); err != nil {
		t.Fatal(err)
	}
	if len(camera.Sources) != 2 {
		t.Fatalf("sources %+v", camera.Sources)
	}
	if camera.Sources[0].Title != "FTuna OSS" || camera.Sources[0].Version != "0.9.5-1" {
		t.Fatalf("oss %+v", camera.Sources[0])
	}
	if camera.Sources[1].Title != "Scratch" || camera.Sources[1].Version != "2.0.0-1" || !strings.HasSuffix(camera.Sources[1].RepoURL, "/aarch64/") {
		t.Fatalf("scratch %+v", camera.Sources[1])
	}
	if _, err := application.Package("harbour-mullvad"); err != nil {
		t.Fatal(err)
	}
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestListingNewestAddedFirst(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64", System: &emptyBus{}})
	if err != nil {
		t.Fatal(err)
	}
	application.view.Packages = []catalog.Pkg{
		{ID: "old", Summary: "Old", Updated: "2020-01-02T00:00:00Z", Category: "application"},
		{ID: "new", Summary: "New", Updated: "2026-09-28T12:00:00Z", Category: "application", Status: catalog.Status{Sailjail: "sandboxed"}},
	}
	application.pkgVariants = map[string][]variant{
		"old": {{PrefID: "openrepos"}},
		"new": {{PrefID: "oss"}},
	}
	raw, err := application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Packages []listPkg `json:"packages"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Packages) != 2 || body.Packages[0].ID != "new" || body.Packages[1].ID != "old" {
		t.Fatalf("%+v", body.Packages)
	}
	if body.Packages[0].Repo != "FTuna OSS" || body.Packages[0].Color != catalog.ColorGreen {
		t.Fatalf("new %+v", body.Packages[0])
	}
	if body.Packages[1].Repo != "OpenRepos" || body.Packages[1].Color != catalog.ColorGray || body.Packages[1].Added != "2020-01-02T00:00:00Z" {
		t.Fatalf("old %+v", body.Packages[1])
	}
}

func TestRememberedFallbackStaysQuiet(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.2.0.17", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	note := `[{"title":"FTuna OSS","device_release":"5.2.0.17","chosen_release":"5.1.0.11","arch":"aarch64"}]`
	if _, err := application.RememberFallbacks([]byte(note)); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"oss","enabled":false}]}`)); err != nil {
		t.Fatal(err)
	}
	got, err := application.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SeenFallbacks) != 1 || got.SeenFallbacks[0].ChosenRelease != "5.1.0.11" {
		t.Fatalf("%+v", got.SeenFallbacks)
	}
	if !got.Onboarded || got.Repositories[0].Enabled {
		t.Fatalf("repo save lost %+v", got.Repositories[0])
	}
	again := `[{"title":"FTuna OSS","device_release":"5.2.0.17","chosen_release":"5.2.0.16","arch":"aarch64"}]`
	if _, err := application.RememberFallbacks([]byte(again)); err != nil {
		t.Fatal(err)
	}
	got, err = application.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SeenFallbacks) != 2 {
		t.Fatalf("new release should be recorded separately: %+v", got.SeenFallbacks)
	}
}

func TestCustomRepositoryIsKept(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.2.0.17", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"repositories": [
			{"id":"oss","enabled":false},
			{"id":"chum","enabled":false},
			{"id":"openrepos","enabled":true},
			{"title":"Mine","url":"https://example.com/rpm/info.json.xz","enabled":true}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Onboarded {
		t.Fatal("onboarded")
	}
	var custom RepoPref
	for _, repo := range got.Repositories {
		if repo.ID == "oss" && repo.Enabled {
			t.Fatal("oss should stay off")
		}
		if repo.Title == "Mine" {
			custom = repo
		}
	}
	if custom.URL != "https://example.com/rpm/" || !custom.Enabled || custom.Host != "example.com" {
		t.Fatalf("%+v", custom)
	}
	edited, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"repositories": [
			{"id":"chum","enabled":true,"title":"Chum","url":"https://repo-chum.ftuna.eu/obs/sailfishos:/chum/{version}_{arch}/"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range edited.Repositories {
		if repo.ID != "chum" {
			continue
		}
		if !repo.Enabled || repo.URL != "https://repo.sailfishos.org/obs/sailfishos:/chum/{version}_{arch}/" || repo.Host != "repo.sailfishos.org" {
			t.Fatalf("%+v", repo)
		}
	}
	customChum, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"repositories": [
			{"id":"chum","enabled":true,"title":"Chum","url":"https://example.com/chum/{version}_{arch}/"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range customChum.Repositories {
		if repo.ID == "chum" && repo.URL != "https://example.com/chum/{version}_{arch}/" {
			t.Fatalf("custom chum url %+v", repo)
		}
	}
	if indexURL(custom, "") != "https://example.com/rpm/info.json.xz" {
		t.Fatal(indexURL(custom, ""))
	}
}

func TestScratchpadRepositoryKeepsPublisherAndRepo(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.2.0.17", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	const want = "https://scratchpad-repo.ftuna.eu/92eb8a757d1314c967c8e1807929adcb/acb1cf619edf4d23ae8df98df159d3c1/"
	got, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"repositories": [
			{"title":"Pad","url":"https://scratchpad-repo.ftuna.eu/92eb8a757d1314c967c8e1807929adcb/acb1cf619edf4d23ae8df98df159d3c1/info.json.xz","enabled":true,"color":"blue"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var pad RepoPref
	for _, repo := range got.Repositories {
		if repo.Title == "Pad" {
			pad = repo
		}
	}
	if pad.URL != want || pad.Host != "scratchpad-repo.ftuna.eu" || pad.Color != "blue" || !pad.Enabled {
		t.Fatalf("%+v", pad)
	}
	if indexURL(pad, "") != want+"info.json.xz" {
		t.Fatal(indexURL(pad, ""))
	}
}

func TestRepoPackagesKeepsOnlyThatRepository(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.2.0.17", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	application.view.Packages = []catalog.Pkg{
		{ID: "both", Summary: "Both", Updated: "2026-01-02T00:00:00Z"},
		{ID: "only-oss", Summary: "OSS", Updated: "2026-01-01T00:00:00Z"},
	}
	application.pkgVariants = map[string][]variant{
		"both": {
			{PrefID: "oss", Pkg: catalog.Pkg{ID: "both", Summary: "Both"}},
			{PrefID: "pad", Pkg: catalog.Pkg{ID: "both", Summary: "From pad"}},
		},
		"only-oss": {
			{PrefID: "oss", Pkg: catalog.Pkg{ID: "only-oss", Summary: "OSS"}},
		},
	}
	raw, err := application.RepoPackages("pad")
	if err != nil {
		t.Fatal(err)
	}
	var rows []listPkg
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "both" || rows[0].Summary != "Both" {
		t.Fatalf("%+v", rows)
	}
}

func TestPublisherNameSurvivesRepoSave(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.2.0.17", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	named, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"publisher_names": [{"id":"92eb8a757d1314c967c8e1807929adcb","title":"Ada"},{"id":"bad/id","title":"No"},{"id":"kept","title":"  "}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(named.PublisherNames) != 1 || named.PublisherNames[0].ID != "92eb8a757d1314c967c8e1807929adcb" || named.PublisherNames[0].Title != "Ada" {
		t.Fatalf("%+v", named.PublisherNames)
	}
	again, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"oss","enabled":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.PublisherNames) != 1 || again.PublisherNames[0].Title != "Ada" {
		t.Fatalf("name dropped %+v", again.PublisherNames)
	}
	cleared, err := application.SaveSettings([]byte(`{"onboarded":true,"publisher_names":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.PublisherNames) != 0 {
		t.Fatalf("%+v", cleared.PublisherNames)
	}
}

func TestReportFollowsSelectedSource(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64", System: &emptyBus{}})
	if err != nil {
		t.Fatal(err)
	}
	oss := catalog.Pkg{
		ID:     "harbour-advanced-camera",
		Status: catalog.Status{Sailjail: "sandboxed"},
		RPMs: []catalog.RPM{{
			Name: "harbour-advanced-camera", Version: "1.0", Release: "1", Install: true,
			Status: catalog.Status{Sailjail: "sandboxed"},
		}},
	}
	chum := catalog.Pkg{
		ID:     "openrepos.net/piggz/harbour-advanced-camera",
		Status: catalog.Status{Sailjail: "no"},
		RPMs: []catalog.RPM{{
			Name: "harbour-advanced-camera", Version: "2.0", Release: "1", Install: true,
			Status: catalog.Status{Sailjail: "no", Setuid: []catalog.Setuid{{Path: "/usr/bin/x", UID: 0}}},
		}},
	}
	application.view.Packages = []catalog.Pkg{oss}
	application.pkgVariants = map[string][]variant{
		"harbour-advanced-camera": {
			{PrefID: "oss", Pkg: oss},
			{PrefID: "chum", Pkg: chum},
		},
	}
	got := application.Report("harbour-advanced-camera", "chum", []string{"harbour-advanced-camera\t2.0\t1"})
	if got.Color != catalog.ColorYellow {
		t.Fatalf("chum %+v", got)
	}
	got = application.Report("harbour-advanced-camera", "oss", []string{"harbour-advanced-camera\t1.0\t1"})
	if got.Color != catalog.ColorGreen {
		t.Fatalf("oss %+v", got)
	}
}

func TestPresentKeepsEverySource(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64", System: &emptyBus{}})
	if err != nil {
		t.Fatal(err)
	}
	rpm := func(version string) catalog.Pkg {
		return catalog.Pkg{
			ID: "harbour-advanced-camera", Summary: "Camera " + version,
			RPMs: []catalog.RPM{{Name: "harbour-advanced-camera", Version: version, Release: "1", Install: true, Requires: []string{"qt5-qtcore"}}},
		}
	}
	application.pkgVariants = map[string][]variant{
		"harbour-advanced-camera": {
			{PrefID: "oss", RepoURL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/", Pkg: rpm("0.9.5")},
			{PrefID: "chum", RepoURL: "https://repo-chum.ftuna.eu/5.1.0.11/aarch64/", Pkg: rpm("0.9.8")},
		},
	}
	got := application.present(rpm("0.9.5"), nil, DefaultSettings().Repositories)
	if len(got.Sources) != 2 {
		t.Fatalf("sources %d", len(got.Sources))
	}
	if got.Sources[0].Title != "FTuna OSS" || !got.Sources[0].Enabled || got.Sources[0].RPMs[0].Version != "0.9.5" {
		t.Fatalf("oss %+v", got.Sources[0])
	}
	if got.Sources[1].Title != "Chum" || !got.Sources[1].Enabled || got.Sources[1].RPMs[0].Version != "0.9.8" {
		t.Fatalf("chum %+v", got.Sources[1])
	}
	_, chosen := application.chosenRPMs("https://repo-chum.ftuna.eu/5.1.0.11/aarch64/", []string{"harbour-advanced-camera"})
	if len(chosen) != 1 || chosen[0].Version != "0.9.8" {
		t.Fatalf("chosen %+v", chosen)
	}
}

func TestPresentSelectsOnlyNewestVersion(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64", System: &emptyBus{}})
	if err != nil {
		t.Fatal(err)
	}
	pkg := catalog.Pkg{
		ID: "harbour-demo",
		RPMs: []catalog.RPM{
			{Name: "harbour-demo", Version: "1.0", Release: "1", Arch: "aarch64", Install: true},
			{Name: "harbour-demo", Version: "1.2", Release: "3", Arch: "aarch64", Install: true},
		},
	}
	repo := "https://repo-openrepos.ftuna.eu/piggz/personal/main/"
	application.pkgVariants = map[string][]variant{
		"harbour-demo": {{PrefID: "openrepos", RepoURL: repo, Pkg: pkg}},
	}
	got := application.present(pkg, nil, nil)
	if got.Source != "or/piggz" || got.Sources[0].Title != "or/piggz" {
		t.Fatalf("source %q %+v", got.Source, got.Sources[0])
	}
	rpms := got.Sources[0].RPMs
	if len(rpms) != 2 || rpms[0].Version != "1.2" || !rpms[0].Install || rpms[1].Version != "1.0" || rpms[1].Install {
		t.Fatalf("%+v", rpms)
	}
	_, chosen := application.chosenRPMs(repo, []string{"harbour-demo\t1.0\t1"})
	if len(chosen) != 1 || chosen[0].Version != "1.0" || chosen[0].Release != "1" {
		t.Fatalf("older %+v", chosen)
	}
	_, chosen = application.chosenRPMs(repo, []string{"harbour-demo"})
	if len(chosen) != 1 || chosen[0].Version != "1.2" {
		t.Fatalf("newest %+v", chosen)
	}
}

func TestInstalledIDs(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64",
		System: idBus{},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := application.InstalledIDs([]string{"harbour-advanced-camera", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "harbour-advanced-camera;0.9.5-1;aarch64;installed" {
		t.Fatalf("%v", ids)
	}
	if _, err := application.InstalledIDs(nil); err == nil {
		t.Fatal("expected no packages selected")
	}
}

type idBus struct{ emptyBus }

func (idBus) Installed(names []string) ([]install.Found, error) {
	return []install.Found{{
		ID:   "harbour-advanced-camera;0.9.5-1;aarch64;installed",
		Name: "harbour-advanced-camera",
	}}, nil
}

func TestSameRPMJoinsScratchpadAndOpenRepos(t *testing.T) {
	var listings packageListings
	listings.add("scratch", "https://scratch.example/5.1.0.11/aarch64/", catalog.Pkg{
		ID:      "github.com/piggz/harbour-advanced-camera",
		Summary: "Advanced Camera",
		Icon:    "icon.svg",
		RPMs:    []catalog.RPM{{Name: "harbour-advanced-camera", Version: "2.0.0", Release: "1", Arch: "aarch64", Install: true}},
	})
	listings.add("openrepos", "https://repo-openrepos.ftuna.eu/piggz/personal/main/", catalog.Pkg{
		ID:      "openrepos.net/piggz/harbour-advanced-camera",
		Summary: "Advanced Camera",
		RPMs: []catalog.RPM{
			{Name: "harbour-advanced-camera", Version: "1.9.0", Release: "1", Arch: "aarch64", Install: true},
			{Name: "harbour-advanced-camera-debuginfo", Version: "1.9.0", Release: "1", Arch: "aarch64"},
		},
	})
	listings.add("chum", "https://repo-chum.ftuna.eu/5.1.0.11/aarch64/", catalog.Pkg{
		ID:   "other-app",
		RPMs: []catalog.RPM{{Name: "other-app", Version: "1", Release: "1", Arch: "aarch64", Install: true}},
	})
	listed, variants, _ := listings.emit()
	if len(listed) != 2 {
		t.Fatalf("listings %d", len(listed))
	}
	sources := variants["github.com/piggz/harbour-advanced-camera"]
	if len(sources) != 2 || sources[0].PrefID != "scratch" || sources[1].PrefID != "openrepos" {
		t.Fatalf("%+v", sources)
	}
	if len(variants["other-app"]) != 1 {
		t.Fatalf("other %+v", variants["other-app"])
	}
}

func TestRefreshLeavesAnalysisUntilPackageOpen(t *testing.T) {
	const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	catalogXZ := mustXZ(t, analysisCatalog(t, sum))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"1"`)
		if r.Method == http.MethodHead {
			return
		}
		w.Write(catalogXZ)
	}))
	defer srv.Close()

	dir := t.TempDir()
	data := t.TempDir()
	seedRatings(t, data, sum, analysis.Result{
		Version: analysis.Version, SHA256: sum,
		Issues: []analysis.Issue{{Code: "sailjail.permission", Detail: "Audio"}},
	})
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, DataDir: data, IndexURL: srv.URL + "/info.json.xz",
		Release: "5.2.0.17", Arch: "aarch64",
		System: &emptyBus{}, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"oss","enabled":true},{"id":"chum","enabled":false},{"id":"openrepos","enabled":false}]}`)); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- application.Refresh() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh blocked on analysis")
	}
	listed, err := application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte(`"color":"gray"`)) {
		t.Fatalf("listing before open %s", listed)
	}
	application.ensureAnalysis(application.catalogSHAs())
	listed, err = application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte(`"color":"green"`)) {
		t.Fatalf("listing after ratings %s", listed)
	}

	full, err := application.Package("harbour-advanced-camera")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(full, []byte(`"color":"green"`)) || !bytes.Contains(full, []byte("Audio")) {
		t.Fatalf("package %s", full)
	}
	rep := application.Report("harbour-advanced-camera", "oss", []string{"harbour-advanced-camera\t0.9.5\t1"})
	if rep.Sample || rep.Color != catalog.ColorGreen {
		t.Fatalf("%+v", rep)
	}
	sawAudio := false
	for _, pkg := range rep.Packages {
		for _, row := range pkg.Findings {
			if strings.Contains(row.Detail, "Audio") && row.Title != "" {
				sawAudio = true
			}
		}
	}
	if !sawAudio {
		t.Fatalf("packages %+v", rep.Packages)
	}
	if _, err := application.Package("harbour-advanced-camera"); err != nil {
		t.Fatal(err)
	}
	listed, err = application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte(`"color":"green"`)) {
		t.Fatalf("listing after open %s", listed)
	}
}

func analysisCatalog(t *testing.T, sum string) []byte {
	t.Helper()
	doc := catalog.Doc{
		Version:    1,
		Updated:    "2026-09-28T12:00:00Z",
		Categories: []string{"application"},
		Repositories: []catalog.Repo{{
			ID:  "ftuna-oss-5.1.0.11-aarch64",
			URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/",
		}},
		Packages: []catalog.Pkg{{
			ID: "harbour-advanced-camera", Category: "application", Updated: "2026-09-28T12:00:00Z",
			Summary: "Advanced camera", Repo: "ftuna-oss-5.1.0.11-aarch64",
			Status: catalog.Status{Sailjail: "unknown"},
			RPMs: []catalog.RPM{{
				Name: "harbour-advanced-camera", Version: "0.9.5", Release: "1", Arch: "aarch64",
				Install: true, SHA256: sum,
			}},
		}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func seedRatings(t *testing.T, dataDir, sum string, doc analysis.Result) {
	t.Helper()
	origin := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %s", args, out)
		}
	}
	run(origin, "init", "-b", "master")
	run(origin, "config", "user.email", "t@example.com")
	run(origin, "config", "user.name", "test")
	run(origin, "config", "commit.gpgsign", "false")
	rel := sum[:2] + "/" + sum[2:4] + "/" + sum[4:6] + "/" + sum + ".json"
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(origin, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatal(err)
	}
	run(origin, "add", rel)
	run(origin, "commit", "-m", "rating")
	run("", "clone", "--bare", origin, filepath.Join(dataDir, "ratings.git"))
	stamp := []byte(strconv.FormatInt(time.Now().Unix(), 10))
	if err := os.WriteFile(filepath.Join(dataDir, "ratings.fetched"), stamp, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPreferredSourceUsesVersionThenDate(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64", System: &emptyBus{}})
	if err != nil {
		t.Fatal(err)
	}
	const id = "harbour-advanced-camera"
	oss := catalog.Pkg{
		ID: id, Summary: "Camera", Updated: "2026-01-01T00:00:00Z", Category: "application",
		RPMs: []catalog.RPM{{
			Name: "harbour-advanced-camera", Version: "2.0", Release: "1", Arch: "aarch64", Install: true,
			Status: catalog.Status{Sailjail: "no", Setuid: []catalog.Setuid{{Path: "/usr/bin/x", UID: 0}}},
		}},
	}
	chum := catalog.Pkg{
		ID: "chum-" + id, Summary: "Camera", Updated: "2026-02-01T00:00:00Z", Category: "application",
		RPMs: []catalog.RPM{{
			Name: "harbour-advanced-camera", Version: "0.9.0", Release: "1", Arch: "aarch64", Install: true,
			Status: catalog.Status{Sailjail: "sandboxed"},
		}},
	}
	application.view = emptyView("5.1.0.11", "aarch64")
	application.view.Packages = []catalog.Pkg{oss}
	application.pkgVariants = map[string][]variant{
		id: {
			{PrefID: "oss", RepoURL: "https://repo-oss.example/5.1.0.11/aarch64/", Pkg: oss},
			{PrefID: "chum", RepoURL: "https://repo.example/chum/", Pkg: chum},
		},
	}
	application.installed = []catalog.Installed{{Name: "harbour-advanced-camera", Version: "0.9.0-1"}}
	application.installedOK = true
	application.enabledRepos = []install.Repo{{URL: "https://repo-oss.example/5.1.0.11/aarch64/"}}
	application.recomputePreferredLocked()
	if application.preferred[id] != "oss" {
		t.Fatalf("preferred %q", application.preferred[id])
	}
	if len(application.view.Updates) != 1 || application.view.Updates[0] != id {
		t.Fatalf("newer preferred source should be an update: %+v", application.view.Updates)
	}
	listed, err := application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte(`"repo":"FTuna OSS"`)) || !bytes.Contains(listed, []byte(`"preferred_source":"oss"`)) {
		t.Fatalf("listing %s", listed)
	}
	got := application.present(oss, nil, DefaultSettings().Repositories)
	if got.PreferredSource != "oss" {
		t.Fatalf("preferred %+v", got.PreferredSource)
	}
	if len(got.Sources) != 2 || got.Sources[0].Version != "2.0-1" || got.Sources[1].Version != "0.9.0-1" {
		t.Fatalf("versions %+v %+v", got.Sources[0], got.Sources[1])
	}

	oss.RPMs[0].Version = "1.0"
	chum.RPMs[0].Version = "1.0"
	application.pkgVariants[id][0].Pkg = oss
	application.pkgVariants[id][1].Pkg = chum
	application.recomputePreferredLocked()
	if application.preferred[id] != "chum" {
		t.Fatalf("newer publication %q", application.preferred[id])
	}

	oss.RPMs[0].Version = "2.0"
	chum.RPMs[0].Version = "3.0"
	application.pkgVariants[id][0].Pkg = oss
	application.pkgVariants[id][1].Pkg = chum
	application.enabledRepos = []install.Repo{{URL: "https://repo.example/chum/"}}
	application.recomputePreferredLocked()
	if application.preferred[id] != "chum" {
		t.Fatalf("newer version %q", application.preferred[id])
	}
	if len(application.view.Updates) != 1 || application.view.Updates[0] != id {
		t.Fatalf("enabled source update %+v", application.view.Updates)
	}
	listed, err = application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(listed, []byte(`"version":"3.0-1"`)) || !bytes.Contains(listed, []byte(`"repo":"Chum"`)) {
		t.Fatalf("update should follow the enabled repository %s", listed)
	}

	publisher := catalog.Pkg{
		ID: "openrepos.net/nobodyinperson/harbour-advanced-camera", Summary: "Camera", Updated: "2026-03-01T00:00:00Z",
		RPMs: []catalog.RPM{{
			Name: "harbour-advanced-camera", Version: "9.0", Release: "1", Arch: "aarch64", Install: true,
		}},
	}
	application.pkgVariants[id] = append(application.pkgVariants[id], variant{
		PrefID: "openrepos", RepoURL: "https://repo-openrepos.ftuna.eu/nobodyinperson/personal/main/", Pkg: publisher,
	})
	application.recomputePreferredLocked()
	if application.preferred[id] != "openrepos" {
		t.Fatalf("newest catalog source %q", application.preferred[id])
	}
	if len(application.view.Updates) != 1 || application.view.Updates[0] != id {
		t.Fatalf("uninstalled publisher must not replace the enabled update: %+v", application.view.Updates)
	}
	listed, err = application.Listing()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(listed, []byte("nobodyinperson")) || !bytes.Contains(listed, []byte(`"version":"3.0-1"`)) || !bytes.Contains(listed, []byte(`"repo":"Chum"`)) {
		t.Fatalf("update stayed with the enabled repository %s", listed)
	}
}

func TestDependenciesFollowsSource(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	const id = "harbour-advanced-camera"
	oss := catalog.Pkg{ID: id, RPMs: []catalog.RPM{{
		Name: "harbour-advanced-camera", Version: "1.0", Release: "1", Install: true, Requires: []string{"qt5-qtcore"},
	}}}
	chum := catalog.Pkg{ID: "other", RPMs: []catalog.RPM{{
		Name: "harbour-advanced-camera", Version: "2.0", Release: "1", Install: true, Requires: []string{"sailfishapp"},
	}}}
	application.view.Packages = []catalog.Pkg{oss}
	application.pkgVariants = map[string][]variant{
		id: {
			{PrefID: "oss", Pkg: oss},
			{PrefID: "chum", Pkg: chum},
		},
	}
	ossDeps := application.Dependencies(id, "oss", []string{"harbour-advanced-camera"})
	if len(ossDeps) != 1 || ossDeps[0] != "qt5-qtcore" {
		t.Fatalf("oss %v", ossDeps)
	}
	chumDeps := application.Dependencies(id, "chum", []string{"harbour-advanced-camera"})
	if len(chumDeps) != 1 || chumDeps[0] != "sailfishapp" {
		t.Fatalf("chum %v", chumDeps)
	}
}

func TestCustomReposOnOneHostStayDistinct(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{CacheDir: dir, ConfigDir: dir, Release: "5.1.0.11", Arch: "aarch64"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := application.SaveSettings([]byte(`{
		"onboarded": true,
		"repositories": [
			{"title":"One","url":"https://example.com/one/","enabled":true},
			{"title":"Two","url":"https://example.com/two/","enabled":true}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var one, two RepoPref
	for _, repo := range got.Repositories {
		if repo.Title == "One" {
			one = repo
		}
		if repo.Title == "Two" {
			two = repo
		}
	}
	if one.ID == "" || one.ID == two.ID || one.URL != "https://example.com/one/" || two.URL != "https://example.com/two/" {
		t.Fatalf("one %+v two %+v", one, two)
	}
}

func TestWipeRemovesOnlyAppDirs(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache", "ftuna")
	config := filepath.Join(root, "config", "ftuna")
	data := filepath.Join(root, "data", "ftuna")
	neighbor := filepath.Join(root, "cache", "other")
	application, err := Open(Config{
		CacheDir: cache, ConfigDir: config, DataDir: data,
		Release: "5.2.0.17", Arch: "aarch64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"oss","enabled":false}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "info.json.xz"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "ratings.fetched"), []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(neighbor, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(neighbor, "keep"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := application.Wipe(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cache, config, data} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s still present: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(neighbor, "keep")); err != nil {
		t.Fatal(err)
	}
	settings, err := application.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Onboarded {
		t.Fatal("settings survived wipe")
	}
	for _, repo := range settings.Repositories {
		if repo.ID == "oss" && !repo.Enabled {
			t.Fatal("oss enablement survived wipe")
		}
	}
}

func TestWipeRefusesBroadPath(t *testing.T) {
	dir := t.TempDir()
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir, DataDir: dir,
		Release: "5.2.0.17", Arch: "aarch64",
	})
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "keep")
	if err := os.WriteFile(keep, []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := application.Wipe(); err == nil {
		t.Fatal("expected refusal")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal(err)
	}
}

func TestReadLimitedRejectsOverflow(t *testing.T) {
	if _, err := readLimited(bytes.NewReader(make([]byte, maxFetch+1))); err == nil {
		t.Fatal("expected limit")
	}
	got, err := readLimited(strings.NewReader("ok"))
	if err != nil || string(got) != "ok" {
		t.Fatalf("%q %v", got, err)
	}
}

type reposBus struct {
	emptyBus
	repos []install.Repo
}

func (b reposBus) ListRepos() ([]install.Repo, error) {
	return append([]install.Repo{}, b.repos...), nil
}

func openReposDoc(t *testing.T) []byte {
	t.Helper()
	doc := catalog.Doc{
		Version: 1,
		Repositories: []catalog.Repo{
			{ID: "openrepos-mrcyjanek", URL: "https://repo-openrepos.ftuna.eu/mrcyjanek/personal/main/"},
			{ID: "openrepos-poetaster", URL: "https://repo-openrepos.ftuna.eu/poetaster/personal/main/"},
			{ID: "oss", URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"},
			{ID: "openrepos-ade", URL: "https://repo-openrepos.ftuna.eu/ade/personal/main"},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOpenReposUsersMarksEnabled(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "catalogs", "openrepos", "index")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "info.json.xz"), mustXZ(t, openReposDoc(t)), 0644); err != nil {
		t.Fatal(err)
	}
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir,
		Release: "5.2.0.17", Arch: "aarch64",
		System: reposBus{repos: []install.Repo{{
			Alias: "poetaster",
			URL:   "https://repo-openrepos.ftuna.eu/poetaster/personal/main/",
		}}},
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("cached index used the network")
			return nil, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	users, err := application.OpenReposUsers(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 || users[0].Title != "poetaster" || !users[0].Enabled || users[0].ID != "openrepos-poetaster" {
		t.Fatalf("enabled publisher: %+v", users)
	}
	if users[1].Title != "ade" || users[1].Enabled || users[2].Title != "mrcyjanek" || users[2].Enabled {
		t.Fatalf("disabled publishers: %+v", users)
	}
}

func TestOpenReposUsersFetchesWhenMissing(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/info.json.xz" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"openrepos"`)
		if r.Method == http.MethodHead {
			return
		}
		w.Write(mustXZ(t, openReposDoc(t)))
	}))
	defer srv.Close()

	dir := t.TempDir()
	application, err := Open(Config{
		CacheDir: dir, ConfigDir: dir,
		Release: "5.2.0.17", Arch: "aarch64",
		System: reposBus{repos: []install.Repo{{
			URL: "https://repo-openrepos.ftuna.eu/poetaster/personal/main/",
		}}},
		HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveSettings([]byte(`{"onboarded":true,"repositories":[{"id":"openrepos","enabled":true,"url":"` + srv.URL + `/"}]}`)); err != nil {
		t.Fatal(err)
	}
	users, err := application.OpenReposUsers(false)
	if err != nil {
		t.Fatal(err)
	}
	if hits == 0 || len(users) != 3 || !users[0].Enabled || users[0].Title != "poetaster" {
		t.Fatalf("hits %d users %+v", hits, users)
	}
	seen := hits
	if _, err := application.OpenReposUsers(false); err != nil {
		t.Fatal(err)
	}
	if hits != seen {
		t.Fatalf("cached read used the network: %d then %d", seen, hits)
	}
	if _, err := application.OpenReposUsers(true); err != nil {
		t.Fatal(err)
	}
	if hits == seen {
		t.Fatal("refresh did not contact the index")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mustXZ(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
