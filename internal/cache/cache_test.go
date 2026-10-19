package cache

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImagePath(t *testing.T) {
	got, err := ImagePath("/home/defaultuser/.cache/ftuna", "https://cdn-mirror.ftuna.eu/raw.githubusercontent.com/piggz/harbour-advanced-camera/cdcef3fb/harbour-advanced-camera.svg")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/home/defaultuser/.cache/ftuna", "cdn", "cdn-mirror.ftuna.eu", "raw.githubusercontent.com", "piggz", "harbour-advanced-camera", "cdcef3fb", "harbour-advanced-camera.svg")
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestFetchUsesETag(t *testing.T) {
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		if r.Method == http.MethodHead {
			return
		}
		gets++
		w.Write([]byte("index-bytes"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	first, err := FetchIndex(srv.Client(), srv.URL, dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.New || string(first.Body) != "index-bytes" || first.ETag != `"abc"` {
		t.Fatalf("%+v", first)
	}
	second, err := FetchIndex(srv.Client(), srv.URL, dir, first.ETag, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.New || gets != 1 {
		t.Fatalf("gets %d new %v", gets, second.New)
	}
}

func TestEnsureImage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("png"))
	}))
	defer srv.Close()
	raw := srv.URL + "/raw.githubusercontent.com/ex/app/icon.svg"
	dir := t.TempDir()
	path, err := EnsureImage(srv.Client(), dir, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, filepath.Join("cdn", strings.TrimPrefix(srv.URL, "https://"))) {
		t.Fatalf("path %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "png" {
		t.Fatalf("%s %v", body, err)
	}
	if _, err := EnsureImage(srv.Client(), dir, raw); err != nil {
		t.Fatal(err)
	}
}
