// Package device reads the Sailfish release and CPU architecture.
package device

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mrcyjanek/ftuna/client/internal/catalog"
)

// Detect reads VERSION_ID from /etc/os-release and the machine from uname.
func Detect() (release, arch string, err error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	release, err = Release(f)
	if err != nil {
		return "", "", err
	}
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", "", err
	}
	arch = catalog.NormalizeArch(strings.TrimSpace(string(out)))
	if arch == "" {
		return "", "", fmt.Errorf("uname returned an empty machine")
	}
	return release, arch, nil
}

// Release reads VERSION_ID from an os-release file.
func Release(r interface{ Read([]byte) (int, error) }) (string, error) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "VERSION_ID=") {
			continue
		}
		value := strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"'`)
		if value == "" {
			return "", fmt.Errorf("VERSION_ID is empty")
		}
		return value, nil
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("VERSION_ID is missing")
}
