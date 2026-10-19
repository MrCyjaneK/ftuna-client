// Package layout turns a repository URL pattern into a concrete rpm directory.
// Patterns use {version} and {arch}. The listing at the static prefix supplies
// the releases, and the phone keeps its own release or the closest older one.
package layout

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

var (
	versionRe = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)+$`)
	joinedRe  = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)+)_(aarch64|armv7hl|i486)$`)
)

// Hit is one release directory the phone can use.
type Hit struct {
	URL      string
	Version  string
	Fallback bool
}

// Templated reports whether the URL still has a version or arch placeholder.
func Templated(raw string) bool {
	return strings.Contains(raw, "{version}") || strings.Contains(raw, "{arch}")
}

// Prefix is the URL up to the first placeholder. A plain URL is returned with a trailing slash.
func Prefix(raw string) string {
	i := strings.Index(raw, "{")
	if i < 0 {
		return strings.TrimRight(raw, "/") + "/"
	}
	return raw[:i]
}

// Matches lists acceptable releases, newest first. fallback is set when the
// directory is older than the phone.
func Matches(pattern, device, arch, html string) ([]Hit, error) {
	prefix, kind, err := split(pattern)
	if err != nil {
		return nil, err
	}
	arch = catalog.NormalizeArch(arch)
	var hits []Hit
	for _, name := range dirNames(html) {
		ver, dirArch, ok := matchName(kind, name)
		if !ok {
			continue
		}
		if dirArch != "" && catalog.NormalizeArch(dirArch) != arch {
			continue
		}
		if catalog.RPMVerCmp(ver, device) > 0 {
			continue
		}
		hits = append(hits, Hit{
			URL:      concrete(prefix, kind, ver, arch),
			Version:  ver,
			Fallback: catalog.RPMVerCmp(ver, device) != 0,
		})
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("no %s repository for %s or an older release", arch, device)
	}
	sort.Slice(hits, func(i, j int) bool {
		return catalog.RPMVerCmp(hits[i].Version, hits[j].Version) > 0
	})
	return hits, nil
}

func split(pattern string) (prefix, kind string, err error) {
	i := strings.Index(pattern, "{")
	if i < 0 {
		return "", "", fmt.Errorf("repository URL has no version or arch")
	}
	prefix = pattern[:i]
	kind = strings.Trim(pattern[i:], "/")
	switch kind {
	case "{version}/{arch}", "{version}_{arch}":
		return prefix, kind, nil
	default:
		return "", "", fmt.Errorf("unsupported repository pattern %s", kind)
	}
}

func matchName(kind, name string) (version, arch string, ok bool) {
	switch kind {
	case "{version}/{arch}":
		if versionRe.MatchString(name) {
			return name, "", true
		}
	case "{version}_{arch}":
		m := joinedRe.FindStringSubmatch(name)
		if m != nil {
			return m[1], m[2], true
		}
	}
	return "", "", false
}

// PreferChum puts the directory SailfishOS:Chum enables first.
// From 4.6 that app uses the major.minor name, such as 5.1_aarch64.
// Older releases use the full version. That directory is the upstream
// repository for the phone, so it is not reported as a fallback.
func PreferChum(hits []Hit, device string) []Hit {
	if len(hits) == 0 {
		return hits
	}
	want := device
	if catalog.RPMVerCmp(device, "4.6") >= 0 {
		if mm, ok := majorMinor(device); ok {
			want = mm
		}
	}
	for i, hit := range hits {
		if hit.Version != want {
			continue
		}
		hit.Fallback = false
		out := make([]Hit, 0, len(hits))
		out = append(out, hit)
		out = append(out, hits[:i]...)
		out = append(out, hits[i+1:]...)
		return out
	}
	return hits
}

func majorMinor(version string) (string, bool) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "." + parts[1], true
}

func concrete(prefix, kind, version, arch string) string {
	if kind == "{version}/{arch}" {
		return prefix + version + "/" + arch + "/"
	}
	return prefix + version + "_" + arch + "/"
}

func dirNames(html string) []string {
	var out []string
	seen := map[string]bool{}
	rest := html
	for {
		i := strings.Index(rest, `href="`)
		if i < 0 {
			return out
		}
		rest = rest[i+6:]
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			return out
		}
		name := hrefName(rest[:j])
		rest = rest[j+1:]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
}

func hrefName(href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "?") || strings.HasPrefix(href, "#") {
		return ""
	}
	if strings.Contains(href, "://") {
		u, err := url.Parse(href)
		if err != nil {
			return ""
		}
		href = u.Path
	}
	href = strings.Trim(href, "/")
	if i := strings.LastIndex(href, "/"); i >= 0 {
		href = href[i+1:]
	}
	if href == "" || href == "." || href == ".." {
		return ""
	}
	return href
}
