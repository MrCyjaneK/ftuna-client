package app

import (
	"github.com/mrcyjanek/ftuna/client/internal/analysis"
	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

// ensureAnalysis loads ratings for these digests from the local clone.
// A package page asks for one package. After refresh, the listing asks for the catalog.
func (a *App) ensureAnalysis(shas []string) {
	if len(shas) == 0 {
		return
	}
	found, misses, err := analysis.LoadMany(a.dataDir, shas)
	if err != nil {
		return
	}
	st := make(map[string]catalog.Status, len(found))
	for sha, doc := range found {
		st[sha] = analysis.StatusFromResult(doc)
	}
	a.finishAnalysis(st, misses)
}

// catalogSHAs is every newest RPM digest the listing can score, skipping ones already answered.
func (a *App) catalogSHAs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	eachPackage(a.view.Packages, a.pkgVariants, func(pkg catalog.Pkg) {
		for _, rpm := range catalog.Newest(pkg.RPMs) {
			sum := normSHA(rpm.SHA256)
			if sum == "" || seen[sum] || catalog.StatusFilled(rpm.Status) || a.tried[sum] {
				continue
			}
			seen[sum] = true
			out = append(out, sum)
		}
	})
	return out
}

// finishAnalysis remembers which digests the server has answered.
// A 404 is a real miss. A network error is not, so the next open tries again.
func (a *App) finishAnalysis(st map[string]catalog.Status, misses []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tried == nil {
		a.tried = map[string]bool{}
	}
	mark := func(sha string) {
		if sum := normSHA(sha); sum != "" {
			a.tried[sum] = true
		}
	}
	for sha := range st {
		mark(sha)
	}
	for _, sha := range misses {
		mark(sha)
	}
	if len(st) == 0 && len(misses) == 0 {
		return
	}
	eachPackage(a.view.Packages, a.pkgVariants, func(pkg catalog.Pkg) {
		stampRPMs(pkg.RPMs, st)
	})
	a.listColors = nil
	a.listLines = nil
	a.recomputePreferredLocked()
}

// confirmedMiss is true when every selected install RPM was requested and the server had no analysis.
func (a *App) confirmedMiss(rpms []catalog.RPM) bool {
	saw := false
	for _, rpm := range rpms {
		if !rpm.Install {
			continue
		}
		if catalog.StatusFilled(rpm.Status) {
			return false
		}
		sum := normSHA(rpm.SHA256)
		if sum == "" || !a.tried[sum] {
			return false
		}
		saw = true
	}
	return saw
}

func (a *App) shownColor(color string) string {
	if color == catalog.ColorGreen || color == catalog.ColorYellow || color == catalog.ColorRed {
		return color
	}
	return catalog.ColorGray
}

func (a *App) markPending(rpms []catalog.RPM) []catalog.RPM {
	for i := range rpms {
		if rpms[i].Color == catalog.ColorGreen || rpms[i].Color == catalog.ColorYellow || rpms[i].Color == catalog.ColorRed {
			continue
		}
		rpms[i].Color = catalog.ColorGray
	}
	return rpms
}

func stampRPMs(rpms []catalog.RPM, st map[string]catalog.Status) {
	for i := range rpms {
		sum := normSHA(rpms[i].SHA256)
		if sum == "" || catalog.StatusFilled(rpms[i].Status) {
			continue
		}
		if got, ok := st[sum]; ok {
			rpms[i].Status = got
		}
	}
}

// shasFor is the opened package and the dependencies its report scores.
func (a *App) shasFor(id, source string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return collectSHAs(a.view.Packages, a.pkgVariants, id, source)
}

func collectSHAs(pkgs []catalog.Pkg, vars map[string][]variant, id, source string) []string {
	nameSHAs := map[string][]string{}
	filled := map[string]bool{}
	note := func(pkg catalog.Pkg) {
		for _, rpm := range catalog.Newest(pkg.RPMs) {
			sum := normSHA(rpm.SHA256)
			if rpm.Name == "" || sum == "" {
				continue
			}
			seenSum := false
			for _, got := range nameSHAs[rpm.Name] {
				if got == sum {
					seenSum = true
					break
				}
			}
			if !seenSum {
				nameSHAs[rpm.Name] = append(nameSHAs[rpm.Name], sum)
			}
			if catalog.StatusFilled(rpm.Status) {
				filled[sum] = true
			}
		}
	}
	eachPackage(pkgs, vars, note)

	var selected []catalog.Pkg
	if source != "" {
		for _, row := range vars[id] {
			if row.PrefID == source {
				selected = append(selected, row.Pkg)
			}
		}
	}
	if len(selected) == 0 {
		for _, pkg := range pkgs {
			if pkg.ID == id {
				selected = append(selected, pkg)
			}
		}
		for _, row := range vars[id] {
			selected = append(selected, row.Pkg)
		}
	}

	seen := map[string]bool{}
	var out []string
	add := func(sum string) {
		sum = normSHA(sum)
		if sum == "" || seen[sum] || filled[sum] {
			return
		}
		seen[sum] = true
		out = append(out, sum)
	}
	for _, pkg := range selected {
		for _, rpm := range pkg.RPMs {
			if !catalog.StatusFilled(rpm.Status) {
				add(rpm.SHA256)
			}
			for _, req := range rpm.Requires {
				for _, sum := range nameSHAs[req] {
					add(sum)
				}
			}
		}
	}
	return out
}

func normSHA(sum string) string {
	return analysis.Digest(sum)
}

func eachPackage(pkgs []catalog.Pkg, vars map[string][]variant, fn func(catalog.Pkg)) {
	for _, pkg := range pkgs {
		fn(pkg)
	}
	for _, rows := range vars {
		for _, row := range rows {
			fn(row.Pkg)
		}
	}
}
