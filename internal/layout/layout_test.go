package layout

import "testing"

func TestJoinedFallback(t *testing.T) {
	html := `<a href="4.4.0.58_aarch64/">a</a><a href="4.4.0.64_aarch64/">b</a><a href="4.4.0.64_armv7hl/">c</a><a href="../">up</a>`
	pattern := "https://repo.sailfishos.org/obs/sailfishos:/chum/{version}_{arch}/"
	hits, err := Matches(pattern, "5.2.0.17", "aarch64", html)
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].Version != "4.4.0.64" || !hits[0].Fallback {
		t.Fatalf("%+v", hits[0])
	}
	if hits[0].URL != "https://repo.sailfishos.org/obs/sailfishos:/chum/4.4.0.64_aarch64/" {
		t.Fatal(hits[0].URL)
	}
	if len(hits) != 2 || hits[1].Version != "4.4.0.58" {
		t.Fatalf("%+v", hits)
	}
}

func TestPreferChumUsesMajorMinor(t *testing.T) {
	html := `<a href="5.0.0.76_aarch64/">snap</a><a href="5.0_aarch64/">roll</a><a href="4.6_aarch64/">old</a>`
	pattern := "https://repo.sailfishos.org/obs/sailfishos:/chum/{version}_{arch}/"
	hits, err := Matches(pattern, "5.0.0.76", "aarch64", html)
	if err != nil {
		t.Fatal(err)
	}
	hits = PreferChum(hits, "5.0.0.76")
	if hits[0].Version != "5.0" || hits[0].Fallback {
		t.Fatalf("%+v", hits[0])
	}
	if hits[0].URL != "https://repo.sailfishos.org/obs/sailfishos:/chum/5.0_aarch64/" {
		t.Fatal(hits[0].URL)
	}
}

func TestPreferChumKeepsFullReleaseBefore46(t *testing.T) {
	html := `<a href="4.5.0.24_aarch64/">a</a><a href="4.5.0.25_aarch64/">b</a>`
	pattern := "https://repo.sailfishos.org/obs/sailfishos:/chum/{version}_{arch}/"
	hits, err := Matches(pattern, "4.5.0.25", "aarch64", html)
	if err != nil {
		t.Fatal(err)
	}
	hits = PreferChum(hits, "4.5.0.25")
	if hits[0].Version != "4.5.0.25" || hits[0].Fallback {
		t.Fatalf("%+v", hits[0])
	}
}

func TestArchOnly(t *testing.T) {
	pattern := "https://sfos.repo.mrcyjanek.net/{arch}/"
	if NeedsListing(pattern) || !NeedsListing("https://repo-oss.ftuna.eu/{version}/{arch}/") {
		t.Fatal("listing requirement")
	}
	hits, err := Matches(pattern, "5.2.0.17", "aarch64", `<a href="nope/">x</a>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Fallback || hits[0].Version != "" || hits[0].URL != "https://sfos.repo.mrcyjanek.net/aarch64/" {
		t.Fatalf("%+v", hits)
	}
	hits, err = Matches(pattern, "5.2.0.17", "armv7l", "")
	if err != nil {
		t.Fatal(err)
	}
	if hits[0].URL != "https://sfos.repo.mrcyjanek.net/armv7hl/" {
		t.Fatal(hits[0].URL)
	}
	if _, err := Matches(pattern, "5.2.0.17", "", ""); err == nil {
		t.Fatal("empty arch")
	}
	if _, err := Matches("https://example.com/{arch}/extra/", "5.2.0.17", "aarch64", ""); err == nil {
		t.Fatal("expected unsupported pattern")
	}
	dir, err := RPMDir("https://sfos.repo.mrcyjanek.net/aarch64/", "armv7hl")
	if err != nil || dir != "https://sfos.repo.mrcyjanek.net/aarch64/" {
		t.Fatalf("%s %v", dir, err)
	}
	dir, err = RPMDir("https://sfos.repo.mrcyjanek.net/armv7l", "aarch64")
	if err != nil || dir != "https://sfos.repo.mrcyjanek.net/armv7hl/" {
		t.Fatalf("%s %v", dir, err)
	}
	dir, err = RPMDir("https://sfos.repo.mrcyjanek.net/", "armv7l")
	if err != nil || dir != "https://sfos.repo.mrcyjanek.net/armv7hl/" {
		t.Fatalf("%s %v", dir, err)
	}
}

func TestSplitExact(t *testing.T) {
	html := `<a href="5.1.0.11/">old</a><a href="5.2.0.17/">now</a><a href="info.json.xz">index</a>`
	pattern := "https://repo-oss.ftuna.eu/{version}/{arch}/"
	hits, err := Matches(pattern, "5.2.0.17", "armv7l", html)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Version != "5.2.0.17" || hits[0].Fallback {
		t.Fatalf("%+v", hits)
	}
	if hits[0].URL != "https://repo-oss.ftuna.eu/5.2.0.17/armv7hl/" {
		t.Fatal(hits[0].URL)
	}
	if Templated(pattern) != true || Templated("https://repo-openrepos.ftuna.eu/") {
		t.Fatal("template detection")
	}
	if Prefix(pattern) != "https://repo-oss.ftuna.eu/" {
		t.Fatal(Prefix(pattern))
	}
}
