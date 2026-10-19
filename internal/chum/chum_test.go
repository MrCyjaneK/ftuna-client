package chum

import (
	"strings"
	"testing"
)

func TestChooseOlderAarch64(t *testing.T) {
	dirs := []string{"4.4.0.58_aarch64", "4.4.0.64_aarch64", "4.4.0.64_armv7hl", "5.2.0.17_i486"}
	dir, fallback, err := Choose(dirs, "5.2.0.17", "aarch64")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "4.4.0.64_aarch64" || !fallback || Version(dir) != "4.4.0.64" {
		t.Fatalf("%s fallback %v", dir, fallback)
	}
	dir, fallback, err = Choose(append(dirs, "5.2.0.17_aarch64"), "5.2.0.17", "aarch64")
	if err != nil || dir != "5.2.0.17_aarch64" || fallback {
		t.Fatalf("%s %v %v", dir, fallback, err)
	}
}

func TestReleasesAndPackages(t *testing.T) {
	names, err := Releases(`<a href="4.4.0.64_aarch64/">4.4.0.64_aarch64/</a><a href="../">up</a>`)
	if err != nil || len(names) != 1 || names[0] != "4.4.0.64_aarch64" {
		t.Fatal(err, names)
	}
	href, err := PrimaryHref([]byte(`<repomd><data type="primary"><location href="repodata/primary.xml.gz"/></data></repomd>`))
	if err != nil || href != "repodata/primary.xml.gz" {
		t.Fatal(err, href)
	}
	pkgs, err := Packages([]byte(`
<metadata>
  <package type="rpm">
    <name>harbour-demo</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1.2" rel="3"/>
    <checksum type="sha256" pkgid="YES">ABC123</checksum>
    <summary>Demo</summary>
    <description>A demo</description>
    <time file="1" build="1700000000"/>
    <format>
      <rpm:sourcerpm>harbour-demo-1.2-3.src.rpm</rpm:sourcerpm>
      <rpm:requires>
        <rpm:entry name="qt5-qtcore"/>
        <rpm:entry name="rpmlib(CompressedFileNames)"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>harbour-demo-debuginfo</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1.2" rel="3"/>
    <checksum type="sha" pkgid="YES">7535892df011b5dbaaab324f02e7232ba75dd9f3</checksum>
    <summary>debug</summary>
    <format><rpm:sourcerpm>harbour-demo-1.2-3.src.rpm</rpm:sourcerpm></format>
  </package>
  <package type="rpm">
    <name>harbour-demo-devel</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1.2" rel="3"/>
    <summary>devel</summary>
    <format><rpm:sourcerpm>harbour-demo-1.2-3.src.rpm</rpm:sourcerpm></format>
  </package>
</metadata>`), "chum-4.4.0.64-aarch64", "https://repo-chum.ftuna.eu/obs/sailfishos:/chum/4.4.0.64_aarch64/")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].ID != "harbour-demo" || pkgs[0].Category != "application" {
		t.Fatalf("%+v", pkgs)
	}
	if len(pkgs[0].RPMs) != 3 || !pkgs[0].RPMs[0].Install || pkgs[0].RPMs[1].Install || pkgs[0].RPMs[2].Install {
		t.Fatalf("%+v", pkgs[0].RPMs)
	}
	if pkgs[0].RPMs[0].Version != "1.2" || pkgs[0].RPMs[0].Release != "3" || len(pkgs[0].RPMs[0].Requires) != 1 {
		t.Fatalf("%+v", pkgs[0].RPMs[0])
	}
	if pkgs[0].RPMs[0].SHA256 != "abc123" || pkgs[0].RPMs[1].SHA256 != "" {
		t.Fatalf("%+v", pkgs[0].RPMs)
	}
	if !strings.HasSuffix(pkgs[0].RepoURL, "4.4.0.64_aarch64/") {
		t.Fatal(pkgs[0].RepoURL)
	}
	if !strings.HasSuffix(pkgs[0].Description, "AI-score-unknown.svg\">") || !strings.HasPrefix(pkgs[0].Description, "A demo") {
		t.Fatalf("description %s", pkgs[0].Description)
	}
}

func TestPackageCategoryUsesChumType(t *testing.T) {
	pkgs, err := Packages([]byte(`
<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>angelfish</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1.0" rel="1"/>
    <summary>Web browser</summary>
    <description>A browser

Type: desktop-application
Categories:
 - Network
PackageIcon: https://example.com/angelfish.png
</description>
    <format>
      <rpm:group>Unspecified</rpm:group>
      <rpm:sourcerpm>angelfish-1.0-1.src.rpm</rpm:sourcerpm>
    </format>
  </package>
  <package type="rpm">
    <name>libfoo</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1" rel="1"/>
    <summary>Foo</summary>
    <description>Helpers</description>
    <format>
      <rpm:group>Development/Libraries</rpm:group>
      <rpm:sourcerpm>libfoo-1-1.src.rpm</rpm:sourcerpm>
    </format>
  </package>
  <package type="rpm">
    <name>harbour-addon</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1" rel="1"/>
    <summary>Addon</summary>
    <description>Extra data

Type: addon
</description>
    <format>
      <rpm:group>Applications</rpm:group>
      <rpm:sourcerpm>harbour-addon-1-1.src.rpm</rpm:sourcerpm>
    </format>
  </package>
  <package type="rpm">
    <name>hutspot</name>
    <arch>aarch64</arch>
    <version epoch="0" ver="1" rel="1"/>
    <summary>Hutspot</summary>
    <description>Player</description>
    <format>
      <rpm:group>Applications/Multimedia</rpm:group>
      <rpm:sourcerpm>hutspot-1-1.src.rpm</rpm:sourcerpm>
    </format>
  </package>
</metadata>`), "chum", "https://example/")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, pkg := range pkgs {
		got[pkg.ID] = pkg.Category
	}
	if got["angelfish"] != "application" || got["libfoo"] != "library" || got["harbour-addon"] != "library" || got["hutspot"] != "application" {
		t.Fatalf("%v", got)
	}
	for _, pkg := range pkgs {
		if pkg.ID == "angelfish" && pkg.Icon != "https://example.com/angelfish.png" {
			t.Fatalf("icon %q", pkg.Icon)
		}
	}
}

func TestChumAIRating(t *testing.T) {
	raw := "A camera application.\n\nTitle: Advanced Camera\nType: desktop-application\nAIRating: B\nPackageIcon: https://example.com/icon.svg\n"
	body, code := chumAI(raw)
	if body != "A camera application." || code != "B" {
		t.Fatalf("body %q code %q", body, code)
	}
	got := descriptionWithAI(raw)
	if !strings.HasPrefix(got, "A camera application.\n\n<img src=\"") || !strings.HasSuffix(got, "AI-score-B.svg\">") {
		t.Fatalf("description %s", got)
	}
	if _, code := chumAI("Just prose"); code != "" {
		t.Fatalf("code %q", code)
	}
	body, code = chumAI("Title: Orphan\nAIRating: nope\n")
	if body != "" || code != "" {
		t.Fatalf("invalid body %q code %q", body, code)
	}
	got = descriptionWithAI("Title: Orphan\nAIRating: X\n")
	if strings.Contains(got, "Title:") || !strings.Contains(got, "AI-score-will-not-tell.svg") {
		t.Fatalf("withheld %s", got)
	}
}
