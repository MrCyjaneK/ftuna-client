// Package app is the on-device catalog: fetch, filter, cache, and install.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrcyjanek/ftuna/client/internal/analysis"
	"github.com/mrcyjanek/ftuna/client/internal/cache"
	"github.com/mrcyjanek/ftuna/client/internal/catalog"
	"github.com/mrcyjanek/ftuna/client/internal/device"
	"github.com/mrcyjanek/ftuna/client/internal/install"
	"github.com/mrcyjanek/ftuna/client/internal/layout"
	"github.com/ulikunitz/xz"
)

// Config selects where the library stores files and which phone it targets.
type Config struct {
	CacheDir  string
	ConfigDir string
	IndexURL  string
	Release   string
	Arch      string
	// System, when set, is used instead of the system bus. Tests use this.
	System install.System
	HTTP   *http.Client
	// DataDir is the app's local storage. The ratings git clone lives here.
	DataDir string
	// PrepareRatings force-fetches the ratings clone during Refresh, then loads ratings.
	PrepareRatings bool
}

// App is safe to call from the UI thread and one worker.
type App struct {
	cacheDir  string
	configDir string
	dataDir   string
	indexURL  string
	release   string
	arch      string
	system    install.System
	client    *http.Client

	mu          sync.Mutex
	progress    atomic.Int32
	status      atomic.Value
	view        catalog.View
	pkgRepo     map[string]string
	pkgVariants map[string][]variant
	catalogs    []CatalogInfo
	fallbacks   []Fallback
	ready       bool
	lastErr     string
	listColors  map[string]string
	listLines   map[string]catalog.Line
	preferred   map[string]string
	installed   []catalog.Installed
	installedOK bool
	// enabledRepos are the rpm repositories SSU currently has turned on.
	// An update can only come from one of these.
	enabledRepos []install.Repo
	// updateOffer is the enabled repository that supplies each update.
	updateOffer  map[string]variant
	settings     Settings
	haveSettings bool
	// tried digests are ones the ratings repository has answered, including a missing file.
	tried          map[string]bool
	prepareRatings bool
	preparing      atomic.Bool
	ratingsGen     atomic.Int32
	catalogGen     atomic.Int32
}

// variant is one catalog's copy of a package.
type variant struct {
	PrefID  string
	RepoURL string
	Pkg     catalog.Pkg
}

// packageListings merges catalogs that ship the same RPM. Scratchpad ids such as
// github.com/piggz/harbour-advanced-camera and OpenRepos ids such as
// openrepos.net/piggz/harbour-advanced-camera are one listing when both build
// harbour-advanced-camera.
type packageListings struct {
	items []*listingGroup
	owner map[string]int
	byID  map[string]int
}

type listingGroup struct {
	id      string
	repoURL string
	face    catalog.Pkg
	sources []variant
}

func (g *packageListings) add(prefID, repoURL string, pkg catalog.Pkg) {
	if g.owner == nil {
		g.owner = map[string]int{}
		g.byID = map[string]int{}
	}
	hit := -1
	if idx, ok := g.byID[pkg.ID]; ok {
		hit = g.join(hit, idx)
	}
	for _, name := range installableRPMNames(pkg) {
		if idx, ok := g.owner[name]; ok {
			hit = g.join(hit, idx)
		}
	}
	if hit < 0 {
		hit = len(g.items)
		g.items = append(g.items, &listingGroup{id: pkg.ID, repoURL: repoURL, face: pkg})
	}
	g.items[hit].sources = append(g.items[hit].sources, variant{PrefID: prefID, RepoURL: repoURL, Pkg: pkg})
	g.byID[pkg.ID] = hit
	for _, name := range installableRPMNames(pkg) {
		g.owner[name] = hit
	}
	if len(installableRPMNames(pkg)) == 0 {
		g.owner["id\t"+pkg.ID] = hit
	}
}

func (g *packageListings) join(a, b int) int {
	if a < 0 {
		return b
	}
	if b < 0 || a == b {
		return a
	}
	if b < a {
		a, b = b, a
	}
	g.absorb(a, b)
	return a
}

func (g *packageListings) absorb(dst, src int) {
	if dst == src || g.items[src] == nil {
		return
	}
	g.items[dst].sources = append(g.items[dst].sources, g.items[src].sources...)
	for name, idx := range g.owner {
		if idx == src {
			g.owner[name] = dst
		}
	}
	for id, idx := range g.byID {
		if idx == src {
			g.byID[id] = dst
		}
	}
	g.items[src] = nil
}

func (g *packageListings) emit() (listed []catalog.Pkg, variants map[string][]variant, repos map[string]string) {
	variants = map[string][]variant{}
	repos = map[string]string{}
	for _, item := range g.items {
		if item == nil {
			continue
		}
		listed = append(listed, item.face)
		variants[item.id] = item.sources
		repos[item.id] = item.repoURL
	}
	if listed == nil {
		listed = []catalog.Pkg{}
	}
	return listed, variants, repos
}

func installableRPMNames(pkg catalog.Pkg) []string {
	seen := map[string]bool{}
	var names []string
	for _, rpm := range pkg.RPMs {
		if rpm.Name == "" || seen[rpm.Name] || !installableRPM(rpm.Name) {
			continue
		}
		seen[rpm.Name] = true
		names = append(names, rpm.Name)
	}
	return names
}

func installableRPM(name string) bool {
	return !strings.HasSuffix(name, "-debuginfo") && !strings.HasSuffix(name, "-debugsource") && !strings.HasSuffix(name, "-doc")
}

// CatalogInfo is how one configured repository resolved for this phone.
type CatalogInfo struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	DeviceRelease string `json:"device_release"`
	ChosenRelease string `json:"chosen_release"`
	Arch          string `json:"arch"`
	Fallback      bool   `json:"fallback"`
	Error         string `json:"error"`
}

// Fallback names a repository that had to use an older release.
type Fallback struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	DeviceRelease string `json:"device_release"`
	ChosenRelease string `json:"chosen_release"`
	Arch          string `json:"arch"`
}

func Open(cfg Config) (*App, error) {
	if cfg.IndexURL == "" {
		cfg.IndexURL = catalog.IndexURL
	}
	if cfg.CacheDir == "" || cfg.ConfigDir == "" {
		cacheDir, configDir, err := defaultDirs()
		if err != nil {
			return nil, err
		}
		if cfg.CacheDir == "" {
			cfg.CacheDir = cacheDir
		}
		if cfg.ConfigDir == "" {
			cfg.ConfigDir = configDir
		}
	}
	if cfg.DataDir == "" {
		dataDir, err := defaultDataDir()
		if err != nil {
			return nil, err
		}
		cfg.DataDir = dataDir
	}
	if cfg.Release == "" || cfg.Arch == "" {
		release, arch, err := device.Detect()
		if err != nil && (cfg.Release == "" || cfg.Arch == "") {
			return nil, err
		}
		if cfg.Release == "" {
			cfg.Release = release
		}
		if cfg.Arch == "" {
			cfg.Arch = arch
		}
	}
	client := cache.OrDefault(cfg.HTTP)
	app := &App{
		cacheDir:       cfg.CacheDir,
		configDir:      cfg.ConfigDir,
		dataDir:        cfg.DataDir,
		indexURL:       cfg.IndexURL,
		release:        cfg.Release,
		arch:           catalog.NormalizeArch(cfg.Arch),
		system:         cfg.System,
		client:         client,
		prepareRatings: cfg.PrepareRatings,
	}
	app.status.Store("")
	return app, nil
}

func defaultDirs() (string, string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(cacheRoot, "ftuna"), filepath.Join(configRoot, "ftuna"), nil
}

func defaultDataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "ftuna"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "ftuna"), nil
}

// Wipe removes the cache, settings, and ratings clone. It only deletes
// directories whose last element is "ftuna".
func (a *App) Wipe() error {
	seen := map[string]bool{}
	var errs []error
	for _, dir := range []string{a.cacheDir, a.configDir, a.dataDir} {
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if filepath.Base(dir) != "ftuna" || dir == "ftuna" || !filepath.IsAbs(dir) {
			errs = append(errs, fmt.Errorf("refuse to wipe %s", dir))
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings = Settings{}
	a.haveSettings = false
	a.view = catalog.View{}
	a.pkgRepo = nil
	a.pkgVariants = nil
	a.catalogs = nil
	a.fallbacks = nil
	a.ready = false
	a.lastErr = ""
	a.listColors = nil
	a.listLines = nil
	a.preferred = nil
	a.installed = nil
	a.installedOK = false
	a.enabledRepos = nil
	a.updateOffer = nil
	a.tried = nil
	a.mu.Unlock()
	return nil
}

func (a *App) Progress() int { return int(a.progress.Load()) }

func (a *App) setProgress(n int) {
	if n < 0 {
		n = 0
	}
	if n > 100 {
		n = 100
	}
	a.progress.Store(int32(n))
}

func (a *App) setStatus(s string) {
	a.status.Store(s)
}

func (a *App) Status() string {
	s, _ := a.status.Load().(string)
	return s
}

// errNotCached means this phone has no saved copy of a catalog yet.
var errNotCached = errors.New("catalog is not cached")

// Refresh publishes the saved catalog, refreshes it from the network, then updates ratings.
// The saved catalog is on screen before either download starts.
func (a *App) Refresh() error {
	defer a.setStatus("")
	if _, err := a.LoadCached(); err != nil {
		a.kickPrepare()
		return err
	}
	catErr := a.RefreshCatalogs()
	ratingsErr := a.FetchRatings()
	if catErr != nil {
		return catErr
	}
	return ratingsErr
}

// FetchRatings updates the ratings repository after the catalog can already be browsed.
func (a *App) FetchRatings() error {
	defer a.kickPrepare()
	defer a.setStatus("")
	if !a.prepareRatings {
		return nil
	}
	return a.fetchRatings()
}

// LoadCached publishes catalogs and installed packages already stored on the phone.
// It does not use the network. published is false when nothing has been saved yet.
func (a *App) LoadCached() (published bool, err error) {
	defer a.setStatus("")
	a.setStatus("Opening saved catalog")
	prefs, err := a.Settings()
	if err != nil {
		return false, a.fail(err)
	}
	enabled := enabledRepos(prefs)
	if len(enabled) == 0 {
		return false, a.publishUnavailable("No repositories are turned on", "no repositories are turned on", nil)
	}
	built, ok := a.collect(enabled, func(pref RepoPref) (catalog.View, error) {
		return a.loadCachedCatalog(pref)
	}, false)
	if !ok {
		return false, nil
	}
	a.commit(built)
	return true, nil
}

// RefreshCatalogs downloads every enabled catalog. A saved catalog stays in place when every download fails.
func (a *App) RefreshCatalogs() error {
	prefs, err := a.Settings()
	if err != nil {
		return a.fail(err)
	}
	enabled := enabledRepos(prefs)
	if len(enabled) == 0 {
		return a.publishUnavailable("No repositories are turned on", "no repositories are turned on", nil)
	}
	a.setStatus("Loading repositories")
	a.setProgress(0)
	built, ok := a.collect(enabled, func(pref RepoPref) (catalog.View, error) {
		return a.loadCatalog(pref)
	}, true)
	if !ok {
		msg := "no repositories could be loaded"
		for _, info := range built.infos {
			if info.Error != "" {
				msg = info.Title + ": " + info.Error
				break
			}
		}
		a.mu.Lock()
		keep := a.ready
		a.mu.Unlock()
		if keep {
			a.mu.Lock()
			a.lastErr = msg
			a.mu.Unlock()
			a.setProgress(100)
			return fmt.Errorf("%s", msg)
		}
		return a.publishUnavailable(msg, msg, built.infos)
	}
	a.commit(built)
	return nil
}

func enabledRepos(prefs Settings) []RepoPref {
	var enabled []RepoPref
	for _, pref := range prefs.Repositories {
		if pref.Enabled {
			enabled = append(enabled, pref)
		}
	}
	return enabled
}

type builtCatalog struct {
	infos    []CatalogInfo
	notes    []Fallback
	primary  catalog.View
	variants map[string][]variant
	repos    map[string]string
}

func (a *App) collect(enabled []RepoPref, load func(RepoPref) (catalog.View, error), announce bool) (builtCatalog, bool) {
	built := builtCatalog{infos: make([]CatalogInfo, 0, len(enabled))}
	var listings packageListings
	var havePrimary bool
	var oss catalog.View
	var haveOSS bool
	categories := []catalog.Cat{}
	seenCat := map[string]bool{}
	for i, pref := range enabled {
		if announce {
			a.setProgress(5 + 30*i/len(enabled))
			name := pref.Title
			if name == "" {
				name = pref.Host
			}
			if name == "" {
				name = "repository"
			}
			a.setStatus("Loading " + name)
		}
		info := CatalogInfo{ID: pref.ID, Title: pref.Title, DeviceRelease: a.release, Arch: a.arch}
		view, err := load(pref)
		if errors.Is(err, errNotCached) {
			continue
		}
		if err != nil {
			info.Error = err.Error()
			built.infos = append(built.infos, info)
			continue
		}
		info.URL = view.RepoURL
		info.DeviceRelease = view.DeviceRelease
		info.ChosenRelease = view.ChosenRelease
		info.Arch = view.Arch
		info.Fallback = view.Fallback
		built.infos = append(built.infos, info)
		if view.Fallback {
			built.notes = append(built.notes, Fallback{
				Title:         pref.Title,
				URL:           view.RepoURL,
				DeviceRelease: view.DeviceRelease,
				ChosenRelease: view.ChosenRelease,
				Arch:          view.Arch,
			})
		}
		for _, cat := range view.Categories {
			if !seenCat[cat.ID] {
				seenCat[cat.ID] = true
				categories = append(categories, cat)
			}
		}
		for _, pkg := range view.Packages {
			repoURL := pkg.RepoURL
			if repoURL == "" {
				repoURL = view.RepoURL
			}
			listings.add(pref.ID, repoURL, pkg)
		}
		if pref.ID == "oss" {
			oss = view
			haveOSS = true
		}
		if !havePrimary {
			built.primary = view
			havePrimary = true
		}
	}
	if !havePrimary {
		return built, false
	}
	if haveOSS {
		built.primary = oss
	}
	listed, variants, repos := listings.emit()
	built.primary.Categories = categories
	built.primary.Packages = listed
	built.primary.Recent = catalog.Recent(listed)
	built.primary.Fallback = len(built.notes) > 0
	built.primary.Updates = []string{}
	built.variants = variants
	built.repos = repos
	return built, true
}

func (a *App) commit(built builtCatalog) {
	a.setProgress(85)
	a.setStatus("Checking installed packages")
	installed, repos, updateErr := a.lookupInstalled(rpmNames(built.variants))
	built.primary.UpdateError = updateErr
	a.mu.Lock()
	a.view = built.primary
	a.pkgRepo = built.repos
	a.pkgVariants = built.variants
	a.catalogs = built.infos
	a.fallbacks = built.notes
	a.installed = installed
	a.enabledRepos = repos
	a.installedOK = updateErr == ""
	a.tried = map[string]bool{}
	a.recomputePreferredLocked()
	a.listColors = nil
	a.listLines = nil
	a.ready = true
	a.lastErr = ""
	a.catalogGen.Add(1)
	a.mu.Unlock()
	a.setProgress(100)
}

func (a *App) publishUnavailable(shown, returned string, infos []CatalogInfo) error {
	if infos == nil {
		infos = []CatalogInfo{}
	}
	a.mu.Lock()
	a.view = emptyView(a.release, a.arch)
	a.listColors = nil
	a.listLines = nil
	a.catalogs = infos
	a.fallbacks = []Fallback{}
	a.pkgRepo = map[string]string{}
	a.pkgVariants = map[string][]variant{}
	a.resetCatalogLocked()
	a.ready = true
	a.lastErr = shown
	a.mu.Unlock()
	a.setProgress(100)
	return fmt.Errorf("%s", returned)
}

// fetchRatings always updates the ratings clone. The bar follows git's own percent.
func (a *App) fetchRatings() error {
	a.setStatus("Fetching ratings")
	a.setProgress(0)
	return analysis.ForcePrepare(a.dataDir, func(label string, percent int) {
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		a.setProgress(percent)
		phase := "Fetching ratings"
		if label == "Cloning" {
			phase = "Cloning ratings"
		} else if label != "" && label != "Fetching" {
			phase = "Fetching ratings: " + label
		}
		if percent > 0 {
			a.setStatus(fmt.Sprintf("%s %d%%", phase, percent))
			return
		}
		a.setStatus(phase)
	})
}

func emptyView(release, arch string) catalog.View {
	return catalog.View{
		DeviceRelease: release,
		Arch:          arch,
		Categories:    []catalog.Cat{},
		Packages:      []catalog.Pkg{},
		Recent:        []string{},
		Updates:       []string{},
	}
}

func (a *App) loadCatalog(pref RepoPref) (catalog.View, error) {
	view, err := a.loadIndex(pref)
	if err == nil {
		return view, nil
	}
	if layout.Templated(pref.URL) {
		return a.loadResolved(pref)
	}
	// A scratch repo is rpm-md under aarch64, armv7hl, and i486, with no info.json.xz.
	if indexMissing(err) {
		view, rpmErr := a.loadFlatArch(pref)
		if rpmErr == nil {
			return view, nil
		}
	}
	return catalog.View{}, err
}

func indexMissing(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}

func (a *App) loadCachedCatalog(pref RepoPref) (catalog.View, error) {
	dir := filepath.Join(a.cacheDir, "catalogs", sanitizeID(pref.ID))
	idx, err := cache.ReadIndex(dir)
	if err != nil {
		return catalog.View{}, errNotCached
	}
	view, err := a.catalogFromBody(idx.Body)
	if err != nil {
		return catalog.View{}, errNotCached
	}
	return view, nil
}

func (a *App) loadIndex(pref RepoPref) (catalog.View, error) {
	dir := filepath.Join(a.cacheDir, "catalogs", sanitizeID(pref.ID))
	idx, err := cache.FetchIndex(a.client, a.indexFor(pref), dir, cache.ReadETag(dir), func(int) {})
	if err != nil {
		return catalog.View{}, err
	}
	return a.catalogFromBody(idx.Body)
}

func (a *App) catalogFromBody(body []byte) (catalog.View, error) {
	raw, err := decodeXZ(body)
	if err != nil {
		return catalog.View{}, err
	}
	doc, err := catalog.Parse(raw)
	if err != nil {
		return catalog.View{}, err
	}
	return catalog.Select(doc, a.release, a.arch)
}

// indexFor is the catalog index beside a repository.
// The OSS builtin keeps the configured index so a test server and --index still apply.
func (a *App) indexFor(pref RepoPref) string {
	if layout.Templated(pref.URL) && !(pref.ID == "oss" && a.indexURL != "") {
		return layout.Prefix(pref.URL) + "info.json.xz"
	}
	return indexURL(pref, a.indexURL)
}

func (a *App) fail(err error) error {
	a.mu.Lock()
	a.lastErr = err.Error()
	a.mu.Unlock()
	a.setProgress(100)
	return err
}

func rpmNames(vars map[string][]variant) []string {
	seen := map[string]bool{}
	var names []string
	for _, list := range vars {
		for _, item := range list {
			for _, rpm := range item.Pkg.RPMs {
				if rpm.Name == "" || seen[rpm.Name] {
					continue
				}
				seen[rpm.Name] = true
				names = append(names, rpm.Name)
			}
		}
	}
	return names
}

func (a *App) lookupInstalled(names []string) ([]catalog.Installed, []install.Repo, string) {
	sys, closeSys, err := a.bus()
	if err != nil {
		return nil, nil, err.Error()
	}
	defer closeSys()
	rows := []catalog.Installed{}
	if len(names) > 0 {
		found, err := sys.Installed(names)
		if err != nil {
			return nil, nil, err.Error()
		}
		rows = make([]catalog.Installed, 0, len(found))
		for _, row := range found {
			rows = append(rows, catalog.Installed{Name: row.Name, Version: row.Version})
		}
	}
	repos, err := sys.ListRepos()
	if err != nil {
		return rows, nil, err.Error()
	}
	if repos == nil {
		repos = []install.Repo{}
	}
	return rows, repos, ""
}

func (a *App) resetCatalogLocked() {
	a.preferred = map[string]string{}
	a.installed = nil
	a.installedOK = false
	a.enabledRepos = nil
	a.updateOffer = nil
	a.tried = map[string]bool{}
	a.catalogGen.Add(1)
}

// recomputePreferredLocked picks the newest source for each listing.
// A newer package version wins. Equal versions keep the later publication date.
// Remaining ties keep repository order.
// Updates use only a repository SSU already has enabled, so a newer build from
// a publisher who is not installed on the phone is not an update.
func (a *App) recomputePreferredLocked() {
	a.preferred = map[string]string{}
	a.updateOffer = map[string]variant{}
	for _, pkg := range a.view.Packages {
		rows := a.pkgVariants[pkg.ID]
		id := preferredVariant(rows)
		a.preferred[pkg.ID] = id
		if offer, ok := a.enabledOffer(rows); ok {
			a.updateOffer[pkg.ID] = offer
		}
	}
	if !a.installedOK {
		a.updateOffer = nil
		return
	}
	offered := make([]catalog.Pkg, 0, len(a.updateOffer))
	for id, row := range a.updateOffer {
		pkg := row.Pkg
		pkg.ID = id
		offered = append(offered, pkg)
	}
	a.view.Updates = catalog.Updates(offered, a.installed)
	kept := map[string]variant{}
	for _, id := range a.view.Updates {
		kept[id] = a.updateOffer[id]
	}
	a.updateOffer = kept
}

// enabledOffer is the newest package whose repository is already enabled.
func (a *App) enabledOffer(rows []variant) (variant, bool) {
	best := -1
	for i := range rows {
		if rows[i].RepoURL == "" || !install.RepoEnabled(a.enabledRepos, rows[i].RepoURL) {
			continue
		}
		if best < 0 || newerSource(rows[i].Pkg, rows[best].Pkg) {
			best = i
		}
	}
	if best < 0 {
		return variant{}, false
	}
	return rows[best], true
}

func preferredVariant(rows []variant) string {
	if len(rows) == 0 {
		return ""
	}
	best := 0
	for i := 1; i < len(rows); i++ {
		if newerSource(rows[i].Pkg, rows[best].Pkg) {
			best = i
		}
	}
	return rows[best].PrefID
}

// newerSource reports whether next should replace best.
// A higher RPM version wins. Equal versions keep the later publication date.
func newerSource(next, best catalog.Pkg) bool {
	if cmp := catalog.RPMVerCmp(sourceVersion(next), sourceVersion(best)); cmp != 0 {
		return cmp > 0
	}
	return publishedAt(next).After(publishedAt(best))
}

func publishedAt(pkg catalog.Pkg) time.Time {
	when, err := time.Parse(time.RFC3339, pkg.Updated)
	if err != nil {
		return time.Time{}
	}
	return when
}

func sourceVersion(pkg catalog.Pkg) string {
	for _, rpm := range catalog.PreferNewest(pkg.RPMs) {
		if !rpm.Install || rpm.Name == "" {
			continue
		}
		if rpm.Release == "" {
			return rpm.Version
		}
		if rpm.Version == "" {
			return rpm.Release
		}
		return rpm.Version + "-" + rpm.Release
	}
	return ""
}

func (a *App) kickPrepare() {
	if !a.prepareRatings {
		return
	}
	if !a.preparing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.preparing.Store(false)
		for {
			gen := a.catalogGen.Load()
			if err := analysis.Prepare(a.dataDir); err != nil {
				break
			}
			a.ensureAnalysis(a.catalogSHAs())
			if a.catalogGen.Load() == gen {
				break
			}
		}
		a.ratingsGen.Add(1)
	}()
}

// RatingsGeneration changes after each background ratings prepare finishes.
func (a *App) RatingsGeneration() int {
	return int(a.ratingsGen.Load())
}

func (a *App) bus() (install.System, func(), error) {
	if a.system != nil {
		return a.system, func() {}, nil
	}
	bus, err := install.Connect()
	if err != nil {
		return nil, nil, err
	}
	return bus, func() { bus.Close() }, nil
}

func (a *App) chosenRPMs(repoURL string, names []string) (string, []catalog.RPM) {
	a.mu.Lock()
	view := a.view
	variants := a.pkgVariants
	if repoURL == "" {
		repoURL = view.RepoURL
	}
	var pool []catalog.RPM
	if repoURL != "" {
		for _, list := range variants {
			for _, item := range list {
				if !install.SameRepo(item.RepoURL, repoURL) {
					continue
				}
				pool = append(pool, item.Pkg.RPMs...)
			}
		}
	}
	if len(pool) == 0 {
		for _, pkg := range view.Packages {
			pool = append(pool, pkg.RPMs...)
		}
	}
	a.mu.Unlock()
	return repoURL, catalog.MatchSpecs(pool, names)
}

// InstalledIDs returns PackageKit ids for the selected names that are already installed.
func (a *App) InstalledIDs(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("no packages selected")
	}
	sys, closeSys, err := a.bus()
	if err != nil {
		return nil, err
	}
	defer closeSys()
	found, err := sys.Installed(names)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, name := range names {
		want[name] = true
	}
	ids := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, row := range found {
		if !want[row.Name] || row.ID == "" || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("none of the selected packages are installed")
	}
	return ids, nil
}

// SystemRepos lists repositories SSU currently has enabled.
func (a *App) SystemRepos() ([]install.Repo, error) {
	sys, closeSys, err := a.bus()
	if err != nil {
		return nil, err
	}
	defer closeSys()
	return sys.ListRepos()
}

// SetRepoEnabled turns one rpm repository on or off. It does not touch the others.
func (a *App) SetRepoEnabled(repoURL string, on bool) error {
	sys, closeSys, err := a.bus()
	if err != nil {
		return err
	}
	defer closeSys()
	ctrl, ok := sys.(install.RepoSwitch)
	if !ok {
		return fmt.Errorf("repository enable is unavailable")
	}
	return install.SetRepoEnabled(ctrl, repoURL, on)
}

// Preview resolves the selected packages and asks PackageKit what installing
// them would change. Installation itself stays in the Qt process.
func (a *App) Preview(repoURL string, names []string) (install.Plan, error) {
	repoURL, chosen := a.chosenRPMs(repoURL, names)
	sys, closeSys, err := a.bus()
	if err != nil {
		return install.Plan{}, err
	}
	defer closeSys()
	ids, err := install.PackageIDs(sys, repoURL, chosen)
	if err != nil {
		return install.Plan{}, err
	}
	found, err := sys.Simulate(ids)
	if err != nil {
		return install.Plan{}, err
	}
	changes := install.Changes(found)
	if len(changes) == 0 {
		changes = install.ChangesFromIDs(ids)
	}
	return install.Plan{IDs: ids, Changes: changes}, nil
}

// PackageIDs adds the native repository if needed and returns PackageKit ids.
// Installation itself is PackageKit::Daemon::installPackages in the Qt process.
func (a *App) PackageIDs(repoURL string, names []string) ([]string, error) {
	repoURL, chosen := a.chosenRPMs(repoURL, names)
	sys, closeSys, err := a.bus()
	if err != nil {
		return nil, err
	}
	defer closeSys()
	return install.PackageIDs(sys, repoURL, chosen)
}

// Install adds the native repository if needed and installs the named RPMs.
func (a *App) Install(repoURL string, names []string) error {
	repoURL, chosen := a.chosenRPMs(repoURL, names)
	sys, closeSys, err := a.bus()
	if err != nil {
		return err
	}
	defer closeSys()
	return install.Packages(sys, repoURL, chosen)
}

// Source is one catalog that offers a package, so the page can switch.
type Source struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Enabled         bool           `json:"enabled"`
	RepoURL         string         `json:"repo_url"`
	Updated         string         `json:"updated"`
	Summary         string         `json:"summary"`
	Description     string         `json:"description"`
	IconPath        string         `json:"icon_path"`
	ScreenshotPaths []string       `json:"screenshot_paths"`
	Status          catalog.Status `json:"status"`
	RPMs            []catalog.RPM  `json:"rpms"`
	Version         string         `json:"version"`
}

type shown struct {
	catalog.Pkg
	IconPath        string   `json:"icon_path"`
	ScreenshotPaths []string `json:"screenshot_paths"`
	RepoURL         string   `json:"repo_url"`
	Source          string   `json:"source"`
	SourceEnabled   bool     `json:"source_enabled"`
	Sources         []Source `json:"sources"`
	PreferredSource string   `json:"preferred_source"`
}

type state struct {
	catalog.View
	Packages  []shown       `json:"packages"`
	Catalogs  []CatalogInfo `json:"catalogs"`
	Fallbacks []Fallback    `json:"fallbacks"`
	Error     string        `json:"error"`
	Progress  int           `json:"progress"`
	Ready     bool          `json:"ready"`
}

func (a *App) State() ([]byte, error) {
	if _, err := a.Settings(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	prefs := a.sourcePrefs()
	pkgs := make([]shown, 0, len(a.view.Packages))
	for _, pkg := range a.view.Packages {
		pkgs = append(pkgs, a.present(pkg, pkg.Screenshots, prefs))
	}
	body := state{
		View:      a.view,
		Packages:  pkgs,
		Catalogs:  a.catalogs,
		Fallbacks: a.fallbacks,
		Error:     a.lastErr,
		Progress:  a.Progress(),
		Ready:     a.ready,
	}
	if body.Catalogs == nil {
		body.Catalogs = []CatalogInfo{}
	}
	if body.Fallbacks == nil {
		body.Fallbacks = []Fallback{}
	}
	if body.Categories == nil {
		body.Categories = []catalog.Cat{}
	}
	if body.Recent == nil {
		body.Recent = []string{}
	}
	if body.Updates == nil {
		body.Updates = []string{}
	}
	return json.Marshal(body)
}

// listPkg is the row a listing shows. The package page loads the full record separately.
type listPkg struct {
	ID        string       `json:"id"`
	Summary   string       `json:"summary"`
	Category  string       `json:"category"`
	IconPath  string       `json:"icon_path"`
	Added     string       `json:"added"`
	Repo      string       `json:"repo"`
	Color     string       `json:"color"`
	Line      catalog.Line `json:"line"`
	Preferred string       `json:"preferred_source"`
	// Version, Installed, RepoURL, and Specs are set for packages that have an update.
	Version   string   `json:"version,omitempty"`
	Installed string   `json:"installed,omitempty"`
	RepoURL   string   `json:"repo_url,omitempty"`
	Specs     []string `json:"specs,omitempty"`
}

type listing struct {
	Categories  []catalog.Cat `json:"categories"`
	Packages    []listPkg     `json:"packages"`
	Recent      []string      `json:"recent"`
	Updates     []string      `json:"updates"`
	UpdateError string        `json:"update_error"`
	Catalogs    []CatalogInfo `json:"catalogs"`
	Fallbacks   []Fallback    `json:"fallbacks"`
	Error       string        `json:"error"`
	Progress    int           `json:"progress"`
	Ready       bool          `json:"ready"`
}

func listRepo(a *App, pkg catalog.Pkg, prefs []RepoPref) string {
	vars := a.pkgVariants[pkg.ID]
	want := ""
	if a.preferred != nil {
		want = a.preferred[pkg.ID]
	}
	for _, item := range vars {
		if want != "" && item.PrefID != want {
			continue
		}
		title, _ := sourceLabel(item.PrefID, item.RepoURL, prefs, a.catalogs)
		if title != "" {
			return title
		}
		if want != "" {
			break
		}
	}
	if pkg.Repo != "" {
		return pkg.Repo
	}
	return ""
}

// listingPackages scores each row from the source the package page shows first.
func (a *App) listingPackages() []catalog.Pkg {
	out := make([]catalog.Pkg, len(a.view.Packages))
	for i, pkg := range a.view.Packages {
		out[i] = pkg
		rows := a.pkgVariants[pkg.ID]
		chosen := variantByPref(rows, a.preferred[pkg.ID])
		if chosen == nil && len(rows) > 0 {
			chosen = &rows[0]
		}
		if chosen == nil {
			continue
		}
		cp := chosen.Pkg
		if len(cp.RPMs) == 0 && !catalog.StatusFilled(cp.Status) {
			continue
		}
		cp.ID = pkg.ID
		out[i] = cp
	}
	return out
}

// listRows builds the catalog rows shared by the main listing and a single repository.
// keep, when set, drops packages that do not belong in that view. Caller holds a.mu.
func (a *App) listRows(keep func(catalog.Pkg) bool) []listPkg {
	prefs := a.sourcePrefs()
	scored := a.listingPackages()
	if a.listColors == nil {
		a.listColors = catalog.ListingColors(scored)
		a.listLines = catalog.ListingLines(scored)
	}
	rows := make([]listPkg, 0, len(a.view.Packages))
	for _, pkg := range a.view.Packages {
		if keep != nil && !keep(pkg) {
			continue
		}
		color := a.shownColor(a.listColors[pkg.ID])
		line := a.listLines[pkg.ID]
		if line.Red == 0 && line.Yellow == 0 && line.Gray == 0 && line.Green == 0 {
			line = catalog.Line{Gray: 1}
		}
		item := listPkg{
			ID:        pkg.ID,
			Summary:   pkg.Summary,
			Category:  pkg.Category,
			IconPath:  pkg.Icon,
			Added:     pkg.Updated,
			Repo:      listRepo(a, pkg, prefs),
			Color:     color,
			Line:      line,
			Preferred: a.preferred[pkg.ID],
		}
		if src, ok := a.updateOffer[pkg.ID]; ok {
			chosen := src.Pkg
			chosen.ID = pkg.ID
			if from, to, specs, ok := catalog.UpdateChange(chosen, a.installed); ok {
				item.Installed = from
				item.Version = to
				item.Specs = specs
			}
			item.RepoURL = src.RepoURL
			if title, _ := sourceLabel(src.PrefID, src.RepoURL, prefs, a.catalogs); title != "" {
				item.Repo = title
			}
		}
		rows = append(rows, item)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Added != rows[j].Added {
			return rows[i].Added > rows[j].Added
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}

// Listing is the catalog summary used by the main page, categories, and package lists.
func (a *App) Listing() ([]byte, error) {
	if _, err := a.Settings(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := a.listRows(nil)
	body := listing{
		Categories:  a.view.Categories,
		Packages:    rows,
		Recent:      a.view.Recent,
		Updates:     a.view.Updates,
		UpdateError: a.view.UpdateError,
		Catalogs:    a.catalogs,
		Fallbacks:   a.fallbacks,
		Error:       a.lastErr,
		Progress:    a.Progress(),
		Ready:       a.ready,
	}
	if body.Categories == nil {
		body.Categories = []catalog.Cat{}
	}
	if body.Packages == nil {
		body.Packages = []listPkg{}
	}
	if body.Recent == nil {
		body.Recent = []string{}
	}
	if body.Updates == nil {
		body.Updates = []string{}
	}
	if body.Catalogs == nil {
		body.Catalogs = []CatalogInfo{}
	}
	if body.Fallbacks == nil {
		body.Fallbacks = []Fallback{}
	}
	return json.Marshal(body)
}

// RepoPackages lists catalog rows that this saved repository offers.
func (a *App) RepoPackages(prefID string) ([]byte, error) {
	if _, err := a.Settings(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := a.listRows(func(pkg catalog.Pkg) bool {
		return variantHasPref(a.pkgVariants[pkg.ID], prefID)
	})
	return json.Marshal(rows)
}

func variantHasPref(rows []variant, prefID string) bool {
	if prefID == "" {
		return false
	}
	for _, row := range rows {
		if row.PrefID == prefID {
			return true
		}
	}
	return false
}

// Package is one full catalog record for the package page.
func (a *App) Package(id string) ([]byte, error) {
	a.ensureAnalysis(a.shasFor(id, ""))
	if _, err := a.Settings(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var pkg catalog.Pkg
	found := false
	for _, item := range a.view.Packages {
		if item.ID == id {
			pkg = item
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("package %s was not found", id)
	}
	return json.Marshal(a.present(pkg, pkg.Screenshots, a.sourcePrefs()))
}

func (a *App) Search(query string) ([]byte, error) {
	if _, err := a.Settings(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	found := catalog.Search(a.view.Packages, query)
	prefs := a.sourcePrefs()
	pkgs := make([]shown, 0, len(found))
	for _, pkg := range found {
		pkgs = append(pkgs, a.present(pkg, pkg.Screenshots, prefs))
	}
	return json.Marshal(pkgs)
}

func (a *App) sourcePrefs() []RepoPref {
	return a.settings.Repositories
}

func variantByPref(rows []variant, id string) *variant {
	if id == "" {
		return nil
	}
	for i := range rows {
		if rows[i].PrefID == id {
			return &rows[i]
		}
	}
	return nil
}

func (a *App) present(pkg catalog.Pkg, shots []string, prefs []RepoPref) shown {
	vars := a.pkgVariants[pkg.ID]
	if len(vars) == 0 {
		vars = []variant{{RepoURL: a.pkgRepo[pkg.ID], Pkg: pkg}}
	}
	sources := make([]Source, 0, len(vars))
	for _, item := range vars {
		title, enabled := sourceLabel(item.PrefID, item.RepoURL, prefs, a.catalogs)
		shots := item.Pkg.Screenshots
		if shots == nil {
			shots = []string{}
		}
		rpms := a.markPending(catalog.WithColors(item.Pkg, item.Pkg.RPMs))
		sources = append(sources, Source{
			ID:              item.PrefID,
			Title:           title,
			Enabled:         enabled,
			RepoURL:         item.RepoURL,
			Updated:         item.Pkg.Updated,
			Summary:         item.Pkg.Summary,
			Description:     item.Pkg.Description,
			IconPath:        item.Pkg.Icon,
			ScreenshotPaths: shots,
			Status:          item.Pkg.Status,
			RPMs:            rpms,
			Version:         sourceVersion(item.Pkg),
		})
	}
	if sources[0].Title == "" {
		if pkg.Repo != "" {
			sources[0].Title = pkg.Repo
		} else {
			sources[0].Title = "Repository"
		}
	}
	first := sources[0]
	if len(shots) == 0 {
		shots = first.ScreenshotPaths
	}
	repoURL := first.RepoURL
	if repoURL == "" {
		repoURL = a.pkgRepo[pkg.ID]
	}
	pkg.RPMs = a.markPending(catalog.WithColors(pkg, pkg.RPMs))
	pref := ""
	if a.preferred != nil {
		pref = a.preferred[pkg.ID]
	}
	if pref == "" {
		pref = preferredVariant(vars)
	}
	return shown{
		Pkg:             pkg,
		IconPath:        pkg.Icon,
		ScreenshotPaths: shots,
		RepoURL:         repoURL,
		Source:          first.Title,
		SourceEnabled:   first.Enabled,
		Sources:         sources,
		PreferredSource: pref,
	}
}

func sourceLabel(id, repoURL string, prefs []RepoPref, catalogs []CatalogInfo) (string, bool) {
	title, enabled := sourceName(id, prefs, catalogs)
	if id != "openrepos" {
		return title, enabled
	}
	name, _, ok := openReposPublisher(repoURL)
	if !ok {
		return title, enabled
	}
	return "or/" + name, enabled
}

func sourceName(id string, prefs []RepoPref, catalogs []CatalogInfo) (string, bool) {
	for _, pref := range prefs {
		if pref.ID != id {
			continue
		}
		title := pref.Title
		if title == "" {
			title = pref.Host
		}
		return title, pref.Enabled
	}
	for _, info := range catalogs {
		if info.ID == id && info.Title != "" {
			return info.Title, false
		}
	}
	return id, false
}

func (a *App) Report(id, source string, specs []string) catalog.Report {
	a.ensureAnalysis(a.shasFor(id, source))
	a.mu.Lock()
	defer a.mu.Unlock()
	var preferred []catalog.Pkg
	for _, item := range a.pkgVariants[id] {
		if source != "" && item.PrefID != source {
			continue
		}
		pkg := item.Pkg
		pkg.ID = id
		preferred = append(preferred, pkg)
	}
	var pool []catalog.Pkg
	if len(a.pkgVariants) == 0 {
		pool = a.view.Packages
	} else {
		for listingID, vars := range a.pkgVariants {
			for _, item := range vars {
				pkg := item.Pkg
				pkg.ID = listingID
				pool = append(pool, pkg)
			}
		}
	}
	if len(pool) == 0 {
		pool = a.view.Packages
	}
	rep := catalog.ReportFor(pool, id, specs, preferred)
	if !rep.Sample {
		return rep
	}
	var selected []catalog.RPM
	for _, pkg := range preferred {
		selected = append(selected, catalog.MatchSpecs(pkg.RPMs, specs)...)
	}
	if a.confirmedMiss(selected) {
		return rep
	}
	rep.Sample = false
	rep.Color = catalog.ColorGray
	rep.Line = catalog.Line{Gray: 1}
	if len(rep.Packages) > 0 {
		rep.Packages[0].Line = rep.Line
		rep.Packages[0].Color = catalog.ColorGray
	}
	return rep
}

func (a *App) Dependencies(id, source string, names []string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	pkgs := a.view.Packages
	if source != "" {
		var selected []catalog.Pkg
		for _, item := range a.pkgVariants[id] {
			if item.PrefID != source {
				continue
			}
			pkg := item.Pkg
			pkg.ID = id
			selected = append(selected, pkg)
		}
		if len(selected) > 0 {
			pkgs = selected
		}
	}
	return catalog.Dependencies(pkgs, id, names)
}

const maxFetch = cache.MaxBytes

func readLimited(r io.Reader) ([]byte, error) {
	return cache.ReadLimited(r)
}

func decodeXZ(in []byte) ([]byte, error) {
	r, err := xz.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	return readLimited(r)
}
