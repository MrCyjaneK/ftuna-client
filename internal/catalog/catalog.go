// Package catalog reads an info.json document and picks the repository
// that matches the phone, or the closest older release.
package catalog

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const IndexURL = "https://repo-oss.ftuna.eu/info.json.xz"

// Doc is the info.json document published for a catalog.
type Doc struct {
	Version      int      `json:"version"`
	Updated      string   `json:"updated"`
	Categories   []string `json:"categories"`
	Repositories []Repo   `json:"repositories"`
	Packages     []Pkg    `json:"packages"`
}

type Repo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Pkg struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Updated     string   `json:"updated"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Screenshots []string `json:"screenshots"`
	Repo        string   `json:"repo"`
	RepoURL     string   `json:"repo_url,omitempty"`
	Status      Status   `json:"status"`
	RPMs        []RPM    `json:"rpms"`
}

type Status struct {
	Sailjail    string   `json:"sailjail"`
	Setuid      []Setuid `json:"setuid"`
	Permissions []string `json:"permissions"`
	RedFlags    []Flag   `json:"red_flags"`
	// BuiltInFTuna means this package was built by FTuna.
	BuiltInFTuna bool `json:"built_in_ftuna,omitempty"`
	// FTunaClean means that build had nothing in .sfosbuild/image.
	FTunaClean bool `json:"ftuna_clean,omitempty"`
	// ImageHooks are entry names from .sfosbuild/image.
	ImageHooks []string `json:"image_hooks,omitempty"`
	// Analyzed means a rating document was read. Color is that document's line:
	// the worst finding, or green when every finding is gray or there are none.
	Analyzed bool   `json:"-"`
	Color    string `json:"-"`
	// Notes are the scanner findings. They are the report lines.
	Notes []Note `json:"-"`
}

// Note is one published scanner finding. Color is red, yellow, green, or gray.
type Note struct {
	Module string
	Code   string
	Path   string
	Detail string
	Color  string
}

type Setuid struct {
	Path string `json:"path"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
}

type Flag struct {
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
}

type RPM struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Release  string   `json:"release"`
	Arch     string   `json:"arch"`
	Install  bool     `json:"install"`
	Requires []string `json:"requires"`
	// SHA256 is the digest of the RPM file. info.json carries it directly.
	// An rpm-md catalog fills it when the primary.xml checksum type is sha256.
	SHA256 string `json:"sha256,omitempty"`
	// Status is this RPM's own analysis. An empty status uses the package status.
	Status Status `json:"status,omitempty"`
	// Color is filled in when a package is shown. It is not part of info.json.
	Color string `json:"color,omitempty"`
	// Line counts findings by color for the segmented bar.
	Line Line `json:"line,omitempty"`
}

// View is the catalog after it has been limited to one release and arch.
type View struct {
	DeviceRelease string   `json:"device_release"`
	ChosenRelease string   `json:"chosen_release"`
	Arch          string   `json:"arch"`
	Fallback      bool     `json:"fallback"`
	RepoID        string   `json:"repo_id"`
	RepoURL       string   `json:"repo_url"`
	Categories    []Cat    `json:"categories"`
	Packages      []Pkg    `json:"packages"`
	Recent        []string `json:"recent"`
	Updates       []string `json:"updates"`
	UpdateError   string   `json:"update_error"`
}

type Cat struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Installed is a package PackageKit reports as already on the device.
type Installed struct {
	Name    string
	Version string
}

var versionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)

func Parse(raw []byte) (Doc, error) {
	var doc Doc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Doc{}, err
	}
	if doc.Categories == nil {
		doc.Categories = []string{}
	}
	if doc.Repositories == nil {
		doc.Repositories = []Repo{}
	}
	if doc.Packages == nil {
		doc.Packages = []Pkg{}
	}
	for i := range doc.Packages {
		p := &doc.Packages[i]
		if p.Screenshots == nil {
			p.Screenshots = []string{}
		}
		if p.Status.Setuid == nil {
			p.Status.Setuid = []Setuid{}
		}
		if p.Status.Permissions == nil {
			p.Status.Permissions = []string{}
		}
		if p.Status.RedFlags == nil {
			p.Status.RedFlags = []Flag{}
		}
		if p.RPMs == nil {
			p.RPMs = []RPM{}
		}
		for j := range p.RPMs {
			if p.RPMs[j].Requires == nil {
				p.RPMs[j].Requires = []string{}
			}
			p.RPMs[j].Install = checked(p.RPMs[j])
		}
	}
	return doc, nil
}

func checked(rpm RPM) bool {
	name := rpm.Name
	if strings.HasSuffix(name, "-debuginfo") || strings.HasSuffix(name, "-debugsource") || strings.HasSuffix(name, "-doc") || strings.HasSuffix(name, "-devel") {
		return false
	}
	return rpm.Install
}

// Select keeps packages for this phone.
// A release catalog such as FTuna OSS picks the matching release and arch, or the closest older release.
// A publisher catalog such as OpenRepos has no release in the URL. Every publisher stays, and packages are kept when an RPM matches arch or is noarch.
func Select(doc Doc, device, arch string) (View, error) {
	arch = NormalizeArch(arch)
	view := View{
		DeviceRelease: device,
		Arch:          arch,
		Categories:    labels(doc.Categories),
		Packages:      []Pkg{},
		Recent:        []string{},
		Updates:       []string{},
	}
	if releaseRepos(doc.Repositories) {
		return selectRelease(doc, device, arch, view)
	}
	return selectPublishers(doc, arch, view)
}

func releaseRepos(repos []Repo) bool {
	for _, repo := range repos {
		ver, repoArch, ok := SplitRepo(repo.URL)
		if ok && repoArch != "" && versionPattern.MatchString(ver) {
			return true
		}
	}
	return false
}

func selectRelease(doc Doc, device, arch string, view View) (View, error) {
	chosen, fallback, err := choose(doc.Repositories, device, arch)
	if err != nil {
		return view, err
	}
	view.ChosenRelease = chosen.version
	view.Fallback = fallback
	view.RepoID = chosen.repo.ID
	view.RepoURL = chosen.repo.URL
	for _, pkg := range doc.Packages {
		if pkg.Repo != chosen.repo.ID {
			continue
		}
		kept := make([]RPM, 0, len(pkg.RPMs))
		for _, rpm := range pkg.RPMs {
			if NormalizeArch(rpm.Arch) == arch {
				kept = append(kept, rpm)
			}
		}
		if len(kept) == 0 {
			continue
		}
		pkg.RPMs = kept
		pkg.RepoURL = chosen.repo.URL
		view.Packages = append(view.Packages, pkg)
	}
	sort.Slice(view.Packages, func(i, j int) bool {
		return view.Packages[i].ID < view.Packages[j].ID
	})
	view.Recent = Recent(view.Packages)
	return view, nil
}

func selectPublishers(doc Doc, arch string, view View) (View, error) {
	urls := map[string]string{}
	for _, repo := range doc.Repositories {
		urls[repo.ID] = repo.URL
	}
	for _, pkg := range doc.Packages {
		kept := make([]RPM, 0, len(pkg.RPMs))
		for _, rpm := range pkg.RPMs {
			got := NormalizeArch(rpm.Arch)
			if got == arch || got == "noarch" {
				kept = append(kept, rpm)
			}
		}
		if len(kept) == 0 {
			continue
		}
		pkg.RPMs = kept
		pkg.RepoURL = urls[pkg.Repo]
		view.Packages = append(view.Packages, pkg)
	}
	sort.Slice(view.Packages, func(i, j int) bool {
		return view.Packages[i].ID < view.Packages[j].ID
	})
	view.Recent = Recent(view.Packages)
	if len(view.Packages) == 0 {
		return view, fmt.Errorf("no %s packages", arch)
	}
	return view, nil
}

type candidate struct {
	repo    Repo
	version string
}

func choose(repos []Repo, device, arch string) (candidate, bool, error) {
	var exact *candidate
	var older *candidate
	for _, repo := range repos {
		ver, repoArch, ok := SplitRepo(repo.URL)
		if !ok || repoArch != arch || !versionPattern.MatchString(ver) {
			continue
		}
		c := candidate{repo: repo, version: ver}
		switch compareVersion(ver, device) {
		case 0:
			exact = &c
		case -1:
			if older == nil || compareVersion(ver, older.version) > 0 {
				copy := c
				older = &copy
			}
		}
	}
	if exact != nil {
		return *exact, false, nil
	}
	if older != nil {
		return *older, true, nil
	}
	return candidate{}, false, fmt.Errorf("no %s repository for %s or an older release", arch, device)
}

// SplitRepo reads the version and arch from a native rpm repository URL.
func SplitRepo(raw string) (version, arch string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	arch = NormalizeArch(parts[len(parts)-1])
	version = parts[len(parts)-2]
	if version == "" || arch == "" {
		return "", "", false
	}
	return version, arch, true
}

func compareVersion(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// NormalizeArch maps the kernel name onto the repository arch.
func NormalizeArch(arch string) string {
	switch arch {
	case "armv7l", "armv7hl":
		return "armv7hl"
	default:
		return arch
	}
}

func labels(ids []string) []Cat {
	if len(ids) == 0 {
		ids = []string{"application", "library"}
	}
	out := make([]Cat, 0, len(ids))
	for _, id := range ids {
		out = append(out, Cat{ID: id, Label: Label(id)})
	}
	return out
}

// Label is the name shown for a category id from the index.
func Label(id string) string {
	switch id {
	case "application":
		return "Application"
	case "library":
		return "Libraries"
	default:
		if id == "" {
			return ""
		}
		r := []rune(id)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		return string(r)
	}
}

const (
	ColorRed    = "red"
	ColorYellow = "yellow"
	ColorGreen  = "green"
	ColorGray   = "gray"
)

// Line counts findings for the bar under a listing.
// The bar draws red, then yellow, then gray, then green, each as wide as its count.
type Line struct {
	Red    int `json:"red,omitempty"`
	Yellow int `json:"yellow,omitempty"`
	Gray   int `json:"gray,omitempty"`
	Green  int `json:"green,omitempty"`
}

func (l Line) add(color string) Line {
	switch color {
	case ColorRed:
		l.Red++
	case ColorYellow:
		l.Yellow++
	case ColorGreen:
		l.Green++
	default:
		l.Gray++
	}
	return l
}

func (l Line) plus(other Line) Line {
	l.Red += other.Red
	l.Yellow += other.Yellow
	l.Gray += other.Gray
	l.Green += other.Green
	return l
}

func (l Line) empty() bool {
	return l.Red == 0 && l.Yellow == 0 && l.Gray == 0 && l.Green == 0
}

func oneColor(color string) Line {
	return Line{}.add(color)
}

// TallyOf counts each finding's own color. An analyzed package with no
// findings is one green segment. A package that has not been analyzed is gray.
func TallyOf(st Status) Line {
	if len(st.Notes) > 0 {
		var line Line
		for _, note := range st.Notes {
			color := note.Color
			if color == "" {
				color = ColorGray
			}
			line = line.add(color)
		}
		return line
	}
	if isSample(st) {
		return oneColor(ColorGray)
	}
	rows := statusFindings(st)
	if len(rows) == 0 {
		color := st.Color
		if color == "" {
			color = shade(legacyScore(st))
		}
		return oneColor(color)
	}
	var line Line
	for _, row := range rows {
		line = line.add(row.Color)
	}
	return line
}

// LineColor is the color shown for one status.
// A rating document uses its own color. Findings that already have a color
// use the worst of those, and gray findings alone stay green.
// A status that has not been analyzed is gray.
func LineColor(st Status) string {
	if st.Color != "" {
		return st.Color
	}
	if painted(st.Notes) {
		return worstNotes(st.Notes)
	}
	if isSample(st) {
		return ColorGray
	}
	return shade(legacyScore(st))
}

func painted(notes []Note) bool {
	for _, note := range notes {
		if note.Color != "" {
			return true
		}
	}
	return false
}

func worstNotes(notes []Note) string {
	worst := ""
	for _, note := range notes {
		if note.Color == "" || note.Color == ColorGray {
			continue
		}
		worst = WorseColor(worst, note.Color)
	}
	if worst == "" {
		return ColorGreen
	}
	return worst
}

// WorseColor keeps the more concerning color. Gray yields to every other color.
func WorseColor(a, b string) string {
	if concern(b) > concern(a) {
		return b
	}
	return a
}

func concern(color string) int {
	switch color {
	case ColorRed:
		return 4
	case ColorYellow:
		return 3
	case ColorGreen:
		return 2
	case ColorGray:
		return 1
	default:
		return 0
	}
}

func safety(color string) int {
	switch color {
	case ColorGreen:
		return 3
	case ColorYellow:
		return 2
	case ColorRed:
		return 1
	default:
		return 0
	}
}

func shade(score int) string {
	if score < 0 {
		return ColorGray
	}
	if score >= 80 {
		return ColorGreen
	}
	if score >= 50 {
		return ColorYellow
	}
	return ColorRed
}

// legacyScore is the old info.json penalty, used only when a status has no
// rating document. The number is not published. shade turns it into a color.
func legacyScore(st Status) int {
	penalty := 0
	jail := strings.TrimSpace(st.Sailjail)
	if jail != "" {
		switch sailjailPenalty(jail) {
		case 2:
			penalty += 20
		case 1:
			penalty += 10
		}
	}
	for _, row := range st.Setuid {
		if row.UID == 0 {
			penalty += 30
		} else {
			penalty += 15
		}
	}
	penalty += 25 * len(st.RedFlags)
	if len(st.ImageHooks) > 0 {
		penalty += 10
	}
	if penalty > 100 {
		return 0
	}
	return 100 - penalty
}

// EffectiveStatus is the RPM's own analysis, or the package analysis when this RPM has none yet.
func EffectiveStatus(pkg Pkg, rpm RPM) Status {
	if statusOwn(rpm.Status) {
		return rpm.Status
	}
	return pkg.Status
}

// WithColors attaches each RPM's own color. Older builds stay in the list and are unchecked.
func WithColors(pkg Pkg, rpms []RPM) []RPM {
	rpms = PreferNewest(rpms)
	for i := range rpms {
		st := EffectiveStatus(pkg, rpms[i])
		rpms[i].Color = LineColor(st)
		rpms[i].Line = TallyOf(st)
	}
	return rpms
}

// Finding is one line of a report.
type Finding struct {
	Color  string `json:"color"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Dependency is a require of the selected RPMs. Known is false when the catalog has no such package.
// ID is the catalog package that provides it, so the list can open that package.
type Dependency struct {
	Name  string `json:"name"`
	ID    string `json:"id"`
	Known bool   `json:"known"`
	Color string `json:"color"`
	Line  Line   `json:"line"`
}

// ReportPackage is one package on the findings page.
// Main is the package that was opened. Dependencies follow it.
type ReportPackage struct {
	Name     string    `json:"name"`
	ID       string    `json:"id"`
	Main     bool      `json:"main"`
	Known    bool      `json:"known"`
	Color    string    `json:"color"`
	Line     Line      `json:"line"`
	Findings []Finding `json:"findings"`
}

// Report is the analysis for the selected RPMs and the dependencies the catalog knows.
// Color is the opened package. A dependency color does not change it.
type Report struct {
	Color        string          `json:"color"`
	Line         Line            `json:"line"`
	Sample       bool            `json:"sample"`
	Packages     []ReportPackage `json:"packages"`
	Dependencies []Dependency    `json:"dependencies"`
}

// ReportFor lists the opened package first, then each known dependency.
// The opened package stays gray until it has been analyzed, even when a
// dependency already has a color.
func ReportFor(pkgs []Pkg, id string, specs []string, preferred []Pkg) Report {
	search := preferred
	if len(search) == 0 {
		search = pkgs
	}
	var selected []RPM
	var parent Pkg
	foundParent := false
	for _, pkg := range search {
		if pkg.ID != id {
			continue
		}
		got := MatchSpecs(pkg.RPMs, specs)
		if len(got) > len(selected) || !foundParent {
			selected = got
			parent = pkg
			foundParent = true
		}
	}
	if !foundParent || len(selected) == 0 {
		return Report{
			Color:        ColorGray,
			Line:         oneColor(ColorGray),
			Packages:     []ReportPackage{},
			Dependencies: []Dependency{},
		}
	}

	index := dependencyIndex(pkgs)
	chosen := map[string]bool{}
	for _, rpm := range selected {
		chosen[rpmIdentity(rpm)] = true
	}

	mainLine := packageLine(parent, selected)
	main := ReportPackage{
		Name:     parent.ID,
		ID:       parent.ID,
		Main:     true,
		Known:    !allSample(parent, selected),
		Color:    mainColor(parent, selected),
		Line:     mainLine,
		Findings: packageFindings(parent, selected),
	}
	packages := []ReportPackage{main}

	reqSeen := map[string]bool{}
	var deps []Dependency
	for _, rpm := range selected {
		for _, req := range rpm.Requires {
			if req == "" || reqSeen[req] {
				continue
			}
			reqSeen[req] = true
			dep := Dependency{Name: req, Color: ColorGray, Line: oneColor(ColorGray)}
			hit, color, ok := highestProvider(index[req])
			if ok {
				st := EffectiveStatus(hit.pkg, hit.rpm)
				dep.Known = color != ColorGray
				dep.ID = hit.pkg.ID
				dep.Color = color
				dep.Line = TallyOf(st)
				if !(chosen[rpmIdentity(hit.rpm)] && hit.pkg.ID == parent.ID) {
					packages = append(packages, ReportPackage{
						Name:     hit.pkg.ID,
						ID:       hit.pkg.ID,
						Known:    color != ColorGray,
						Color:    color,
						Line:     dep.Line,
						Findings: packageFindings(hit.pkg, []RPM{hit.rpm}),
					})
				}
			}
			deps = append(deps, dep)
		}
	}
	if deps == nil {
		deps = []Dependency{}
	}
	return Report{
		Color:        main.Color,
		Line:         mainLine,
		Sample:       !main.Known,
		Packages:     packages,
		Dependencies: deps,
	}
}

func packageLine(parent Pkg, selected []RPM) Line {
	if allSample(parent, selected) {
		return oneColor(ColorGray)
	}
	var line Line
	for _, rpm := range selected {
		st := EffectiveStatus(parent, rpm)
		if isSample(st) {
			continue
		}
		line = line.plus(TallyOf(st))
	}
	if line.empty() {
		return oneColor(ColorGray)
	}
	return line
}

func allSample(parent Pkg, selected []RPM) bool {
	for _, rpm := range selected {
		if !isSample(EffectiveStatus(parent, rpm)) {
			return false
		}
	}
	return true
}

func mainColor(parent Pkg, selected []RPM) string {
	if allSample(parent, selected) {
		return ColorGray
	}
	worst := ""
	for _, rpm := range selected {
		color := LineColor(EffectiveStatus(parent, rpm))
		if color == "" || color == ColorGray {
			continue
		}
		worst = WorseColor(worst, color)
	}
	if worst == "" {
		return ColorGray
	}
	return worst
}

func packageFindings(parent Pkg, selected []RPM) []Finding {
	var rows []Finding
	for _, rpm := range selected {
		rows = append(rows, statusFindings(EffectiveStatus(parent, rpm))...)
	}
	if rows == nil {
		rows = []Finding{}
	}
	return rows
}

// ListingColors is the line color of each package for the RPMs selected by default.
// A dependency color does not change the package line.
func ListingColors(pkgs []Pkg) map[string]string {
	out := make(map[string]string, len(pkgs))
	for _, pkg := range pkgs {
		var selected []RPM
		for _, rpm := range PreferNewest(pkg.RPMs) {
			if rpm.Install {
				selected = append(selected, rpm)
			}
		}
		if len(selected) == 0 {
			out[pkg.ID] = LineColor(pkg.Status)
			continue
		}
		out[pkg.ID] = mainColor(pkg, selected)
	}
	return out
}

// ListingLines is the segmented bar for each package's selected RPMs.
func ListingLines(pkgs []Pkg) map[string]Line {
	out := make(map[string]Line, len(pkgs))
	for _, pkg := range pkgs {
		var selected []RPM
		for _, rpm := range PreferNewest(pkg.RPMs) {
			if rpm.Install {
				selected = append(selected, rpm)
			}
		}
		if len(selected) == 0 {
			out[pkg.ID] = TallyOf(pkg.Status)
			continue
		}
		out[pkg.ID] = packageLine(pkg, selected)
	}
	return out
}

func dependencyIndex(pkgs []Pkg) map[string][]rpmHit {
	index := map[string][]rpmHit{}
	for _, pkg := range pkgs {
		for _, rpm := range Newest(pkg.RPMs) {
			if rpm.Name == "" {
				continue
			}
			index[rpm.Name] = append(index[rpm.Name], rpmHit{pkg: pkg, rpm: rpm})
		}
	}
	return index
}

type rpmHit struct {
	pkg Pkg
	rpm RPM
}

// highestProvider is the safest known color: green, then yellow, then red.
// Unknown does not outrank a known provider. Ties keep the earlier hit.
func highestProvider(hits []rpmHit) (rpmHit, string, bool) {
	if len(hits) == 0 {
		return rpmHit{}, "", false
	}
	best := hits[0]
	bestColor := LineColor(EffectiveStatus(best.pkg, best.rpm))
	picked := safety(bestColor) > 0
	for _, hit := range hits[1:] {
		color := LineColor(EffectiveStatus(hit.pkg, hit.rpm))
		if safety(color) == 0 {
			continue
		}
		if !picked || safety(color) > safety(bestColor) {
			best = hit
			bestColor = color
			picked = true
		}
	}
	return best, bestColor, true
}

func rpmIdentity(rpm RPM) string {
	return rpm.Name + "\t" + rpm.Version + "\t" + rpm.Release + "\t" + rpm.Arch
}

// StatusFilled reports whether status already came from the catalog or a prior analysis load.
func StatusFilled(st Status) bool {
	return statusOwn(st)
}

func statusOwn(st Status) bool {
	if st.Analyzed || st.BuiltInFTuna || len(st.ImageHooks) > 0 || len(st.Notes) > 0 {
		return true
	}
	jail := strings.TrimSpace(st.Sailjail)
	if jail != "" && !strings.EqualFold(jail, "unknown") {
		return true
	}
	return len(st.Setuid) > 0 || len(st.Permissions) > 0 || len(st.RedFlags) > 0
}

func isSample(st Status) bool {
	return !statusOwn(st)
}

func statusFindings(st Status) []Finding {
	if isSample(st) {
		return nil
	}
	if len(st.Notes) > 0 {
		return noteFindings(st.Notes)
	}
	var rows []Finding
	jail := strings.TrimSpace(st.Sailjail)
	if jail != "" {
		rows = append(rows, Finding{
			Color:  levelColor(sailjailPenalty(jail)),
			Title:  "Sailjail",
			Detail: jail,
		})
	}
	for _, row := range st.Setuid {
		color := ColorRed
		title := "setuid"
		if row.UID == 0 {
			title = "Runs as root"
		}
		rows = append(rows, Finding{
			Color:  color,
			Title:  title,
			Detail: row.Path + "  uid " + strconv.Itoa(row.UID) + "  gid " + strconv.Itoa(row.GID),
		})
	}
	for _, perm := range st.Permissions {
		rows = append(rows, Finding{
			Color:  ColorGray,
			Title:  "Permission",
			Detail: perm,
		})
	}
	for _, flag := range st.RedFlags {
		bits := []string{}
		if flag.Subtitle != "" {
			bits = append(bits, flag.Subtitle)
		}
		if flag.Description != "" {
			bits = append(bits, flag.Description)
		}
		title := flag.Title
		if title == "" {
			title = "Finding"
		}
		rows = append(rows, Finding{
			Color:  ColorRed,
			Title:  title,
			Detail: strings.Join(bits, " — "),
		})
	}
	if st.BuiltInFTuna {
		detail := "This package was built in FTuna."
		color := ColorGray
		if st.FTunaClean {
			detail = "The build was fully offline."
			color = ColorGreen
		}
		rows = append(rows, Finding{
			Color:  color,
			Title:  "Built in FTuna",
			Detail: detail,
		})
	}
	if len(st.ImageHooks) > 0 {
		detail := "The build wasn't performed fully offline, which means some dependencies weren't packaged properly.\n" + strings.Join(st.ImageHooks, "\n")
		rows = append(rows, Finding{
			Color:  ColorYellow,
			Title:  "Not fully offline",
			Detail: detail,
		})
	}
	return rows
}

func levelColor(level int) string {
	if level >= 2 {
		return ColorRed
	}
	if level == 1 {
		return ColorYellow
	}
	if level < 0 {
		return ColorGreen
	}
	return ColorGray
}

func noteFindings(notes []Note) []Finding {
	rows := make([]Finding, 0, len(notes))
	for _, note := range notes {
		color := note.Color
		if color == "" {
			color = ColorGray
		}
		rows = append(rows, Finding{
			Color:  color,
			Title:  noteTitle(note),
			Detail: noteDetail(note),
		})
	}
	return rows
}

func noteTitle(note Note) string {
	name := note.Code
	switch note.Code {
	case "sailjail.absent":
		name = "No Sailjail"
	case "sailjail.disabled":
		name = "Sailjail disabled"
	case "sailjail.jailed":
		name = "Sandboxed"
	case "sailjail.permission":
		name = "Permission"
	case "scriptlet":
		name = "Install script"
		if note.Path != "" {
			name += " (" + note.Path + ")"
		}
	case "systemd.service":
		name = "systemd service"
	case "systemd.timer":
		name = "systemd timer"
	case "systemd.socket":
		name = "systemd socket"
	case "systemd.path":
		name = "systemd path"
	case "setuid":
		name = "setuid"
		if strings.Contains(note.Detail, "user root") {
			name = "Runs as root"
		}
	case "setgid":
		name = "setgid"
	case "dbus.service":
		name = "D-Bus service"
	case "dbus.policy":
		name = "D-Bus policy"
	case "worldwrite":
		name = "World-writable"
	case "udev.run":
		name = "udev program"
	case "udev.mode":
		name = "udev device mode"
	case "polkit.pkla", "polkit.rules":
		name = "polkit rule"
	case "polkit.policy":
		name = "polkit action"
	case "hooks.boot":
		name = "Boot hook"
	case "hooks.generator":
		name = "systemd generator"
	case "hooks.linker":
		name = "Dynamic linker config"
	case "hooks.module":
		name = "Kernel module"
	case "hooks.pam":
		name = "PAM config"
	case "hooks.profile":
		name = "Shell profile"
	case "filecaps":
		name = "File capabilities"
	case "ftuna.built":
		name = "Built in FTuna"
	case "ftuna.image":
		name = "Not fully offline"
	case "rpm.read":
		name = "Could not read the RPM"
	default:
		if name == "" {
			name = note.Module
		}
		if name == "" {
			name = "Finding"
		}
	}
	return name
}

func noteDetail(note Note) string {
	switch note.Code {
	case "ftuna.built":
		if strings.TrimSpace(note.Detail) == "clean" {
			return "The build was fully offline."
		}
		return "This package was built in FTuna."
	case "ftuna.image":
		detail := "The build wasn't performed fully offline, which means some dependencies weren't packaged properly."
		if text := strings.TrimSpace(note.Detail); text != "" {
			detail += "\n" + text
		}
		return detail
	case "sailjail.permission":
		return strings.TrimSpace(note.Detail)
	case "scriptlet":
		return strings.TrimSpace(note.Detail)
	}
	parts := []string{}
	if path := strings.TrimSpace(note.Path); path != "" {
		parts = append(parts, path)
	}
	if text := strings.TrimSpace(note.Detail); text != "" && text != note.Path {
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func sailjailPenalty(text string) int {
	value := strings.ToLower(text)
	if value == "" || value == "unknown" {
		return 0
	}
	if strings.Contains(value, "not") || value == "no" || value == "disabled" || value == "off" {
		return 2
	}
	if strings.Contains(value, "sandbox") || value == "yes" || value == "on" || value == "checked" {
		return -1
	}
	return 1
}

// Recent lists application ids, newest update first.
func Recent(pkgs []Pkg) []string {
	apps := make([]Pkg, 0)
	for _, pkg := range pkgs {
		if pkg.Category == "application" || pkg.Icon != "" {
			apps = append(apps, pkg)
		}
	}
	sort.Slice(apps, func(i, j int) bool {
		return parseTime(apps[i].Updated).After(parseTime(apps[j].Updated))
	})
	ids := make([]string, 0, len(apps))
	for _, pkg := range apps {
		ids = append(ids, pkg.ID)
	}
	return ids
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// searchable drops a trailing AI-score image so the badge URL is not a search hit.
func searchable(description string) string {
	const marker = "<img src=\"https://raw.githubusercontent.com/sailfishos-chum/sailfishos-chum-gui/main/icons/theme/ai-score/"
	if i := strings.LastIndex(description, marker); i >= 0 {
		return strings.TrimRight(description[:i], "\n")
	}
	return description
}

// Search matches name, summary, and description.
func Search(pkgs []Pkg, query string) []Pkg {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return append([]Pkg{}, pkgs...)
	}
	out := []Pkg{}
	for _, pkg := range pkgs {
		if strings.Contains(strings.ToLower(pkg.ID), query) ||
			strings.Contains(strings.ToLower(pkg.Summary), query) ||
			strings.Contains(strings.ToLower(searchable(pkg.Description)), query) {
			out = append(out, pkg)
		}
	}
	return out
}

// SplitSpec reads one selected package. A bare name leaves exact false.
// The package page sends name, version, and release separated by tabs.
func SplitSpec(raw string) (name, version, release string, exact bool) {
	name, rest, ok := strings.Cut(raw, "\t")
	name = strings.TrimSpace(name)
	if !ok {
		return name, "", "", false
	}
	version, release, _ = strings.Cut(rest, "\t")
	return name, version, release, true
}

// PreferNewest lists every build. Within one name, the newest version is
// first and keeps its install flag. Older builds stay in the list with
// install cleared, so the page can offer them without selecting them.
func PreferNewest(rpms []RPM) []RPM {
	if len(rpms) == 0 {
		return []RPM{}
	}
	best := newestEVR(rpms)
	groups := map[string][]RPM{}
	var order []string
	for _, rpm := range rpms {
		if _, ok := groups[rpm.Name]; !ok {
			order = append(order, rpm.Name)
		}
		groups[rpm.Name] = append(groups[rpm.Name], rpm)
	}
	var out []RPM
	for _, name := range order {
		rows := append([]RPM(nil), groups[name]...)
		sort.SliceStable(rows, func(i, j int) bool {
			return rpmvercmp(rpmEVR(rows[i]), rpmEVR(rows[j])) > 0
		})
		for i := range rows {
			if rpmEVR(rows[i]) != best[name] {
				rows[i].Install = false
			}
		}
		out = append(out, rows...)
	}
	return out
}

// Newest keeps the newest build of each name. Builds that tie stay.
func Newest(rpms []RPM) []RPM {
	best := newestEVR(rpms)
	var out []RPM
	seen := map[string]bool{}
	for _, rpm := range rpms {
		if best[rpm.Name] != rpmEVR(rpm) {
			continue
		}
		key := rpmIdentity(rpm)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rpm)
	}
	if out == nil {
		out = []RPM{}
	}
	return out
}

func newestEVR(rpms []RPM) map[string]string {
	best := map[string]string{}
	for _, rpm := range rpms {
		evr := rpmEVR(rpm)
		if prev, ok := best[rpm.Name]; !ok || rpmvercmp(evr, prev) > 0 {
			best[rpm.Name] = evr
		}
	}
	return best
}

func rpmEVR(rpm RPM) string {
	if rpm.Release == "" {
		return rpm.Version
	}
	if rpm.Version == "" {
		return rpm.Release
	}
	return rpm.Version + "-" + rpm.Release
}

// MatchSpecs keeps the builds named by specs. A bare name keeps the newest
// build. A tab-separated name, version, and release keeps that build.
func MatchSpecs(pool []RPM, specs []string) []RPM {
	var out []RPM
	seen := map[string]bool{}
	var loose []string
	looseSeen := map[string]bool{}
	add := func(rpm RPM) {
		key := rpmIdentity(rpm)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, rpm)
	}
	for _, spec := range specs {
		name, version, release, exact := SplitSpec(spec)
		if name == "" {
			continue
		}
		if !exact {
			if !looseSeen[name] {
				looseSeen[name] = true
				loose = append(loose, name)
			}
			continue
		}
		for _, rpm := range pool {
			if rpm.Name == name && rpm.Version == version && rpm.Release == release {
				add(rpm)
			}
		}
	}
	if len(loose) > 0 {
		want := map[string]bool{}
		for _, name := range loose {
			want[name] = true
		}
		var matched []RPM
		for _, rpm := range pool {
			if want[rpm.Name] {
				matched = append(matched, rpm)
			}
		}
		for _, rpm := range Newest(matched) {
			add(rpm)
		}
	}
	if out == nil {
		out = []RPM{}
	}
	return out
}

// Dependencies lists the requires of the checked RPM specs.
func Dependencies(pkgs []Pkg, id string, specs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, pkg := range pkgs {
		if pkg.ID != id {
			continue
		}
		newest := map[string]bool{}
		for _, rpm := range Newest(pkg.RPMs) {
			newest[rpmIdentity(rpm)] = true
		}
		for _, rpm := range pkg.RPMs {
			if !specWanted(rpm, specs, newest) {
				continue
			}
			for _, req := range rpm.Requires {
				if req == "" || seen[req] {
					continue
				}
				seen[req] = true
				out = append(out, req)
			}
		}
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func specWanted(rpm RPM, specs []string, newest map[string]bool) bool {
	key := rpmIdentity(rpm)
	for _, spec := range specs {
		name, version, release, exact := SplitSpec(spec)
		if rpm.Name != name {
			continue
		}
		if !exact {
			return newest[key]
		}
		if rpm.Version == version && rpm.Release == release {
			return true
		}
	}
	return false
}

// Package returns the filtered package with id.
func Package(pkgs []Pkg, id string) (Pkg, bool) {
	for _, pkg := range pkgs {
		if pkg.ID == id {
			return pkg, true
		}
	}
	return Pkg{}, false
}

// UpdateChange is what an update would replace. from is the installed version,
// to is the newer build, and specs are the name, version, and release to install.
// ok is false when no installed RPM is older than this package.
func UpdateChange(pkg Pkg, installed []Installed) (from, to string, specs []string, ok bool) {
	have := map[string]string{}
	for _, row := range installed {
		prev, seen := have[row.Name]
		if !seen || rpmvercmp(row.Version, prev) > 0 {
			have[row.Name] = row.Version
		}
	}
	seen := map[string]bool{}
	var newer []RPM
	for _, rpm := range PreferNewest(pkg.RPMs) {
		if rpm.Name == "" || seen[rpm.Name] {
			continue
		}
		seen[rpm.Name] = true
		got, found := have[rpm.Name]
		if !found {
			continue
		}
		want := rpmEVR(rpm)
		if rpmvercmp(want, got) > 0 {
			newer = append(newer, rpm)
		}
	}
	if len(newer) == 0 {
		return "", "", nil, false
	}
	specs = make([]string, len(newer))
	for i, rpm := range newer {
		specs[i] = rpm.Name + "\t" + rpm.Version + "\t" + rpm.Release
	}
	return have[newer[0].Name], rpmEVR(newer[0]), specs, true
}

// Updates lists package ids whose indexed version is newer than an installed RPM.
func Updates(pkgs []Pkg, installed []Installed) []string {
	have := map[string]string{}
	for _, row := range installed {
		prev, ok := have[row.Name]
		if !ok || rpmvercmp(row.Version, prev) > 0 {
			have[row.Name] = row.Version
		}
	}
	var ids []string
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for _, rpm := range pkg.RPMs {
			got, ok := have[rpm.Name]
			if !ok {
				continue
			}
			want := rpm.Version + "-" + rpm.Release
			if rpmvercmp(want, got) > 0 && !seen[pkg.ID] {
				seen[pkg.ID] = true
				ids = append(ids, pkg.ID)
			}
		}
	}
	if ids == nil {
		ids = []string{}
	}
	return ids
}

// RPMVerCmp compares RPM version strings. It returns -1, 0, or 1.
func RPMVerCmp(a, b string) int { return rpmvercmp(a, b) }

// rpmvercmp compares RPM version strings. It returns -1, 0, or 1.
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for i < len(a) && !isalnum(a[i]) {
			i++
		}
		for j < len(b) && !isalnum(b[j]) {
			j++
		}
		if i >= len(a) || j >= len(b) {
			break
		}
		adigit := isdigit(a[i])
		bdigit := isdigit(b[j])
		if adigit != bdigit {
			if adigit {
				return 1
			}
			return -1
		}
		if adigit {
			for i < len(a) && a[i] == '0' {
				i++
			}
			for j < len(b) && b[j] == '0' {
				j++
			}
			is, js := i, j
			for i < len(a) && isdigit(a[i]) {
				i++
			}
			for j < len(b) && isdigit(b[j]) {
				j++
			}
			if i-is != j-js {
				if i-is > j-js {
					return 1
				}
				return -1
			}
			if c := strings.Compare(a[is:i], b[js:j]); c != 0 {
				return c
			}
			continue
		}
		is, js := i, j
		for i < len(a) && isalpha(a[i]) {
			i++
		}
		for j < len(b) && isalpha(b[j]) {
			j++
		}
		if c := strings.Compare(a[is:i], b[js:j]); c != 0 {
			return c
		}
	}
	if i >= len(a) && j >= len(b) {
		return 0
	}
	if i >= len(a) {
		return -1
	}
	return 1
}

func isalnum(b byte) bool { return isdigit(b) || isalpha(b) }
func isdigit(b byte) bool { return b >= '0' && b <= '9' }
func isalpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
