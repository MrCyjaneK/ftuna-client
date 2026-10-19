package analysis

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

func TestStatusFromIssuesCamera(t *testing.T) {
	st := StatusFromIssues([]Issue{
		{Code: "sailjail.permission", Detail: "Audio"},
		{Code: "sailjail.permission", Detail: "Location"},
		{Code: "sailjail.permission", Detail: "Pictures"},
	})
	if st.Sailjail != "sandboxed" || len(st.Permissions) != 3 {
		t.Fatalf("%+v", st)
	}
	if catalog.LineColor(st) != catalog.ColorGreen {
		t.Fatalf("color %s", catalog.LineColor(st))
	}
}

func TestStatusFromIssuesClean(t *testing.T) {
	st := StatusFromIssues(nil)
	if st.Sailjail != "checked" || catalog.LineColor(st) != catalog.ColorGreen {
		t.Fatalf("%+v color %s", st, catalog.LineColor(st))
	}
}

func TestStatusFromIssuesSetuid(t *testing.T) {
	st := StatusFromIssues([]Issue{{
		Code:   "setuid",
		Path:   "/usr/bin/x",
		Detail: "/usr/bin/x mode 04755 user root group root",
	}})
	if len(st.Setuid) != 1 || st.Setuid[0].UID != 0 || st.Setuid[0].Path != "/usr/bin/x" {
		t.Fatalf("%+v", st.Setuid)
	}
}

func TestStatusFromIssuesFTuna(t *testing.T) {
	clean := StatusFromIssues([]Issue{{Code: "ftuna.built", Detail: "clean", Color: catalog.ColorGreen}})
	if !clean.BuiltInFTuna || !clean.FTunaClean || len(clean.ImageHooks) != 0 || catalog.LineColor(clean) != catalog.ColorGreen {
		t.Fatalf("%+v color %s", clean, catalog.LineColor(clean))
	}
	hooks := StatusFromIssues([]Issue{
		{Code: "ftuna.built", Detail: "hooks", Color: catalog.ColorRed},
		{Code: "ftuna.image", Detail: "23-rust.sh\n20-tdlib.sh\n20-tdlib.sh", Color: catalog.ColorRed},
	})
	if !hooks.BuiltInFTuna || hooks.FTunaClean || len(hooks.ImageHooks) != 2 || hooks.ImageHooks[0] != "23-rust.sh" {
		t.Fatalf("%+v", hooks)
	}
	if catalog.LineColor(hooks) != catalog.ColorRed {
		t.Fatalf("color %s", catalog.LineColor(hooks))
	}
}

func TestVersion6VerdictIgnoresNeutral(t *testing.T) {
	raw := []byte(`{
		"version": 6,
		"verdict": 80,
		"scores": [
			{"module": "cron", "score": -1},
			{"module": "sailjail", "score": 75},
			{"module": "systemd", "score": 85}
		],
		"issues": [
			{"module": "sailjail", "code": "sailjail.absent", "detail": "no [X-Sailjail] section"},
			{"module": "systemd", "code": "systemd.service", "path": "/usr/lib/systemd/user/app.service"}
		]
	}`)
	doc, err := Decode(raw)
	if err != nil || doc.Verdict == nil || *doc.Verdict != 80 {
		t.Fatalf("%+v %v", doc, err)
	}
	st := StatusFromResult(doc)
	if st.Color != catalog.ColorYellow || st.Sailjail == "" || len(st.RedFlags) != 1 {
		t.Fatalf("color %s status %+v", st.Color, st)
	}

	verdict := 30
	script := StatusFromResult(Result{
		Version: 6,
		Verdict: &verdict,
		Scores:  []ModuleScore{{Module: "scriptlet", Score: 30}},
		Issues: []Issue{{
			Module: "scriptlet",
			Path:   "%post",
			Code:   "scriptlet",
			Detail: "user=root program=/bin/sh body=systemctl enable app.service",
		}},
	})
	if script.Color != catalog.ColorRed || len(script.Notes) != 1 || script.Notes[0].Color != catalog.ColorRed || script.Notes[0].Path != "%post" {
		t.Fatalf("scriptlet %+v", script)
	}

	neutral := -1
	abstained := StatusFromResult(Result{Version: 6, Verdict: &neutral})
	if abstained.Color != catalog.ColorGreen {
		t.Fatalf("neutral verdict %s", abstained.Color)
	}
}

func TestBlobRel(t *testing.T) {
	sum := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	got, err := blobRel(sum)
	if err != nil || got != "ab/cd/ef/"+sum+".json" {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := blobRel("abc"); err == nil {
		t.Fatal("expected bad sha256")
	}
}

func TestLoadBareRepo(t *testing.T) {
	sum := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	other := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	origin := gitOrigin(t)
	commitRating(t, origin, sum, Result{
		Version: Version, SHA256: sum,
		Issues: []Issue{{Code: "sailjail.permission", Detail: "Audio"}},
	})
	prev := ratingsRemote
	ratingsRemote = "file://" + origin
	fetchedAt = time.Time{}
	t.Cleanup(func() {
		ratingsRemote = prev
		fetchedAt = time.Time{}
		preparing = false
	})

	dir := t.TempDir()
	if _, err := Load(dir, sum); !errors.Is(err, ErrNotReady) {
		t.Fatalf("load before prepare: %v", err)
	}
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(dir, sum)
	if err != nil || doc.Issues[0].Detail != "Audio" {
		t.Fatalf("%+v %v", doc, err)
	}
	found, misses, err := LoadMany(dir, []string{sum, other})
	if err != nil || found[sum].Issues[0].Detail != "Audio" || len(misses) != 1 || misses[0] != other {
		t.Fatalf("found %+v misses %v err %v", found, misses, err)
	}
	rel, err := blobRel(sum)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath(dir), rel)); !os.IsNotExist(err) {
		t.Fatal("rating was checked out into the work tree")
	}
	if _, err := Load(dir, other); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	commitRating(t, origin, other, Result{Version: Version, SHA256: other})
	fetchedAt = time.Time{}
	if err := os.WriteFile(fetchedPath(dir), []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, other); !errors.Is(err, ErrNotReady) {
		t.Fatalf("stale clone: %v", err)
	}
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	doc, err = Load(dir, other)
	if err != nil || doc.SHA256 != other {
		t.Fatalf("%+v %v", doc, err)
	}

	old := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	commitRating(t, origin, old, Result{Version: Version + 1, SHA256: old})
	fetchedAt = time.Time{}
	if err := os.WriteFile(fetchedPath(dir), []byte("0"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, old); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	fresh := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	commitRating(t, origin, fresh, Result{Version: Version, SHA256: fresh})
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, fresh); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fresh clone was fetched: %v", err)
	}
	var phases []string
	if err := ForcePrepare(dir, func(label string, percent int) {
		if percent < 0 || percent > 100 {
			t.Fatalf("progress %s %d", label, percent)
		}
		phases = append(phases, label)
	}); err != nil {
		t.Fatal(err)
	}
	if len(phases) == 0 {
		t.Fatal("force fetch reported no progress")
	}
	doc, err = Load(dir, fresh)
	if err != nil || doc.SHA256 != fresh {
		t.Fatalf("force fetch %+v %v", doc, err)
	}
}

func TestParseGitProgress(t *testing.T) {
	label, pct, ok := parseGitProgress("Receiving objects:  45% (25/55), 12.00 KiB | 1.00 MiB/s")
	if !ok || label != "Receiving objects" || pct != 45 {
		t.Fatalf("%q %d %v", label, pct, ok)
	}
	label, pct, ok = parseGitProgress("remote: Compressing objects: 100% (30/30), done.")
	if !ok || label != "Compressing objects" || pct != 100 {
		t.Fatalf("%q %d %v", label, pct, ok)
	}
	if _, _, ok := parseGitProgress("From /tmp/origin"); ok {
		t.Fatal("ref line is not progress")
	}
}

func gitOrigin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-b", "master")
	gitCmd(t, dir, "config", "user.email", "t@example.com")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func commitRating(t *testing.T, repo, sum string, doc Result) {
	t.Helper()
	rel, err := blobRel(sum)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "add", rel)
	gitCmd(t, repo, "commit", "-m", "rating")
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s", args, out)
	}
}
