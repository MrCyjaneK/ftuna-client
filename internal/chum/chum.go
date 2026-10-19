// Package chum reads a Chum rpm-md tree. Chum publishes no info.json.xz.
package chum

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

var releaseDir = regexp.MustCompile(`^((?:[0-9]+\.)+[0-9]+)_(aarch64|armv7hl|i486)/?$`)

// Releases lists release directories from an HTML index, such as 4.4.0.64_aarch64.
func Releases(html string) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	for _, href := range hrefs(html) {
		name := strings.TrimSuffix(strings.TrimPrefix(href, "./"), "/")
		if strings.Contains(name, "/") || !releaseDir.MatchString(name) {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("repository index listed no repositories")
	}
	sort.Strings(names)
	return names, nil
}

func hrefs(html string) []string {
	var out []string
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
		out = append(out, rest[:j])
		rest = rest[j+1:]
	}
}

// Choose picks the release directory for this phone. fallback is set when the
// directory is an older release than the phone.
func Choose(dirs []string, device, arch string) (dir string, fallback bool, err error) {
	arch = catalog.NormalizeArch(arch)
	var exact, older, olderVer string
	for _, name := range dirs {
		ver, dirArch, ok := splitRelease(name)
		if !ok || catalog.NormalizeArch(dirArch) != arch {
			continue
		}
		switch catalog.RPMVerCmp(ver, device) {
		case 0:
			exact = name
		case -1:
			if older == "" || catalog.RPMVerCmp(ver, olderVer) > 0 {
				older = name
				olderVer = ver
			}
		}
	}
	if exact != "" {
		return exact, false, nil
	}
	if older != "" {
		return older, true, nil
	}
	return "", false, fmt.Errorf("no %s repository for %s or an older release", arch, device)
}

func splitRelease(name string) (version, arch string, ok bool) {
	m := releaseDir.FindStringSubmatch(strings.TrimSuffix(name, "/"))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// Version is the Sailfish release encoded in a directory name.
func Version(dir string) string {
	ver, _, ok := splitRelease(dir)
	if !ok {
		return dir
	}
	return ver
}

// PrimaryHref returns the primary.xml location from repomd.xml.
func PrimaryHref(repomd []byte) (string, error) {
	var doc struct {
		Data []struct {
			Type     string `xml:"type,attr"`
			Location struct {
				Href string `xml:"href,attr"`
			} `xml:"location"`
		} `xml:"data"`
	}
	if err := xml.Unmarshal(repomd, &doc); err != nil {
		return "", err
	}
	var href string
	for _, data := range doc.Data {
		if data.Type != "primary" || data.Location.Href == "" {
			continue
		}
		href = data.Location.Href
		if strings.HasSuffix(href, ".xml.gz") {
			return href, nil
		}
	}
	if href == "" {
		return "", fmt.Errorf("repomd has no primary metadata")
	}
	return href, nil
}

type primaryPkg struct {
	Name    string `xml:"name"`
	Arch    string `xml:"arch"`
	Version struct {
		Ver string `xml:"ver,attr"`
		Rel string `xml:"rel,attr"`
	} `xml:"version"`
	Summary     string `xml:"summary"`
	Description string `xml:"description"`
	Time        struct {
		Build int64 `xml:"build,attr"`
	} `xml:"time"`
	Checksum struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"checksum"`
	Format struct {
		SourceRPM string `xml:"sourcerpm"`
		Group     string `xml:"group"`
		Requires  struct {
			Entries []struct {
				Name string `xml:"name,attr"`
			} `xml:"entry"`
		} `xml:"requires"`
	} `xml:"format"`
}

// Packages groups primary.xml RPMs by source package.
func Packages(primary []byte, repoID, repoURL string) ([]catalog.Pkg, error) {
	var doc struct {
		Packages []primaryPkg `xml:"package"`
	}
	if err := xml.Unmarshal(primary, &doc); err != nil {
		return nil, err
	}
	groups := map[string][]primaryPkg{}
	var order []string
	for _, pkg := range doc.Packages {
		if pkg.Name == "" || pkg.Arch == "src" || pkg.Arch == "nosrc" {
			continue
		}
		id := sourceID(pkg)
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], pkg)
	}
	out := make([]catalog.Pkg, 0, len(order))
	for _, id := range order {
		out = append(out, packageOf(id, repoID, repoURL, groups[id]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func sourceID(pkg primaryPkg) string {
	name := sourceName(pkg.Format.SourceRPM)
	if name == "" {
		name = pkg.Name
	}
	return name
}

func sourceName(sourcerpm string) string {
	base := strings.TrimSuffix(sourcerpm, ".src.rpm")
	if base == sourcerpm || base == "" {
		return ""
	}
	i := strings.LastIndex(base, "-")
	if i <= 0 {
		return ""
	}
	base = base[:i]
	i = strings.LastIndex(base, "-")
	if i <= 0 {
		return ""
	}
	return base[:i]
}

func packageOf(id, repoID, repoURL string, rows []primaryPkg) catalog.Pkg {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	main := rows[0]
	for _, row := range rows {
		if row.Name == id {
			main = row
			break
		}
	}
	var newest time.Time
	category := packageCategory(rows)
	rpms := make([]catalog.RPM, 0, len(rows))
	for _, row := range rows {
		if row.Time.Build > 0 {
			when := time.Unix(row.Time.Build, 0).UTC()
			if when.After(newest) {
				newest = when
			}
		}
		rpms = append(rpms, catalog.RPM{
			Name:     row.Name,
			Version:  row.Version.Ver,
			Release:  row.Version.Rel,
			Arch:     row.Arch,
			Install:  installable(row.Name),
			Requires: cleanRequires(row),
			SHA256:   rpmSHA256(row),
		})
	}
	updated := ""
	if !newest.IsZero() {
		updated = newest.Format(time.RFC3339)
	}
	return catalog.Pkg{
		ID:          id,
		Category:    category,
		Updated:     updated,
		Summary:     strings.TrimSpace(main.Summary),
		Icon:        packageIcon(id, rows),
		Description: descriptionWithAI(strings.TrimSpace(main.Description)),
		Screenshots: []string{},
		Repo:        repoID,
		RepoURL:     repoURL,
		Status: catalog.Status{
			Sailjail:    "unknown",
			Setuid:      []catalog.Setuid{},
			Permissions: []string{},
			RedFlags:    []catalog.Flag{},
		},
		RPMs: rpms,
	}
}

func rpmSHA256(row primaryPkg) string {
	if !strings.EqualFold(strings.TrimSpace(row.Checksum.Type), "sha256") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(row.Checksum.Value))
}

func installable(name string) bool {
	suffixes := []string{"-debuginfo", "-debugsource", "-doc", "-devel"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	return true
}

func cleanRequires(row primaryPkg) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, entry := range row.Format.Requires.Entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" || strings.HasPrefix(name, "rpmlib(") || strings.HasPrefix(name, "/") || strings.Contains(name, "(") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
