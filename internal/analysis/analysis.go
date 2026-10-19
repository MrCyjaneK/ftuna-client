// Package analysis loads RPM ratings from a local bare clone of the ratings repository.
package analysis

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

// Version is the analysis suite this client writes expectations against.
// Ratings from version 6 are still read: their module scores become colors.
// A rating file from any other version is ignored.
const Version = 7

func acceptedVersion(v int) bool {
	return v == 6 || v == Version
}

// ErrNotFound means this digest has no rating in the cloned repository.
var ErrNotFound = fmt.Errorf("analysis not found")

// ErrNotReady means the ratings clone is missing or stale. Prepare fetches it.
var ErrNotReady = fmt.Errorf("analysis not ready")

// Result is one aa/bb/cc/<sha256>.json document.
type Result struct {
	Version    int    `json:"version"`
	SHA256     string `json:"sha256"`
	Name       string `json:"name"`
	RPMVersion string `json:"rpm_version"`
	Release    string `json:"release"`
	Arch       string `json:"arch"`
	// Verdict and Scores are version 6 fields. Version 7 does not publish them.
	Verdict *int          `json:"verdict,omitempty"`
	Scores  []ModuleScore `json:"scores,omitempty"`
	Issues  []Issue       `json:"issues"`
}

// ModuleScore is one scanner module. Neutral means that module did not apply.
type ModuleScore struct {
	Module string `json:"module"`
	Score  int    `json:"score"`
}

// Issue is one module finding. Color is red, yellow, green, or gray.
// Version 6 documents leave Color empty and carry it on Scores instead.
type Issue struct {
	Module string `json:"module"`
	Path   string `json:"path"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Color  string `json:"color,omitempty"`
}

// Load reads one rating from the bare clone kept in dataDir.
// It does not clone or fetch. A missing, stale, or busy clone returns ErrNotReady.
func Load(dataDir, sha256 string) (Result, error) {
	rel, err := blobRel(sha256)
	if err != nil {
		return Result{}, err
	}
	// A fetch or another read may already hold the repository. Waiting would
	// freeze the page that asked, so a busy repository is tried again later.
	if !repoMu.TryLock() {
		return Result{}, ErrNotReady
	}
	defer repoMu.Unlock()
	if !ratingsReady(dataDir) {
		return Result{}, ErrNotReady
	}
	body, err := cat(repoPath(dataDir), rel)
	if err != nil {
		return Result{}, err
	}
	doc, err := Decode(body)
	if err != nil {
		return Result{}, err
	}
	if !acceptedVersion(doc.Version) {
		return Result{}, ErrNotFound
	}
	return doc, nil
}

// LoadMany reads many ratings in one git process. Misses are digests the clone
// does not contain, or ratings from another analysis version. ErrNotReady means
// the clone is missing, stale, or in use, and nothing was marked missing.
func LoadMany(dataDir string, shas []string) (map[string]Result, []string, error) {
	rels := make([]string, 0, len(shas))
	order := make([]string, 0, len(shas))
	for _, sha := range shas {
		sum := Digest(sha)
		rel, err := blobRel(sum)
		if err != nil {
			continue
		}
		rels = append(rels, rel)
		order = append(order, sum)
	}
	if len(rels) == 0 {
		return map[string]Result{}, nil, nil
	}
	if !repoMu.TryLock() {
		return nil, nil, ErrNotReady
	}
	defer repoMu.Unlock()
	if !ratingsReady(dataDir) {
		return nil, nil, ErrNotReady
	}
	bodies, missing, err := catMany(repoPath(dataDir), rels)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]Result{}
	var misses []string
	for i, sha := range order {
		if missing[i] {
			misses = append(misses, sha)
			continue
		}
		doc, err := Decode(bodies[i])
		if err != nil || !acceptedVersion(doc.Version) {
			misses = append(misses, sha)
			continue
		}
		found[sha] = doc
	}
	return found, misses, nil
}

func ratingsReady(dataDir string) bool {
	return !preparing && cloneExists(dataDir) && freshFetch(dataDir)
}

// Digest returns sum as lowercase sha256 hex, or empty when it is not a digest.
func Digest(sum string) string {
	sum = strings.ToLower(strings.TrimSpace(sum))
	if len(sum) != 64 {
		return ""
	}
	for _, c := range sum {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return ""
		}
	}
	return sum
}

func blobRel(sha256 string) (string, error) {
	sum := Digest(sha256)
	if sum == "" {
		return "", fmt.Errorf("bad sha256")
	}
	return fmt.Sprintf("%s/%s/%s/%s.json", sum[:2], sum[2:4], sum[4:6], sum), nil
}

// Decode parses a rating JSON document.
func Decode(body []byte) (Result, error) {
	var doc Result
	if err := json.Unmarshal(body, &doc); err != nil {
		return Result{}, err
	}
	if doc.Issues == nil {
		doc.Issues = []Issue{}
	}
	return doc, nil
}

// StatusFromResult maps a rating into catalog.Status.
// Version 7 findings already carry a color. Version 6 module scores are
// turned into the same colors and then dropped. A document with no concerning
// finding is green.
func StatusFromResult(doc Result) catalog.Status {
	st := StatusFromIssues(doc.Issues)
	byModule := map[string]string{}
	for _, row := range doc.Scores {
		byModule[row.Module] = colorFromScore(row.Score)
	}
	for i := range st.Notes {
		if st.Notes[i].Color != "" {
			continue
		}
		if st.Notes[i].Code == "sailjail.permission" {
			st.Notes[i].Color = catalog.ColorGray
			continue
		}
		if color, ok := byModule[st.Notes[i].Module]; ok {
			st.Notes[i].Color = color
			continue
		}
		st.Notes[i].Color = catalog.ColorGray
	}
	st.Analyzed = true
	st.Color = catalog.LineColor(catalog.Status{Notes: st.Notes, Analyzed: true})
	return st
}

func colorFromScore(score int) string {
	if score < 0 {
		return catalog.ColorGray
	}
	if score >= 80 {
		return catalog.ColorGreen
	}
	if score >= 50 {
		return catalog.ColorYellow
	}
	return catalog.ColorRed
}

// StatusFromIssues maps scanner issues into catalog.Status.
func StatusFromIssues(issues []Issue) catalog.Status {
	st := catalog.Status{
		Setuid:      []catalog.Setuid{},
		Permissions: []string{},
		RedFlags:    []catalog.Flag{},
	}
	permSeen := map[string]bool{}
	setuidSeen := map[string]bool{}
	hookSeen := map[string]bool{}
	var sailAbsent, sailDisabled bool
	for _, issue := range issues {
		code := strings.TrimSpace(issue.Code)
		st.Notes = append(st.Notes, catalog.Note{
			Module: strings.TrimSpace(issue.Module),
			Code:   code,
			Path:   strings.TrimSpace(issue.Path),
			Detail: strings.TrimSpace(issue.Detail),
			Color:  strings.TrimSpace(issue.Color),
		})
		switch code {
		case "sailjail.absent":
			sailAbsent = true
			if st.Sailjail == "" {
				st.Sailjail = strings.TrimSpace(issue.Detail)
			}
		case "sailjail.disabled":
			sailDisabled = true
			st.Sailjail = "Disabled"
		case "sailjail.jailed":
			if st.Sailjail == "" {
				st.Sailjail = "sandboxed"
			}
		case "sailjail.permission":
			perm := strings.TrimSpace(issue.Detail)
			if perm != "" && !permSeen[perm] {
				permSeen[perm] = true
				st.Permissions = append(st.Permissions, perm)
			}
		case "setuid", "setgid":
			row, ok := setuidFromDetail(issue.Path, issue.Detail)
			if !ok {
				continue
			}
			key := row.Path + "\t" + strconv.Itoa(row.UID) + "\t" + strconv.Itoa(row.GID)
			if setuidSeen[key] {
				continue
			}
			setuidSeen[key] = true
			st.Setuid = append(st.Setuid, row)
		case "systemd.service":
			path := strings.TrimSpace(issue.Path)
			if path == "" {
				path = strings.TrimSpace(issue.Detail)
			}
			st.RedFlags = append(st.RedFlags, catalog.Flag{
				Title:       "systemd service",
				Description: path,
			})
		case "ftuna.built":
			st.BuiltInFTuna = true
			if strings.TrimSpace(issue.Detail) == "clean" {
				st.FTunaClean = true
			}
		case "ftuna.image":
			st.BuiltInFTuna = true
			st.FTunaClean = false
			for _, name := range strings.Split(issue.Detail, "\n") {
				name = strings.TrimSpace(name)
				if name == "" || hookSeen[name] {
					continue
				}
				hookSeen[name] = true
				st.ImageHooks = append(st.ImageHooks, name)
			}
		}
	}
	if len(st.ImageHooks) > 0 {
		st.FTunaClean = false
	}
	if sailDisabled {
		st.Sailjail = "Disabled"
	} else if len(st.Permissions) > 0 && st.Sailjail == "" {
		st.Sailjail = "sandboxed"
	} else if sailAbsent {
		// detail already set
	} else if len(issues) == 0 {
		st.Sailjail = "checked"
	}
	return st
}

func setuidFromDetail(path, detail string) (catalog.Setuid, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return catalog.Setuid{}, false
	}
	const mid = " mode "
	i := strings.Index(detail, mid)
	if i < 0 {
		return catalog.Setuid{Path: path}, true
	}
	userPart := detail[i+len(mid):]
	j := strings.Index(userPart, " user ")
	if j < 0 {
		return catalog.Setuid{Path: path}, true
	}
	modeStr := strings.TrimSpace(userPart[:j])
	userPart = userPart[j+len(" user "):]
	k := strings.Index(userPart, " group ")
	user := strings.TrimSpace(userPart)
	group := ""
	if k >= 0 {
		user = strings.TrimSpace(userPart[:k])
		group = strings.TrimSpace(userPart[k+len(" group "):])
	}
	uid := 1
	if user == "root" {
		uid = 0
	}
	gid := 1
	if group == "root" {
		gid = 0
	}
	_ = modeStr
	return catalog.Setuid{Path: path, UID: uid, GID: gid}, true
}
