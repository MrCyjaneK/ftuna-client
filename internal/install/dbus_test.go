package install

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestTransactionProgressRefresh(t *testing.T) {
	label, pct, ok := transactionProgress([]interface{}{
		txIface,
		map[string]dbus.Variant{
			"Status":     dbus.MakeVariant(uint32(7)),
			"Percentage": dbus.MakeVariant(uint32(40)),
		},
	})
	if !ok || label != "Refreshing repositories" || pct != 40 {
		t.Fatalf("%q %d %v", label, pct, ok)
	}
}

func TestTransactionProgressIgnoresUnknownPercentage(t *testing.T) {
	label, pct, ok := transactionProgress([]interface{}{
		txIface,
		map[string]dbus.Variant{
			"Percentage": dbus.MakeVariant(uint32(101)),
			"Status":     dbus.MakeVariant(uint32(20)),
		},
	})
	if !ok || label != "Downloading" || pct != -1 {
		t.Fatalf("%q %d %v", label, pct, ok)
	}
}

func TestItemProgress(t *testing.T) {
	label, pct, ok := itemProgress([]interface{}{"repo", uint32(20), uint32(15)})
	if !ok || label != "Downloading" || pct != 15 {
		t.Fatalf("%q %d %v", label, pct, ok)
	}
}
