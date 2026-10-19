package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"encoding/json"
	"sync"
	"unsafe"

	"github.com/mrcyjanek/ftuna/client/internal/app"
	"github.com/mrcyjanek/ftuna/client/internal/install"
)

var (
	once        sync.Once
	application *app.App
	openErr     error
)

func inst() (*app.App, error) {
	once.Do(func() {
		application, openErr = app.Open(app.Config{PrepareRatings: true})
	})
	return application, openErr
}

func cstr(s string) *C.char { return C.CString(s) }

func cerr(err error) *C.char {
	if err == nil {
		return nil
	}
	return C.CString(err.Error())
}

//export FtunaFree
func FtunaFree(p *C.char) {
	if p != nil {
		C.free(unsafe.Pointer(p))
	}
}

//export FtunaStatus
func FtunaStatus() *C.char {
	a, err := inst()
	if err != nil {
		return cstr("")
	}
	return cstr(a.Status())
}

//export FtunaProgress
func FtunaProgress() C.int {
	a, err := inst()
	if err != nil {
		return 0
	}
	return C.int(a.Progress())
}

//export FtunaRefresh
func FtunaRefresh() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	return cerr(a.Refresh())
}

//export FtunaLoadCached
func FtunaLoadCached() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	published, err := a.LoadCached()
	if err != nil {
		return cerr(err)
	}
	if published {
		return cstr("1")
	}
	return cstr("0")
}

//export FtunaRefreshCatalogs
func FtunaRefreshCatalogs() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	return cerr(a.RefreshCatalogs())
}

//export FtunaFetchRatings
func FtunaFetchRatings() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	return cerr(a.FetchRatings())
}

//export FtunaState
func FtunaState() *C.char {
	a, err := inst()
	if err != nil {
		raw, _ := json.Marshal(map[string]string{"error": err.Error()})
		return cstr(string(raw))
	}
	raw, err := a.State()
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaListing
func FtunaListing() *C.char {
	a, err := inst()
	if err != nil {
		raw, _ := json.Marshal(map[string]string{"error": err.Error()})
		return cstr(string(raw))
	}
	raw, err := a.Listing()
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaPackage
func FtunaPackage(id *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	raw, err := a.Package(C.GoString(id))
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaOpenReposUsers
func FtunaOpenReposUsers(refresh C.int) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	users, err := a.OpenReposUsers(refresh != 0)
	if err != nil {
		return cerr(err)
	}
	if users == nil {
		users = []app.OpenReposUser{}
	}
	raw, err := json.Marshal(users)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaRepoPackages
func FtunaRepoPackages(id *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	raw, err := a.RepoPackages(C.GoString(id))
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaSearch
func FtunaSearch(query *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	raw, err := a.Search(C.GoString(query))
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaSettings
func FtunaSettings() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	settings, err := a.Settings()
	if err != nil {
		return cerr(err)
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaSaveSettings
func FtunaSaveSettings(raw *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	settings, err := a.SaveSettings([]byte(C.GoString(raw)))
	if err != nil {
		return cerr(err)
	}
	out, err := json.Marshal(settings)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(out))
}

//export FtunaRememberFallbacks
func FtunaRememberFallbacks(raw *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	if _, err := a.RememberFallbacks([]byte(C.GoString(raw))); err != nil {
		return cerr(err)
	}
	return nil
}

//export FtunaReport
func FtunaReport(id, source, names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	raw, err := json.Marshal(a.Report(C.GoString(id), C.GoString(source), list))
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaDependencies
func FtunaDependencies(id, source, names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	raw, err := json.Marshal(a.Dependencies(C.GoString(id), C.GoString(source), list))
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaInstalledIDs
func FtunaInstalledIDs(names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	ids, err := a.InstalledIDs(list)
	if err != nil {
		return cerr(err)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaSystemRepos
func FtunaSystemRepos() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	repos, err := a.SystemRepos()
	if err != nil {
		return cerr(err)
	}
	if repos == nil {
		repos = []install.Repo{}
	}
	raw, err := json.Marshal(repos)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaSetRepoEnabled
func FtunaSetRepoEnabled(repoURL *C.char, on C.int) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	return cerr(a.SetRepoEnabled(C.GoString(repoURL), on != 0))
}

//export FtunaPreview
func FtunaPreview(repoURL, names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	plan, err := a.Preview(C.GoString(repoURL), list)
	if err != nil {
		return cerr(err)
	}
	if plan.IDs == nil {
		plan.IDs = []string{}
	}
	if plan.Changes == nil {
		plan.Changes = []install.Change{}
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaPackageIDs
func FtunaPackageIDs(repoURL, names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	ids, err := a.PackageIDs(C.GoString(repoURL), list)
	if err != nil {
		return cerr(err)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return cerr(err)
	}
	return cstr(string(raw))
}

//export FtunaRatingsGeneration
func FtunaRatingsGeneration() C.int {
	a, err := inst()
	if err != nil {
		return 0
	}
	return C.int(a.RatingsGeneration())
}

//export FtunaWipe
func FtunaWipe() *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	return cerr(a.Wipe())
}

//export FtunaInstall
func FtunaInstall(repoURL, names *C.char) *C.char {
	a, err := inst()
	if err != nil {
		return cerr(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(C.GoString(names)), &list); err != nil {
		return cerr(err)
	}
	return cerr(a.Install(C.GoString(repoURL), list))
}

func main() {}
