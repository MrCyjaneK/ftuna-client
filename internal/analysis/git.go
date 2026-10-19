package analysis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ratingsBranch = "master"
	fetchTTL      = 24 * time.Hour
	maxRating     = 8 << 20
)

var ratingsRemote = "https://github.com/MrCyjaneK/ftuna-ratings.git"

var (
	repoMu    sync.Mutex
	preparing bool
	fetchedAt time.Time
)

// Prepare clones or fetches the ratings repository when the last fetch is older
// than a day. Load does not do this. A fetch that fails still keeps the clone
// already on disk.
func Prepare(dataDir string) error {
	return prepare(dataDir, false, nil)
}

// ForcePrepare always clones or fetches the ratings repository.
// progress may be nil. It receives a short git phase, such as "Receiving objects",
// and a percent from 0 to 100.
func ForcePrepare(dataDir string, progress func(label string, percent int)) error {
	return prepare(dataDir, true, progress)
}

func prepare(dataDir string, force bool, progress func(label string, percent int)) error {
	repoMu.Lock()
	for preparing {
		repoMu.Unlock()
		if !force {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
		repoMu.Lock()
	}
	preparing = true
	exists := cloneExists(dataDir)
	fresh := exists && freshFetch(dataDir)
	repoMu.Unlock()

	defer func() {
		repoMu.Lock()
		preparing = false
		repoMu.Unlock()
	}()

	if exists && fresh && !force {
		return nil
	}
	report := func(label string, percent int) {
		if progress != nil {
			progress(label, percent)
		}
	}
	if !exists {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return err
		}
		report("Cloning", 0)
		if err := clone(dataDir, report); err != nil {
			return err
		}
	} else {
		report("Fetching", 0)
		if err := fetch(repoPath(dataDir), report); err != nil {
			if !force {
				repoMu.Lock()
				fetchedAt = time.Now()
				repoMu.Unlock()
				return nil
			}
			return err
		}
	}
	repoMu.Lock()
	fetchedAt = time.Now()
	repoMu.Unlock()
	return stamp(dataDir)
}

func cloneExists(dataDir string) bool {
	_, err := os.Stat(filepath.Join(repoPath(dataDir), "HEAD"))
	return err == nil
}

func repoPath(dataDir string) string {
	return filepath.Join(dataDir, "ratings.git")
}

func fetchedPath(dataDir string) string {
	return filepath.Join(dataDir, "ratings.fetched")
}

func clone(dataDir string, progress func(label string, percent int)) error {
	dir := repoPath(dataDir)
	tmp := dir + ".partial"
	_ = os.RemoveAll(tmp)
	if err := gitProgress(progress, "clone", "--bare", "--progress", ratingsRemote, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	_ = os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return nil
}

func fetch(dir string, progress func(label string, percent int)) error {
	return gitProgress(progress, "--git-dir", dir, "fetch", "--force", "--progress", "origin", "+refs/heads/"+ratingsBranch+":refs/heads/"+ratingsBranch)
}

func catMany(dir string, rels []string) ([][]byte, []bool, error) {
	if len(rels) == 0 {
		return nil, nil, nil
	}
	var stdin bytes.Buffer
	for _, rel := range rels {
		stdin.WriteString(ratingsBranch + ":" + rel + "\n")
	}
	stdout, stderr, err := runGit(&stdin, "--git-dir", dir, "cat-file", "--batch")
	if err != nil {
		return nil, nil, gitFail(err, stderr)
	}
	return readBatch(bytes.NewReader(stdout), len(rels))
}

func readBatch(in io.Reader, n int) ([][]byte, []bool, error) {
	r := bufio.NewReader(in)
	bodies := make([][]byte, n)
	missing := make([]bool, n)
	for i := 0; i < n; i++ {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		line = strings.TrimRight(line, "\n")
		if strings.HasSuffix(line, " missing") {
			missing[i] = true
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, nil, fmt.Errorf("bad cat-file line %q", line)
		}
		size, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
		if err != nil || size < 0 || size > maxRating {
			return nil, nil, fmt.Errorf("rating is too large")
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, nil, err
		}
		if b, err := r.ReadByte(); err != nil || b != '\n' {
			return nil, nil, fmt.Errorf("bad cat-file trailer")
		}
		bodies[i] = body
	}
	return bodies, missing, nil
}

func cat(dir, rel string) ([]byte, error) {
	stdout, stderr, err := runGit(nil, "--git-dir", dir, "cat-file", "blob", ratingsBranch+":"+rel)
	if err != nil {
		if missingBlob(strings.TrimSpace(stderr)) {
			return nil, ErrNotFound
		}
		return nil, gitFail(err, stderr)
	}
	if len(stdout) > maxRating {
		return nil, fmt.Errorf("rating is too large")
	}
	return stdout, nil
}

func gitCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv()
	return cmd
}

func runGit(stdin io.Reader, args ...string) (stdout []byte, stderr string, err error) {
	cmd := gitCommand(args...)
	cmd.Stdin = stdin
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.Bytes(), errBuf.String(), err
}

func gitFail(err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		return err
	}
	return fmt.Errorf("git cat-file: %s", msg)
}

func missingBlob(msg string) bool {
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "exists on disk, but not in") ||
		strings.Contains(msg, "Not a valid object name")
}

var percentPattern = regexp.MustCompile(`(\d+)%`)

func gitProgress(progress func(label string, percent int), args ...string) error {
	cmd := gitCommand(args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var text syncBuf
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		watchProgress(io.TeeReader(stderr, &text), progress)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&text, stdout)
	}()
	wg.Wait()
	err = cmd.Wait()
	if err != nil {
		msg := gitMessage(text.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func watchProgress(r io.Reader, progress func(label string, percent int)) {
	sc := bufio.NewScanner(r)
	sc.Split(splitProgress)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if progress == nil {
			continue
		}
		label, pct, ok := parseGitProgress(sc.Text())
		if ok {
			progress(label, pct)
		}
	}
}

func splitProgress(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\r' || b == '\n' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func parseGitProgress(line string) (string, int, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "remote:")
	line = strings.TrimSpace(line)
	m := percentPattern.FindStringSubmatchIndex(line)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(line[m[2]:m[3]])
	if err != nil {
		return "", 0, false
	}
	label := strings.Trim(line[:m[2]], " :")
	if label == "" {
		label = "Fetching"
	}
	return label, n, true
}

func gitMessage(out string) string {
	lines := strings.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == '\r' })
	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, _, ok := parseGitProgress(line); ok {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > 6 {
		kept = kept[len(kept)-6:]
	}
	return strings.Join(kept, "; ")
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}

func freshFetch(dataDir string) bool {
	if !fetchedAt.IsZero() && time.Since(fetchedAt) < fetchTTL {
		return true
	}
	body, err := os.ReadFile(fetchedPath(dataDir))
	if err != nil {
		return false
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(sec, 0)) < fetchTTL
}

func stamp(dataDir string) error {
	body := []byte(strconv.FormatInt(time.Now().Unix(), 10))
	return os.WriteFile(fetchedPath(dataDir), body, 0644)
}
