package install

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ssuDest  = "org.nemo.ssu"
	ssuPath  = "/org/nemo/ssu"
	ssuIface = "org.nemo.ssu"

	pkDest     = "org.freedesktop.PackageKit"
	pkPath     = "/org/freedesktop/PackageKit"
	pkIface    = "org.freedesktop.PackageKit"
	txIface    = "org.freedesktop.PackageKit.Transaction"
	propsIface = "org.freedesktop.DBus.Properties"

	filterNone      uint64 = 1 << 1
	filterInstalled uint64 = 1 << 2
	flagTrusted     uint64 = 1 << 1 // PackageKit TransactionFlagOnlyTrusted
	flagSimulate    uint64 = 1 << 2 // PackageKit TransactionFlagSimulate
)

// DBus talks to org.nemo.ssu and PackageKit on the system bus.
type DBus struct {
	conn     *dbus.Conn
	mu       sync.Mutex
	progress func(label string, percent int)
}

// SetProgress receives PackageKit status text and a percentage from 0 to 100
// while a transaction is running. percent is negative when PackageKit has no percentage.
func (d *DBus) SetProgress(fn func(label string, percent int)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.progress = fn
	d.mu.Unlock()
}

func (d *DBus) report(label string, percent int) {
	d.mu.Lock()
	fn := d.progress
	d.mu.Unlock()
	if fn != nil {
		fn(label, percent)
	}
}

func Connect() (*DBus, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &DBus{conn: conn}, nil
}

func (d *DBus) Close() error {
	if d == nil || d.conn == nil {
		return nil
	}
	return d.conn.Close()
}

func (d *DBus) ListRepos() ([]Repo, error) {
	var rows []struct {
		Alias string
		URL   string
		Props map[string]dbus.Variant
	}
	err := d.conn.Object(ssuDest, ssuPath).Call(ssuIface+".listRepos", 0, false).Store(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(rows))
	for _, row := range rows {
		out = append(out, Repo{Alias: row.Alias, URL: row.URL})
	}
	return out, nil
}

func (d *DBus) AddRepo(alias, raw string) error {
	return d.conn.Object(ssuDest, ssuPath).Call(ssuIface+".addRepo", 0, alias, raw).Store()
}

// SSU modifyRepo actions. Disable keeps the URL and drops the repository out of the enabled list.
const (
	ssuDisable = 2
	ssuEnable  = 3
)

func (d *DBus) EnableRepo(alias string) error {
	return d.modifyRepo(ssuEnable, alias)
}

func (d *DBus) DisableRepo(alias string) error {
	return d.modifyRepo(ssuDisable, alias)
}

func (d *DBus) modifyRepo(action int, alias string) error {
	return d.conn.Object(ssuDest, ssuPath).Call(ssuIface+".modifyRepo", 0, int32(action), alias).Store()
}

func (d *DBus) Refresh() error {
	_, err := d.transact("RefreshCache", false)
	return err
}

func (d *DBus) Resolve(names []string) ([]Found, error) {
	return d.transact("Resolve", filterNone, names)
}

func (d *DBus) Installed(names []string) ([]Found, error) {
	return d.transact("Resolve", filterInstalled, names)
}

func (d *DBus) Install(ids []string) error {
	_, err := d.transact("InstallPackages", flagTrusted, ids)
	return err
}

func (d *DBus) Simulate(ids []string) ([]Found, error) {
	// Sailfish PackageKit does not export SimulateInstallPackages.
	// InstallPackages with the simulate flag reports the same changes and does not install.
	return d.transact("InstallPackages", flagTrusted|flagSimulate, ids)
}

func (d *DBus) transact(method string, args ...interface{}) ([]Found, error) {
	var path dbus.ObjectPath
	if err := d.conn.Object(pkDest, pkPath).Call(pkIface+".CreateTransaction", 0).Store(&path); err != nil {
		return nil, err
	}
	tx := d.conn.Object(pkDest, path)
	if err := tx.Call(txIface+".SetHints", 0, []string{"interactive=true"}).Store(); err != nil {
		return nil, err
	}
	ch := make(chan *dbus.Signal, 256)
	d.conn.Signal(ch)
	defer d.conn.RemoveSignal(ch)
	if err := d.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(txIface),
	); err != nil {
		return nil, err
	}
	defer d.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(txIface),
	)
	if err := d.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(propsIface),
	); err != nil {
		return nil, err
	}
	defer d.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(propsIface),
	)

	callErr := make(chan error, 1)
	go func() {
		callErr <- tx.Call(txIface+"."+method, 0, args...).Store()
	}()

	// Package and Finished are signals. The method reply can arrive before
	// those signals are copied onto ch, and RefreshCache is not done until
	// Finished. Returning on the reply alone drops the package list.
	var found []Found
	var detail string
	var callDone bool
	var callError error
	var finished bool
	timeout := time.NewTimer(10 * time.Minute)
	defer timeout.Stop()
	grace := time.NewTimer(2 * time.Second)
	grace.Stop()
	for !(callDone && finished) {
		select {
		case sig := <-ch:
			if sig.Path != path {
				continue
			}
			switch sig.Name {
			case txIface + ".Package":
				if row, ok := packageFromSignal(sig); ok {
					found = append(found, row)
				}
			case txIface + ".ErrorCode":
				if len(sig.Body) >= 2 {
					if s, ok := sig.Body[1].(string); ok {
						detail = s
					}
				}
			case txIface + ".Finished":
				finished = true
			case txIface + ".ItemProgress":
				if label, pct, ok := itemProgress(sig.Body); ok {
					d.report(label, pct)
				}
			case propsIface + ".PropertiesChanged":
				if label, pct, ok := transactionProgress(sig.Body); ok {
					d.report(label, pct)
				}
			}
		case err := <-callErr:
			callDone = true
			callError = err
			if err != nil {
				grace.Reset(2 * time.Second)
			}
		case <-grace.C:
			if callDone && callError != nil {
				finished = true
			}
		case <-timeout.C:
			if callError != nil {
				return found, callError
			}
			return found, fmt.Errorf("packagekit %s did not finish", method)
		}
	}
	if callError != nil {
		if detail != "" {
			return found, fmt.Errorf("%s: %w", detail, callError)
		}
		return found, callError
	}
	return found, nil
}

func transactionProgress(body []interface{}) (label string, percent int, ok bool) {
	if len(body) < 2 {
		return "", -1, false
	}
	changed, isMap := body[1].(map[string]dbus.Variant)
	if !isMap {
		return "", -1, false
	}
	percent = -1
	if v, exists := changed["Status"]; exists {
		label = statusLabel(signalUint(v.Value()))
		ok = label != ""
	}
	if v, exists := changed["Percentage"]; exists {
		n := signalUint(v.Value())
		if n <= 100 {
			percent = int(n)
			ok = true
		}
	}
	return label, percent, ok
}

func itemProgress(body []interface{}) (label string, percent int, ok bool) {
	if len(body) < 3 {
		return "", -1, false
	}
	label = statusLabel(signalUint(body[1]))
	percent = -1
	n := signalUint(body[2])
	if n <= 100 {
		percent = int(n)
	}
	return label, percent, label != "" || percent >= 0
}

func statusLabel(status uint32) string {
	switch status {
	case 1, 30:
		return "Waiting"
	case 31:
		return "Waiting for authentication"
	case 2, 3:
		return "Preparing"
	case 4, 13:
		return "Resolving dependencies"
	case 7, 27:
		return "Refreshing repositories"
	case 8, 20, 21, 22, 23, 24, 25:
		return "Downloading"
	case 6, 32:
		return "Removing"
	case 9, 10, 16:
		return "Installing"
	case 14:
		return "Checking signatures"
	case 17:
		return "Finishing"
	default:
		return ""
	}
}

func packageFromSignal(sig *dbus.Signal) (Found, bool) {
	if len(sig.Body) < 2 {
		return Found{}, false
	}
	id, ok := sig.Body[1].(string)
	if !ok {
		return Found{}, false
	}
	row, ok := parsePackageID(id)
	if !ok {
		return Found{}, false
	}
	if len(sig.Body) > 0 {
		row.Info = signalUint(sig.Body[0])
	}
	if len(sig.Body) >= 3 {
		if summary, ok := sig.Body[2].(string); ok {
			row.Summary = summary
		}
	}
	return row, true
}

func signalUint(v interface{}) uint32 {
	switch n := v.(type) {
	case uint32:
		return n
	case uint64:
		return uint32(n)
	case int32:
		if n < 0 {
			return 0
		}
		return uint32(n)
	case int64:
		if n < 0 {
			return 0
		}
		return uint32(n)
	case int:
		if n < 0 {
			return 0
		}
		return uint32(n)
	case uint:
		return uint32(n)
	default:
		return 0
	}
}

func parsePackageID(id string) (Found, bool) {
	parts := strings.SplitN(id, ";", 4)
	if len(parts) < 3 || parts[0] == "" {
		return Found{}, false
	}
	row := Found{ID: id, Name: parts[0], Version: parts[1], Arch: parts[2]}
	if len(parts) == 4 {
		row.Data = parts[3]
	}
	return row, true
}
