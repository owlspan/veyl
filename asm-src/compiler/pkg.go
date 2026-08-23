package main

// The package manager.
//
//	veyl get sqlite                  from the official packages
//	veyl get github.com/you/thing    from any GitHub repo
//	veyl get https://.../thing.vl    from a URL
//	veyl list
//	veyl remove sqlite
//
// Packages land in ./veyl_modules/<name>/ next to the program that uses
// them, not in a machine-wide location. A project carries its own
// dependencies, two projects can want different versions of the same
// thing, and deleting the directory uninstalls everything.
//
// A package is Veyl source, and may also carry a native DLL. That is
// the whole reason `veyl get sqlite` exists rather than the compiler
// shipping SQLite: the installer stays at 5 MB, and a program that
// never touches a database never sees the library. `veyl build` copies
// any DLL in a used package next to the executable it writes.
//
// The compiler is a Go program, so this uses Go's own HTTPS rather than
// the WinHTTP path that compiled Veyl programs use. Two implementations
// of "fetch a URL" is not duplication here - they run in different
// processes, at different times, and one of them has to work before the
// other one exists.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// modulesDir is where packages are installed, relative to the working
// directory.
const modulesDir = "veyl_modules"

// officialBase is where a bare name comes from.
//
// Its own repository rather than a directory in the compiler's, so the
// registry can be public while the compiler is not, and so a package
// can be added without touching the compiler at all.
//
// HEAD rather than a branch name, so this keeps working whatever the
// registry calls its default branch.
const officialBase = "https://raw.githubusercontent.com/owlspan/veyl-packages/HEAD/"

// manifestName lists the files a package is made of. A package without
// one is a single .vl file named after the package.
const manifestName = "veyl.pkg"

// pkgClient has a timeout, because a fetch that hangs forever looks
// exactly like a compiler that has crashed.
var pkgClient = &http.Client{Timeout: 30 * time.Second}

// resolveSpec turns what the user typed into a base URL and a package
// name.
func resolveSpec(spec string) (base, name string, err error) {
	switch {
	case strings.HasPrefix(spec, "http://"), strings.HasPrefix(spec, "https://"):
		if strings.HasSuffix(spec, ".vl") {
			// A direct file: its own base, named after the file.
			name = strings.TrimSuffix(filepath.Base(spec), ".vl")
			return spec, name, nil
		}
		return strings.TrimSuffix(spec, "/") + "/", filepath.Base(strings.TrimSuffix(spec, "/")), nil

	case strings.HasPrefix(spec, "github.com/"):
		spec = strings.TrimPrefix(spec, "github.com/")
		fallthrough

	case strings.Count(spec, "/") >= 1:
		parts := strings.Split(strings.Trim(spec, "/"), "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("expected user/repo, got %q", spec)
		}
		user, repo := parts[0], parts[1]
		// HEAD rather than main or master, so this works whatever the
		// repository calls its default branch.
		b := "https://raw.githubusercontent.com/" + user + "/" + repo + "/HEAD/"
		if len(parts) > 2 {
			b += strings.Join(parts[2:], "/") + "/"
			return b, parts[len(parts)-1], nil
		}
		return b, repo, nil

	case spec == "":
		return "", "", fmt.Errorf("nothing to get")

	default:
		if strings.ContainsAny(spec, `\:`) {
			return "", "", fmt.Errorf("%q does not look like a package name or a url", spec)
		}
		return officialBase + spec + "/", spec, nil
	}
}

func fetch(url string) ([]byte, error) {
	resp, err := pkgClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: server replied %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// pkgGet installs one package.
func pkgGet(spec string) error {
	base, name, err := resolveSpec(spec)
	if err != nil {
		return err
	}

	var files []string
	if strings.HasSuffix(base, ".vl") {
		// A direct link to one file. The base is the file itself.
		body, err := fetch(base)
		if err != nil {
			return err
		}
		return writePackage(name, map[string][]byte{name + ".vl": body})
	}

	// A manifest if there is one, otherwise the single obvious file.
	if body, err := fetch(base + manifestName); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			files = append(files, line)
		}
		if len(files) == 0 {
			return fmt.Errorf("%s lists no files", base+manifestName)
		}
	} else {
		files = []string{name + ".vl"}
	}

	contents := map[string][]byte{}
	for _, f := range files {
		// A manifest naming ../ or an absolute path would write outside
		// the package directory.
		if strings.Contains(f, "..") || filepath.IsAbs(f) || strings.ContainsAny(f, `\:`) {
			return fmt.Errorf("%s names an unsafe path: %q", manifestName, f)
		}
		body, err := fetch(base + f)
		if err != nil {
			return err
		}
		contents[f] = body
	}
	return writePackage(name, contents)
}

func writePackage(name string, files map[string][]byte) error {
	dir := filepath.Join(modulesDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for f := range files {
		names = append(names, f)
	}
	sort.Strings(names)

	var total int
	for _, f := range names {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[f], 0o644); err != nil {
			return err
		}
		total += len(files[f])
		fmt.Printf("  %s (%s)\n", f, human(len(files[f])))
	}
	fmt.Printf("installed %s into %s (%s)\n", name, dir, human(total))
	return nil
}

func human(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}

func pkgList() error {
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no packages installed")
			return nil
		}
		return err
	}
	found := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found = true
		var size int64
		var count int
		_ = filepath.Walk(filepath.Join(modulesDir, e.Name()),
			func(_ string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					size += info.Size()
					count++
				}
				return nil
			})
		fmt.Printf("%-20s %d file(s), %s\n", e.Name(), count, human(int(size)))
	}
	if !found {
		fmt.Println("no packages installed")
	}
	return nil
}

func pkgRemove(name string) error {
	if name == "" || strings.ContainsAny(name, `/\:.`) {
		return fmt.Errorf("%q is not a package name", name)
	}
	dir := filepath.Join(modulesDir, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%s is not installed", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", name)
	return nil
}

// packageDLLs lists every DLL in every installed package, so a build
// can put them beside the executable.
//
// Every installed package rather than only the imported ones: working
// out which package an import came from means threading that back from
// the resolver, and a stray DLL next to an executable is harmless where
// a missing one is a program that will not start.
func packageDLLs(root string) []string {
	var out []string
	dir := filepath.Join(root, modulesDir)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".dll") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// requireSQLite checks that the sqlite package is installed before a
// program that uses db.* is built.
//
// Without this the build succeeds and the executable refuses to start
// with 0xC0000135 and no message, because Windows resolves imports
// before main runs and says nothing useful about which one was
// missing. Catching it here costs a directory listing and turns it into
// a sentence naming the fix.
func requireSQLite(root string) error {
	for _, dll := range packageDLLs(root) {
		if strings.EqualFold(filepath.Base(dll), "sqlite3.dll") {
			return nil
		}
	}
	return fmt.Errorf("this program uses db.*, which needs SQLite.\n" +
		"        Install it next to the program with:  veyl get sqlite")
}

// copyDLLsBeside puts each one next to the executable, unless a file of
// that name is already there and identical in size.
func copyDLLsBeside(exePath string, dlls []string) {
	destDir := filepath.Dir(exePath)
	for _, src := range dlls {
		dest := filepath.Join(destDir, filepath.Base(src))
		if s, err := os.Stat(src); err == nil {
			if d, err := os.Stat(dest); err == nil && d.Size() == s.Size() {
				continue
			}
		}
		body, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dest, body, 0o644); err == nil {
			fmt.Printf("copied %s\n", filepath.Base(src))
		}
	}
}
