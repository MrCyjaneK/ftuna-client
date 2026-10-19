package app

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
	"github.com/mrcyjanek/ftuna/client/internal/chum"
	"github.com/mrcyjanek/ftuna/client/internal/layout"
)

// loadResolved fills {version} and {arch} from the directory listing, then reads rpm-md.
// It tries the newest suitable release first and steps back when that directory has no metadata.
func (a *App) loadResolved(pref RepoPref) (catalog.View, error) {
	client := a.client
	if client == nil {
		client = http.DefaultClient
	}
	page, err := getBody(client, layout.Prefix(pref.URL))
	if err != nil {
		return catalog.View{}, err
	}
	hits, err := layout.Matches(pref.URL, a.release, a.arch, string(page))
	if err != nil {
		return catalog.View{}, err
	}
	if upstreamChum(pref.URL) {
		hits = layout.PreferChum(hits, a.release)
	}
	var last error
	for _, hit := range hits {
		view, err := a.readRepo(client, pref, hit)
		if err != nil {
			last = err
			continue
		}
		return view, nil
	}
	if last == nil {
		last = fmt.Errorf("no %s repository for %s or an older release", a.arch, a.release)
	}
	return catalog.View{}, last
}

func (a *App) readRepo(client *http.Client, pref RepoPref, hit layout.Hit) (catalog.View, error) {
	repomd, err := getBody(client, hit.URL+"repodata/repomd.xml")
	if err != nil {
		return catalog.View{}, err
	}
	href, err := chum.PrimaryHref(repomd)
	if err != nil {
		return catalog.View{}, err
	}
	raw, err := getBody(client, hit.URL+href)
	if err != nil {
		return catalog.View{}, err
	}
	if strings.HasSuffix(href, ".gz") {
		raw, err = gunzip(raw)
		if err != nil {
			return catalog.View{}, err
		}
	}
	repoID := sanitizeID(pref.ID + "-" + hit.Version + "-" + a.arch)
	pkgs, err := chum.Packages(raw, repoID, hit.URL)
	if err != nil {
		return catalog.View{}, err
	}
	if len(pkgs) == 0 {
		return catalog.View{}, fmt.Errorf("no %s packages", a.arch)
	}
	return catalog.View{
		DeviceRelease: a.release,
		ChosenRelease: hit.Version,
		Arch:          a.arch,
		Fallback:      hit.Fallback,
		RepoID:        repoID,
		RepoURL:       hit.URL,
		Categories: []catalog.Cat{
			{ID: "application", Label: "Application"},
			{ID: "library", Label: "Libraries"},
		},
		Packages: pkgs,
		Recent:   catalog.Recent(pkgs),
		Updates:  []string{},
	}, nil
}

// loadFlatArch reads <repo>/<arch>/repodata for a repository that has no info.json.xz.
func (a *App) loadFlatArch(pref RepoPref) (catalog.View, error) {
	client := a.client
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(pref.URL, "/")
	base = strings.TrimSuffix(base, "/info.json.xz")
	base = strings.TrimRight(base, "/") + "/"
	arch := catalog.NormalizeArch(a.arch)
	if arch == "" {
		return catalog.View{}, fmt.Errorf("no architecture")
	}
	return a.readRepo(client, pref, layout.Hit{URL: base + arch + "/"})
}

func upstreamChum(raw string) bool {
	return strings.Contains(raw, "://repo.sailfishos.org/obs/sailfishos:/chum/")
}

func getBody(client *http.Client, raw string) ([]byte, error) {
	resp, err := client.Get(raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", raw, resp.Status)
	}
	return body, nil
}

func gunzip(in []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return readLimited(r)
}
