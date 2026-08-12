package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// The package manager.
//
// Three decisions shape all of this, and each was a fork worth naming.
//
// There is no central registry. A package is a git host, an owner, a
// repository and a tag. Running a registry means running a service
// forever, and a language at this stage should not depend on one
// existing. Go took the same route and it has held up.
//
// Packages arrive as tarballs over HTTPS rather than through `git`.
// Go's standard library can fetch and unpack them, so this costs no
// dependency and nobody needs git installed to use a library.
//
// Versions are exact. There is no resolver, no ranges, no "compatible
// with". If two dependencies want different versions of the same
// package, that is reported and the build stops. A wrong answer chosen
// quietly is worse than an error that says what to fix.

// manifestName is the file that makes a directory a Veyl project or
// a Veyl package. Same format for both: a package is just a project
// somebody else depends on.
const (
	manifestName = "veyl.json"
	lockName     = "veyl.lock"
)

// Manifest is veyl.json.
type Manifest struct {
	Name string `json:"name"`
	// Version is what this package calls itself. Unused for an
	// application, required for something others will depend on.
	Version string `json:"version,omitempty"`
	// Main is the file an importer gets. Defaults to <name>.vy.
	Main         string            `json:"main,omitempty"`
	Description  string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// LockEntry pins exactly what was installed. The hash is what makes a
// reinstall reproducible: the same version tag can be moved to point
// at different code, and this is what notices.
type LockEntry struct {
	Source  string `json:"source"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Lock is veyl.lock.
type Lock struct {
	Packages map[string]LockEntry `json:"packages"`
}

// A source is "github.com/owner/repo@v1.2.3", or a path beginning with
// "." or "/" for a package being developed alongside the program.
var sourcePattern = regexp.MustCompile(`^([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)@(.+)$`)

type source struct {
	host, owner, repo, version string
	local                      string // set instead, for a path dependency
}

func (s source) String() string {
	if s.local != "" {
		return s.local
	}
	return fmt.Sprintf("%s/%s/%s@%s", s.host, s.owner, s.repo, s.version)
}

// parseSource reads a dependency string.
func parseSource(spec string) (source, error) {
	if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || filepath.IsAbs(spec) {
		return source{local: spec}, nil
	}
	m := sourcePattern.FindStringSubmatch(spec)
	if m == nil {
		return source{}, fmt.Errorf(
			"cannot understand %q.\n"+
				"A dependency looks like  github.com/owner/repo@v1.0.0\n"+
				"or a path like  ../mylib  for one you are writing yourself", spec)
	}
	if m[1] != "github.com" {
		return source{}, fmt.Errorf("only github.com is supported so far, got %q", m[1])
	}
	return source{host: m[1], owner: m[2], repo: m[3], version: m[4]}, nil
}

// tarballURLs are where the code might come from, best first. codeload
// serves archives directly, without the redirect github.com/... issues.
//
// A tag is tried before a branch. Both are allowed: pinning to a tag is
// the sane default, but refusing branches outright would block anyone
// depending on a library that has not tagged a release yet. What
// actually guarantees reproducibility is the hash in veyl.lock, which
// notices either kind of reference changing underneath.
func (s source) tarballURLs() []string {
	base := fmt.Sprintf("https://codeload.%s/%s/%s/tar.gz", s.host, s.owner, s.repo)
	return []string{
		base + "/refs/tags/" + s.version,
		base + "/refs/heads/" + s.version,
	}
}

// cacheDir is where a fetched package lives. Shared between projects,
// because the same version of the same package is the same bytes, and
// a copy per project would be a lot of duplicated source for nothing.
func (s source) cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "veyl", "pkg",
		s.host, s.owner, s.repo+"@"+s.version), nil
}

// looksLikePath reports whether an import was clearly meant to name a
// file rather than a package. Package names are plain words, so a dot,
// a separator or a drive letter settles it.
func looksLikePath(s string) bool {
	return strings.ContainsAny(s, `./\:`)
}

// ---------------------------------------------------------------------
// reading and writing the manifest

func loadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %v", manifestName, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s has no \"name\"", manifestName)
	}
	return &m, nil
}

func saveManifest(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), append(data, '\n'), 0o644)
}

func loadLock(dir string) *Lock {
	l := &Lock{Packages: map[string]LockEntry{}}
	data, err := os.ReadFile(filepath.Join(dir, lockName))
	if err != nil {
		return l
	}
	// A corrupt lock is not fatal: it is a cache of a decision, and the
	// manifest is the source of truth. Refetching is the safe recovery.
	_ = json.Unmarshal(data, l)
	if l.Packages == nil {
		l.Packages = map[string]LockEntry{}
	}
	return l
}

func saveLock(dir string, l *Lock) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, lockName), append(data, '\n'), 0o644)
}

// findProjectRoot walks up from a file looking for veyl.json, so
// `veyl run src/main.vy` works from anywhere inside a project.
func findProjectRoot(start string) string {
	dir := start
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, manifestName)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached the root without finding one
		}
		dir = parent
	}
}

// ---------------------------------------------------------------------
// fetching

// fetch downloads and unpacks a package unless it is already cached.
// It returns the directory holding the package.
func fetch(s source, quiet bool) (string, error) {
	if s.local != "" {
		abs, err := filepath.Abs(s.local)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, manifestName)); err != nil {
			return "", fmt.Errorf("%s has no %s, so it is not a Veyl package", s.local, manifestName)
		}
		return abs, nil
	}

	dir, err := s.cacheDir()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, manifestName)); err == nil {
		return dir, nil // already have it
	}

	if !quiet {
		fmt.Printf("  fetching %s\n", s)
	}
	var body []byte
	var lastErr error
	for _, url := range s.tarballURLs() {
		body, lastErr = download(url)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("cannot fetch %s: %v", s, lastErr)
	}

	// Unpack beside the target and rename, so an interrupted download
	// never leaves a half-populated directory that looks cached.
	tmp := dir + ".partial"
	os.RemoveAll(tmp)
	if err := extractTarGz(body, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("cannot unpack %s: %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(tmp, manifestName)); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("%s has no %s at its root, so it is not a Veyl package", s, manifestName)
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no such tag or branch (spelling and case both matter)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server said %s", resp.Status)
	}
	// A cap, so a wrong URL pointing at something enormous cannot fill
	// the disk before anyone notices.
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// extractTarGz unpacks a GitHub tarball, dropping the wrapper directory
// that GitHub adds around the repository contents.
func extractTarGz(data []byte, dest string) error {
	zr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Strip the leading "repo-version/" component.
		name := hdr.Name
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		} else {
			continue // the wrapper directory entry itself
		}
		if name == "" {
			continue
		}

		// An archive can name anything it likes, including ../../ or an
		// absolute path. Refusing those is the whole of tarball safety.
		clean := path.Clean(name)
		if path.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("archive contains an unsafe path: %q", hdr.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(clean))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			// Same cap per file as for the whole download.
			if _, err := io.Copy(f, io.LimitReader(tr, 64<<20)); err != nil {
				f.Close()
				return err
			}
			f.Close()
		default:
			// Symlinks and devices have no business in a source package
			// and are the other half of tarball safety. Skipped rather
			// than refused, since an archive may carry harmless extras.
			continue
		}
	}
}

// hashDir fingerprints a package so the lock file can notice if a tag
// is moved to point at different code. Names and contents both count,
// walked in sorted order so the result does not depend on the
// filesystem's iteration order.
func hashDir(dir string) (string, error) {
	var files []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(rel))
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------------------------------------------------------------------
// resolving imports to package directories

// packageResolver maps an import name to the file that satisfies it.
type packageResolver struct {
	root  string // project directory, or "" when there is no manifest
	deps  map[string]string
	cache map[string]string
}

func newPackageResolver(startFile string) *packageResolver {
	r := &packageResolver{deps: map[string]string{}, cache: map[string]string{}}
	r.root = findProjectRoot(startFile)
	if r.root == "" {
		return r
	}
	if m, err := loadManifest(r.root); err == nil {
		r.deps = m.Dependencies
	}
	return r
}

// resolve turns `import "strutil"` into the path of the .vy file that
// package exposes. The error text carries the fix, because "package not
// found" on its own leaves someone guessing which of three things
// went wrong.
func (r *packageResolver) resolve(name string) (string, error) {
	if hit, ok := r.cache[name]; ok {
		return hit, nil
	}
	if r.root == "" {
		return "", fmt.Errorf(
			"%q is not a file, so it is read as a package - but there is no %s here.\n"+
				"Run 'veyl init' to start a project, then 'veyl add <package>'",
			name, manifestName)
	}
	spec, ok := r.deps[name]
	if !ok {
		return "", fmt.Errorf(
			"no package called %q in %s.\nAdd one with: veyl add github.com/owner/%s@v1.0.0",
			name, manifestName, name)
	}
	s, err := parseSource(spec)
	if err != nil {
		return "", err
	}
	dir, err := fetch(s, true)
	if err != nil {
		return "", fmt.Errorf("%s\nRun 'veyl install' to fetch it", err)
	}

	m, err := loadManifest(dir)
	if err != nil {
		return "", fmt.Errorf("%s is installed but its %s is unreadable: %v", name, manifestName, err)
	}
	main := m.Main
	if main == "" {
		main = m.Name + ".vy"
	}
	entry := filepath.Join(dir, filepath.FromSlash(main))
	if _, err := os.Stat(entry); err != nil {
		return "", fmt.Errorf("package %s says its main file is %q, but that file is not there", name, main)
	}
	r.cache[name] = entry
	return entry, nil
}

// badImportName explains why a name cannot be used as a namespace, or
// returns "" when it can. Import names are written in source as
// `name.thing`, so they have to lex as a single identifier - which
// repository names frequently do not.
func badImportName(name string) string {
	if name == "" {
		return "it is empty"
	}
	if r := rune(name[0]); !unicode.IsLetter(r) && r != '_' {
		return "it must start with a letter or underscore"
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Sprintf("%q is not allowed in a name", string(r))
		}
	}
	if _, reserved := keywords[name]; reserved {
		return "it is a keyword"
	}
	return ""
}
