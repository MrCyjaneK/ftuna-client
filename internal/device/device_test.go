package device

import (
	"strings"
	"testing"
)

func TestRelease(t *testing.T) {
	raw := "NAME=Sailfish OS\nVERSION_ID=5.2.0.17\n"
	got, err := Release(strings.NewReader(raw))
	if err != nil || got != "5.2.0.17" {
		t.Fatalf("%s %v", got, err)
	}
}
