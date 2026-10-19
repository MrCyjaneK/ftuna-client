// Package cache stores the catalog index and CDN images under the app cache.
package cache

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// MaxBytes is the most an index or image download will keep.
const MaxBytes int64 = 32 << 32

// Index is the on-disk copy of info.json.xz and the ETag it was fetched with.
type Index struct {
	Body []byte
	ETag string
	New  bool
}

// FetchIndex HEADs indexURL and downloads the body when the ETag changed.
// progress is called with a value from 0 to 100 while the body downloads.
func FetchIndex(client *http.Client, indexURL, dir, previousETag string, progress func(int)) (Index, error) {
	client = OrDefault(client)
	if progress == nil {
		progress = func(int) {}
	}
	etag, status, err := head(client, indexURL)
	if err != nil {
		return Index{}, err
	}
	progress(5)
	indexFile := cached(dir, "info.json.xz")
	if status != http.StatusNotFound && etag != "" && etag == previousETag {
		body, err := os.ReadFile(indexFile)
		if err == nil {
			progress(40)
			return Index{Body: body, ETag: etag, New: false}, nil
		}
	}
	body, etag, err := get(client, indexURL, etag, func(got, total int64) {
		if total <= 0 {
			progress(20)
			return
		}
		progress(5 + int(35*got/total))
	})
	if err != nil {
		return Index{}, err
	}
	if err := writeFile(indexFile, body); err != nil {
		return Index{}, err
	}
	if err := writeFile(cached(dir, "etag"), []byte(etag)); err != nil {
		return Index{}, err
	}
	progress(40)
	return Index{Body: body, ETag: etag, New: true}, nil
}

// ReadIndex returns the info.json.xz already stored for dir. It does not use the network.
func ReadIndex(dir string) (Index, error) {
	body, err := os.ReadFile(cached(dir, "info.json.xz"))
	if err != nil {
		return Index{}, err
	}
	if len(body) == 0 {
		return Index{}, fmt.Errorf("empty catalog index")
	}
	return Index{Body: body, ETag: ReadETag(dir)}, nil
}

func ReadETag(dir string) string {
	b, err := os.ReadFile(cached(dir, "etag"))
	if err != nil {
		return ""
	}
	return string(b)
}

// ImagePath is cacheRoot/cdn/<host>/<object path>, matching the CDN object.
func ImagePath(cacheRoot, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("image url must be https")
	}
	clean := path.Clean("/" + u.Path)
	if strings.Contains(clean, "..") || clean == "/" {
		return "", fmt.Errorf("bad image path")
	}
	return filepath.Join(cacheRoot, "cdn", u.Host, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

// EnsureImage downloads raw into ImagePath when that file is not already there.
func EnsureImage(client *http.Client, cacheRoot, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	dest, err := ImagePath(cacheRoot, raw)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return dest, nil
	}
	body, _, _, err := exchange(OrDefault(client), http.MethodGet, raw, "", nil)
	if err != nil {
		return "", err
	}
	if len(body) == 0 || int64(len(body)) > MaxBytes {
		return "", fmt.Errorf("image %s has an unexpected size", raw)
	}
	if err := writeFile(dest, body); err != nil {
		return "", err
	}
	return dest, nil
}

// OrDefault returns client, or a client with a one-minute timeout.
func OrDefault(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func cached(dir, name string) string {
	return filepath.Join(dir, "index", name)
}

func head(client *http.Client, raw string) (string, int, error) {
	_, etag, status, err := exchange(client, http.MethodHead, raw, "", nil)
	return etag, status, err
}

func get(client *http.Client, raw, etag string, progress func(got, total int64)) ([]byte, string, error) {
	body, gotETag, _, err := exchange(client, http.MethodGet, raw, etag, progress)
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > MaxBytes {
		return nil, "", fmt.Errorf("index larger than %d bytes", MaxBytes)
	}
	return body, gotETag, nil
}

// exchange performs one HEAD or GET. match is sent as If-None-Match when set.
// A HEAD that the server rejects with 405 returns that status and no error.
func exchange(client *http.Client, method, raw, match string, progress func(got, total int64)) (body []byte, etag string, status int, err error) {
	res, err := send(client, method, raw, match)
	if err != nil {
		return nil, "", 0, err
	}
	defer res.Body.Close()
	status = res.StatusCode
	if method == http.MethodHead && status == http.StatusMethodNotAllowed {
		return nil, "", status, nil
	}
	if status >= 400 {
		return nil, "", status, fmt.Errorf("%s %s: %s", method, raw, res.Status)
	}
	etag = res.Header.Get("ETag")
	if method == http.MethodHead {
		return nil, etag, status, nil
	}
	body, err = readBody(res.Body, res.ContentLength, progress)
	if err != nil {
		return nil, "", status, err
	}
	if etag == "" {
		etag = match
	}
	return body, etag, status, nil
}

func send(client *http.Client, method, raw, match string) (*http.Response, error) {
	req, err := http.NewRequest(method, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ftuna")
	if match != "" {
		req.Header.Set("If-None-Match", match)
	}
	return client.Do(req)
}

func readBody(r io.Reader, total int64, progress func(got, total int64)) ([]byte, error) {
	if progress != nil {
		var got int64
		r = &progressReader{r: r, n: &got, total: total, progress: progress}
	}
	return io.ReadAll(io.LimitReader(r, MaxBytes+1))
}

// ReadLimited reads r and rejects a body larger than MaxBytes.
func ReadLimited(r io.Reader) ([]byte, error) {
	body, err := readBody(r, 0, nil)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > MaxBytes {
		return nil, fmt.Errorf("response larger than %d bytes", MaxBytes)
	}
	return body, nil
}

type progressReader struct {
	r        io.Reader
	n        *int64
	total    int64
	progress func(got, total int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	*p.n += int64(n)
	if p.progress != nil && n > 0 {
		p.progress(*p.n, p.total)
	}
	return n, err
}

func writeFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0644)
}
