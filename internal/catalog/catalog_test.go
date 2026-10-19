package catalog

import (
	"strings"
	"testing"
)

func TestSelectFallback(t *testing.T) {
	doc := Doc{
		Categories: []string{"application", "library"},
		Repositories: []Repo{
			{ID: "old", URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"},
			{ID: "other", URL: "https://repo-oss.ftuna.eu/5.1.0.11/armv7hl/"},
			{ID: "mid", URL: "https://repo-oss.ftuna.eu/5.2.0.9/aarch64/"},
		},
		Packages: []Pkg{
			pkg("camera", "application", "mid", "aarch64", true),
			pkg("gallery", "library", "old", "aarch64", false),
			pkg("arm", "application", "other", "armv7hl", true),
		},
	}
	view, err := Select(doc, "5.2.0.17", "aarch64")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Fallback || view.ChosenRelease != "5.2.0.9" || view.RepoID != "mid" {
		t.Fatalf("view %+v", view)
	}
	if len(view.Packages) != 1 || view.Packages[0].ID != "camera" {
		t.Fatalf("packages %+v", view.Packages)
	}
	if view.Categories[0].Label != "Application" || view.Categories[1].Label != "Libraries" {
		t.Fatalf("categories %+v", view.Categories)
	}
	if len(view.Recent) != 1 || view.Recent[0] != "camera" {
		t.Fatalf("recent %+v", view.Recent)
	}
}

func TestSelectExact(t *testing.T) {
	doc := Doc{
		Repositories: []Repo{
			{ID: "old", URL: "https://repo-oss.ftuna.eu/5.1.0.11/aarch64/"},
			{ID: "now", URL: "https://repo-oss.ftuna.eu/5.2.0.17/aarch64/"},
		},
		Packages: []Pkg{pkg("camera", "application", "now", "aarch64", true)},
	}
	view, err := Select(doc, "5.2.0.17", "aarch64")
	if err != nil {
		t.Fatal(err)
	}
	if view.Fallback || view.RepoID != "now" {
		t.Fatalf("view %+v", view)
	}
}

func TestSelectPublishers(t *testing.T) {
	arm := pkg("arm-only", "application", "openrepos-other", "armv7hl", false)
	both := pkg("harbour-demo", "application", "openrepos-piggz", "aarch64", true)
	both.RPMs = append(both.RPMs, RPM{Name: "harbour-demo", Version: "1", Release: "1", Arch: "armv7hl", Install: true})
	both.RPMs = append(both.RPMs, RPM{Name: "harbour-demo-data", Version: "1", Release: "1", Arch: "noarch", Install: true})
	doc := Doc{
		Categories: []string{"application", "library"},
		Repositories: []Repo{
			{ID: "openrepos-piggz", URL: "https://repo-openrepos.ftuna.eu/piggz/personal/main/"},
			{ID: "openrepos-other", URL: "https://repo-openrepos.ftuna.eu/other/personal/main/"},
		},
		Packages: []Pkg{arm, both},
	}
	view, err := Select(doc, "5.2.0.17", "aarch64")
	if err != nil {
		t.Fatal(err)
	}
	if view.Fallback || view.ChosenRelease != "" || view.RepoURL != "" {
		t.Fatalf("view %+v", view)
	}
	if len(view.Packages) != 1 || view.Packages[0].ID != "harbour-demo" {
		t.Fatalf("packages %+v", view.Packages)
	}
	got := view.Packages[0]
	if got.RepoURL != "https://repo-openrepos.ftuna.eu/piggz/personal/main/" || len(got.RPMs) != 2 {
		t.Fatalf("%+v", got)
	}
	for _, rpm := range got.RPMs {
		if rpm.Arch == "armv7hl" {
			t.Fatalf("kept %s", rpm.Arch)
		}
	}
}

func TestSelectNoneOlder(t *testing.T) {
	doc := Doc{
		Repositories: []Repo{{ID: "new", URL: "https://repo-oss.ftuna.eu/5.3.0.1/aarch64/"}},
	}
	_, err := Select(doc, "5.2.0.17", "aarch64")
	if err == nil {
		t.Fatal("expected no older release")
	}
}

func TestDebugUnchecked(t *testing.T) {
	raw := []byte(`{"version":1,"updated":"","categories":[],"repositories":[],"packages":[{"id":"a","category":"library","updated":"","summary":"","description":"","icon":"","screenshots":[],"repo":"r","status":{"sailjail":"unknown","setuid":[],"permissions":[],"red_flags":[]},"rpms":[
		{"name":"a","version":"1","release":"1","arch":"aarch64","install":true,"requires":["qt5-qtcore"],"sha256":"abc123"},
		{"name":"a-debuginfo","version":"1","release":"1","arch":"aarch64","install":true,"requires":[]},
		{"name":"a-doc","version":"1","release":"1","arch":"aarch64","install":true,"requires":[]},
		{"name":"a-devel","version":"1","release":"1","arch":"aarch64","install":true,"requires":[]}
	]}]}`)
	doc, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Packages[0].RPMs[0].Install || doc.Packages[0].RPMs[1].Install || doc.Packages[0].RPMs[2].Install || doc.Packages[0].RPMs[3].Install {
		t.Fatalf("%+v", doc.Packages[0].RPMs)
	}
	if doc.Packages[0].RPMs[0].SHA256 != "abc123" {
		t.Fatalf("sha256 %s", doc.Packages[0].RPMs[0].SHA256)
	}
}

func TestSearchAndDeps(t *testing.T) {
	pkgs := []Pkg{
		{
			ID: "harbour-advanced-camera", Summary: "Advanced camera", Description: "Better camera",
			RPMs: []RPM{
				{Name: "harbour-advanced-camera", Install: true, Requires: []string{"qt5-qtcore", "sailfishsilica-qt5"}},
				{Name: "harbour-advanced-camera-debuginfo", Install: false, Requires: []string{"qt5-qtcore"}},
			},
		},
		{ID: "libthing", Summary: "A library", Description: "helpers"},
	}
	got := Search(pkgs, "CAMERA")
	if len(got) != 1 || got[0].ID != "harbour-advanced-camera" {
		t.Fatalf("%+v", got)
	}
	badged := []Pkg{{
		ID: "libthing", Summary: "A library",
		Description: "helpers\n\n<img src=\"https://raw.githubusercontent.com/sailfishos-chum/sailfishos-chum-gui/main/icons/theme/ai-score/svgs/icons/AI-score-unknown.svg\">",
	}}
	if len(Search(badged, "githubusercontent")) != 0 || len(Search(badged, "helpers")) != 1 {
		t.Fatal(Search(badged, "helpers"))
	}
	deps := Dependencies(pkgs, "harbour-advanced-camera", []string{"harbour-advanced-camera"})
	if len(deps) != 2 || deps[0] != "qt5-qtcore" {
		t.Fatalf("%v", deps)
	}
}

func TestUpdates(t *testing.T) {
	pkgs := []Pkg{{
		ID:   "camera",
		RPMs: []RPM{{Name: "harbour-advanced-camera", Version: "0.9.5", Release: "1"}},
	}}
	ids := Updates(pkgs, []Installed{{Name: "harbour-advanced-camera", Version: "0.9.4-1"}})
	if len(ids) != 1 || ids[0] != "camera" {
		t.Fatalf("%v", ids)
	}
	ids = Updates(pkgs, []Installed{{Name: "harbour-advanced-camera", Version: "0.9.5-1"}})
	if len(ids) != 0 {
		t.Fatalf("same version listed: %v", ids)
	}
	from, to, specs, ok := UpdateChange(pkgs[0], []Installed{{Name: "harbour-advanced-camera", Version: "0.9.4-1"}})
	if !ok || from != "0.9.4-1" || to != "0.9.5-1" || len(specs) != 1 || specs[0] != "harbour-advanced-camera\t0.9.5\t1" {
		t.Fatalf("%q %q %v %v", from, to, specs, ok)
	}
	if _, _, _, ok := UpdateChange(pkgs[0], []Installed{{Name: "harbour-advanced-camera", Version: "0.9.5-1"}}); ok {
		t.Fatal("same version is not an update")
	}
}

func TestPreferNewestLeavesOlderBuilds(t *testing.T) {
	got := PreferNewest([]RPM{
		{Name: "harbour-demo", Version: "1.0", Release: "1", Arch: "aarch64", Install: true, Requires: []string{"old"}},
		{Name: "harbour-demo", Version: "1.10", Release: "1", Arch: "aarch64", Install: true, Requires: []string{"new"}},
		{Name: "harbour-demo", Version: "1.2", Release: "9", Arch: "aarch64", Install: true},
		{Name: "harbour-demo-data", Version: "1.0", Release: "1", Arch: "noarch", Install: true},
	})
	if len(got) != 4 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Version != "1.10" || !got[0].Install {
		t.Fatalf("newest %+v", got[0])
	}
	if got[1].Install || got[2].Install {
		t.Fatalf("older %+v %+v", got[1], got[2])
	}
	if got[3].Name != "harbour-demo-data" || !got[3].Install {
		t.Fatalf("data %+v", got[3])
	}
	specs := MatchSpecs(got, []string{"harbour-demo\t1.0\t1", "harbour-demo-data"})
	if len(specs) != 2 || specs[0].Version != "1.0" || specs[1].Name != "harbour-demo-data" {
		t.Fatalf("%+v", specs)
	}
	deps := Dependencies([]Pkg{{
		ID:   "demo",
		RPMs: got,
	}}, "demo", []string{"harbour-demo\t1.0\t1"})
	if len(deps) != 1 || deps[0] != "old" {
		t.Fatalf("deps %v", deps)
	}
}

func TestReportAveragesKnownDependency(t *testing.T) {
	app := Pkg{
		ID:     "app",
		Status: Status{Sailjail: "sandboxed"},
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Arch: "aarch64", Install: true,
			Requires: []string{"librisk", "libmissing"},
			Status:   Status{Sailjail: "sandboxed"},
		}, {
			Name: "app-data", Version: "1", Release: "1", Arch: "aarch64", Install: true,
			Status: Status{Sailjail: "no"},
		}},
	}
	risk := Pkg{
		ID: "librisk",
		RPMs: []RPM{{
			Name: "librisk", Version: "2", Release: "1",
			Status: Status{
				Sailjail: "no",
				Setuid:   []Setuid{{Path: "/usr/bin/x", UID: 0}},
			},
		}},
	}
	rep := ReportFor([]Pkg{app, risk}, "app", []string{"app\t1\t1", "app-data\t1\t1"}, nil)
	// The opened package stays green. librisk is yellow and does not change that.
	if rep.Color != ColorGreen || rep.Sample || len(rep.Packages) < 2 || !rep.Packages[0].Main {
		t.Fatalf("%+v", rep)
	}
	if rep.Line.Red == 0 || rep.Line.Green == 0 || rep.Packages[0].Line != rep.Line {
		t.Fatalf("line %+v", rep.Line)
	}
	sawRisk := false
	sawData := false
	for _, row := range rep.Packages[0].Findings {
		if row.Title == "Sailjail" && strings.Contains(row.Detail, "no") {
			sawData = true
		}
	}
	for _, pkg := range rep.Packages[1:] {
		if pkg.ID != "librisk" {
			continue
		}
		if pkg.Color != ColorYellow {
			t.Fatalf("librisk %+v", pkg)
		}
		for _, row := range pkg.Findings {
			if row.Title == "Runs as root" {
				sawRisk = true
			}
		}
	}
	if !sawRisk || !sawData {
		t.Fatalf("packages %+v", rep.Packages)
	}
	if len(rep.Dependencies) != 2 || rep.Dependencies[0].Name != "librisk" || rep.Dependencies[0].ID != "librisk" || !rep.Dependencies[0].Known || rep.Dependencies[0].Color != ColorYellow {
		t.Fatalf("deps %+v", rep.Dependencies)
	}
	if rep.Dependencies[1].Known {
		t.Fatalf("unknown dep %+v", rep.Dependencies[1])
	}
	if ListingColors([]Pkg{app, risk})["app"] != rep.Color {
		t.Fatalf("listing %s report %s", ListingColors([]Pkg{app, risk})["app"], rep.Color)
	}
	colored := WithColors(app, app.RPMs)
	if colored[0].Color != ColorGreen || colored[1].Color != ColorGreen {
		t.Fatalf("rpm colors %+v", colored)
	}
}

func TestReportPrefersHighestRatedDependency(t *testing.T) {
	app := Pkg{
		ID:     "app",
		Status: Status{Sailjail: "sandboxed"},
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Arch: "aarch64", Install: true,
			Requires: []string{"librisk"},
			Status:   Status{Sailjail: "sandboxed"},
		}},
	}
	poor := Pkg{
		ID: "librisk-poor",
		RPMs: []RPM{{
			Name: "librisk", Version: "1", Release: "1",
			Status: Status{
				Sailjail: "no",
				Setuid:   []Setuid{{Path: "/usr/bin/x", UID: 0}},
			},
		}},
	}
	good := Pkg{
		ID: "librisk-good",
		RPMs: []RPM{{
			Name: "librisk", Version: "1", Release: "1",
			Status: Status{Sailjail: "sandboxed"},
		}},
	}
	rep := ReportFor([]Pkg{app, poor, good}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGreen {
		t.Fatalf("%+v", rep)
	}
	if len(rep.Dependencies) != 1 || rep.Dependencies[0].Color != ColorGreen || rep.Dependencies[0].ID != "librisk-good" {
		t.Fatalf("deps %+v", rep.Dependencies)
	}
	for _, pkg := range rep.Packages {
		for _, row := range pkg.Findings {
			if row.Title == "Runs as root" {
				t.Fatalf("kept the weaker provider %+v", rep.Packages)
			}
		}
	}
	if ListingColors([]Pkg{app, poor, good})["app"] != ColorGreen {
		t.Fatalf("listing %s", ListingColors([]Pkg{app, poor, good})["app"])
	}
}

func TestReportSampleStays41WithoutDependencies(t *testing.T) {
	app := Pkg{
		ID:     "app",
		Status: Status{Sailjail: "unknown"},
		RPMs: []RPM{
			{Name: "app", Version: "1", Release: "1", Install: true, Requires: []string{"missing"}},
			{Name: "app-extra", Version: "1", Release: "1", Install: true},
		},
	}
	rep := ReportFor([]Pkg{app}, "app", []string{"app\t1\t1", "app-extra\t1\t1"}, nil)
	if rep.Color != ColorGray || !rep.Sample || len(rep.Packages) != 1 || len(rep.Packages[0].Findings) != 0 {
		t.Fatalf("%+v", rep)
	}
}

func TestReportFTunaOffline(t *testing.T) {
	app := Pkg{
		ID: "app",
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Arch: "aarch64", Install: true,
			Status: Status{
				Sailjail:     "sandboxed",
				BuiltInFTuna: true,
				ImageHooks:   []string{"20-tdlib.sh", "23-rust.sh"},
			},
		}},
	}
	rep := ReportFor([]Pkg{app}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGreen {
		t.Fatalf("%+v", rep)
	}
	var built, offline bool
	for _, row := range rep.Packages[0].Findings {
		if row.Title == "Built in FTuna" && row.Color == ColorGray && strings.Contains(row.Detail, "built in FTuna") {
			built = true
		}
		if row.Title == "Not fully offline" && row.Color == ColorYellow &&
			strings.Contains(row.Detail, "wasn't performed fully offline") &&
			strings.Contains(row.Detail, "some dependencies weren't packaged properly") &&
			strings.Contains(row.Detail, "20-tdlib.sh\n23-rust.sh") {
			offline = true
		}
	}
	if !built || !offline {
		t.Fatalf("findings %+v", rep.Packages)
	}

	clean := app
	clean.RPMs = []RPM{{
		Name: "app", Version: "1", Release: "1", Arch: "aarch64", Install: true,
		Status: Status{Sailjail: "sandboxed", BuiltInFTuna: true, FTunaClean: true},
	}}
	rep = ReportFor([]Pkg{clean}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGreen {
		t.Fatalf("clean %+v", rep)
	}
	sawClean := false
	for _, row := range rep.Packages[0].Findings {
		if row.Title == "Built in FTuna" && strings.Contains(row.Detail, "fully offline") && row.Color == ColorGreen {
			sawClean = true
		}
		if row.Title == "Not fully offline" {
			t.Fatalf("clean finding %+v", row)
		}
	}
	if !sawClean {
		t.Fatalf("findings %+v", rep.Packages)
	}
}

func TestReportAveragesDependencies(t *testing.T) {
	app := Pkg{
		ID: "app",
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Install: true,
			Requires: []string{"liblow", "libmid"},
			Status:   Status{Analyzed: true, Color: ColorGreen},
		}},
	}
	low := Pkg{
		ID: "liblow",
		RPMs: []RPM{{
			Name: "liblow", Version: "1", Release: "1",
			Status: Status{Analyzed: true, Color: ColorRed},
		}},
	}
	mid := Pkg{
		ID: "libmid",
		RPMs: []RPM{{
			Name: "libmid", Version: "1", Release: "1",
			Status: Status{Analyzed: true, Color: ColorGreen},
		}},
	}
	pkgs := []Pkg{app, low, mid}
	rep := ReportFor(pkgs, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGreen || len(rep.Packages) != 3 {
		t.Fatalf("%+v", rep)
	}
	if rep.Packages[1].Color != ColorRed || rep.Packages[2].Color != ColorGreen {
		t.Fatalf("deps %+v", rep.Packages)
	}
	if ListingColors(pkgs)["app"] != ColorGreen {
		t.Fatalf("listing %s", ListingColors(pkgs)["app"])
	}
}

func TestReportRoundsWeightedAverage(t *testing.T) {
	app := Pkg{
		ID: "app",
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Install: true,
			Requires: []string{"lib"},
			Status:   Status{Analyzed: true, Color: ColorGreen},
		}},
	}
	lib := Pkg{
		ID: "lib",
		RPMs: []RPM{{
			Name: "lib", Version: "1", Release: "1",
			Status: Status{Analyzed: true, Color: ColorGreen},
		}},
	}
	rep := ReportFor([]Pkg{app, lib}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGreen || len(rep.Packages) != 2 || rep.Packages[1].Color != ColorGreen {
		t.Fatalf("%+v", rep)
	}
}

func TestReportSelectedRPMIsNotADependency(t *testing.T) {
	app := Pkg{
		ID: "app",
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Install: true,
			Requires: []string{"app-data", "liblow"},
			Status:   Status{Analyzed: true, Color: ColorGreen},
		}, {
			Name: "app-data", Version: "1", Release: "1", Install: true,
			Status: Status{Analyzed: true, Color: ColorGreen},
		}},
	}
	low := Pkg{
		ID: "liblow",
		RPMs: []RPM{{
			Name: "liblow", Version: "1", Release: "1",
			Status: Status{Analyzed: true, Color: ColorRed},
		}},
	}
	pkgs := []Pkg{app, low}
	rep := ReportFor(pkgs, "app", []string{"app\t1\t1", "app-data\t1\t1"}, nil)
	if rep.Color != ColorGreen || len(rep.Packages) != 2 || rep.Packages[1].ID != "liblow" || rep.Packages[1].Color != ColorRed {
		t.Fatalf("%+v", rep)
	}
	if ListingColors(pkgs)["app"] != ColorGreen {
		t.Fatalf("listing %s", ListingColors(pkgs)["app"])
	}
}

func TestReportUnscoredAppIgnoresDependencyScores(t *testing.T) {
	app := Pkg{
		ID:     "app",
		Status: Status{Sailjail: "unknown"},
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Install: true,
			Requires: []string{"lib"},
		}},
	}
	lib := Pkg{
		ID: "lib",
		RPMs: []RPM{{
			Name: "lib", Version: "1", Release: "1",
			Status: Status{Analyzed: true, Color: ColorRed},
		}},
	}
	rep := ReportFor([]Pkg{app, lib}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorGray || !rep.Sample {
		t.Fatalf("%+v", rep)
	}
	if len(rep.Packages) != 2 || rep.Packages[1].Color != ColorRed {
		t.Fatalf("dep still listed %+v", rep.Packages)
	}
}

func TestLineColor(t *testing.T) {
	if got := LineColor(Status{Sailjail: "unknown"}); got != ColorGray {
		t.Fatalf("sample %s", got)
	}
	allowed := LineColor(Status{Sailjail: "sandboxed", Permissions: []string{"Internet", "Location"}})
	if allowed != ColorGreen {
		t.Fatalf("sailjail permissions %s", allowed)
	}
	safe := LineColor(Status{Sailjail: "sandboxed"})
	if safe != ColorGreen {
		t.Fatalf("sandboxed %s", safe)
	}
	risky := LineColor(Status{Sailjail: "no", Setuid: []Setuid{{Path: "/usr/bin/x", UID: 0}}})
	if risky != ColorYellow {
		t.Fatalf("risky %s", risky)
	}
}

func TestReportExplainsScriptletScore(t *testing.T) {
	app := Pkg{
		ID: "app",
		RPMs: []RPM{{
			Name: "app", Version: "1", Release: "1", Arch: "aarch64", Install: true,
			Status: Status{
				Analyzed: true,
				Color:    ColorRed,
				Notes: []Note{{
					Module: "scriptlet",
					Code:   "scriptlet",
					Path:   "%post",
					Detail: "user=root program=/bin/sh body=systemctl enable app.service",
					Color:  ColorRed,
				}},
			},
		}},
	}
	rep := ReportFor([]Pkg{app}, "app", []string{"app\t1\t1"}, nil)
	if rep.Color != ColorRed {
		t.Fatalf("%+v", rep)
	}
	got := rep.Packages[0].Findings
	if len(got) != 1 || got[0].Title != "Install script (%post)" || got[0].Color != ColorRed || !strings.Contains(got[0].Detail, "systemctl enable") {
		t.Fatalf("findings %+v", got)
	}
}

func TestRPMVerCmp(t *testing.T) {
	if rpmvercmp("1.0", "1.0") != 0 || rpmvercmp("1.1", "1.0") <= 0 || rpmvercmp("0.9.5-1", "0.9.4-1") <= 0 {
		t.Fatal("rpmvercmp")
	}
}

func pkg(id, category, repo, arch string, icon bool) Pkg {
	iconURL := ""
	if icon {
		iconURL = "https://cdn-mirror.ftuna.eu/raw.githubusercontent.com/ex/app/icon.svg"
	}
	return Pkg{
		ID: id, Category: category, Updated: "2026-09-28T12:00:00Z", Repo: repo, Icon: iconURL,
		Screenshots: []string{},
		Status:      Status{Sailjail: "unknown", Setuid: []Setuid{}, Permissions: []string{}, RedFlags: []Flag{}},
		RPMs:        []RPM{{Name: id, Version: "1", Release: "1", Arch: arch, Install: true, Requires: []string{}}},
	}
}
