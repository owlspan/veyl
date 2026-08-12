package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The `quartz init`, `add`, `install`, `remove` and `packages` commands.
// Kept apart from pkg.go so the resolution machinery can be read
// without the command-line handling wrapped around it.

func pkgUsage() string {
	return `quartz package commands:

  quartz init [name]                 start a project here
  quartz add <source> [as <name>]    add a dependency and fetch it
  quartz remove <name>               drop a dependency
  quartz install                     fetch everything the manifest lists
  quartz packages                    list what is installed

A source is a GitHub repository at a tag:

  github.com/owner/repo@v1.0.0

or a path, for a package you are writing alongside the program:

  ../mylib
`
}

// projectHere finds the project rooted at or above the working
// directory, since every one of these commands operates on one.
func projectHere() (string, *Manifest, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	root := findProjectRoot(wd)
	if root == "" {
		return "", nil, fmt.Errorf(
			"no %s here or in any parent directory.\nStart a project with: quartz init", manifestName)
	}
	m, err := loadManifest(root)
	if err != nil {
		return "", nil, err
	}
	return root, m, nil
}

func cmdInit(args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(wd, manifestName)); err == nil {
		return fmt.Errorf("there is already a %s here", manifestName)
	}

	name := filepath.Base(wd)
	if len(args) > 0 {
		name = args[0]
	}
	m := &Manifest{
		Name:         name,
		Version:      "0.1.0",
		Main:         name + ".qz",
		Dependencies: map[string]string{},
	}
	if err := saveManifest(wd, m); err != nil {
		return err
	}
	fmt.Printf("created %s\n", filepath.Join(wd, manifestName))
	fmt.Printf("\nName it in imports as %q once it is published.\n", name)
	fmt.Println("Add a dependency with: quartz add github.com/owner/repo@v1.0.0")
	return nil
}

// cmdAdd fetches a package and records it. The import name defaults to
// the repository name, with `as` for when that would collide or read
// badly: quartz add github.com/someone/quartz-json@v1.0.0 as json.
func cmdAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("quartz add needs a source, e.g. github.com/owner/repo@v1.0.0")
	}
	root, m, err := projectHere()
	if err != nil {
		return err
	}

	spec := args[0]
	s, err := parseSource(spec)
	if err != nil {
		return err
	}

	name := s.repo
	if s.local != "" {
		name = filepath.Base(filepath.Clean(s.local))
	}
	if len(args) >= 3 && args[1] == "as" {
		name = args[2]
	} else if len(args) == 2 {
		name = args[1]
	}
	// A dependency that shadows a builtin library would be unreachable,
	// so say so now rather than let it be mysteriously ignored.
	if namespaces[name] {
		return fmt.Errorf("%q is a builtin library, so a package cannot take that name.\n"+
			"Choose another with: quartz add %s as <name>", name, spec)
	}
	// The import name is written in source as a namespace, so it has to
	// be spellable. Repositories are routinely called things like
	// quartz-strutil, and `quartz-strutil.titleCase` reads as a
	// subtraction - better to refuse now than to emit that later.
	if why := badImportName(name); why != "" {
		return fmt.Errorf("%q cannot be used as an import name: %s.\n"+
			"Pick one with: quartz add %s as <name>", name, why, spec)
	}

	fmt.Printf("adding %s as %q\n", s, name)
	dir, err := fetch(s, false)
	if err != nil {
		return err
	}
	pm, err := loadManifest(dir)
	if err != nil {
		return fmt.Errorf("fetched it, but its %s is unreadable: %v", manifestName, err)
	}

	if m.Dependencies == nil {
		m.Dependencies = map[string]string{}
	}
	if old, ok := m.Dependencies[name]; ok && old != spec {
		fmt.Printf("  replacing %s\n", old)
	}
	m.Dependencies[name] = spec
	if err := saveManifest(root, m); err != nil {
		return err
	}

	if s.local == "" {
		sum, err := hashDir(dir)
		if err != nil {
			return err
		}
		lock := loadLock(root)
		lock.Packages[name] = LockEntry{Source: spec, Version: s.version, SHA256: sum}
		if err := saveLock(root, lock); err != nil {
			return err
		}
	}

	fmt.Printf("added %s %s\n", pm.Name, pm.Version)
	fmt.Printf("\nUse it with:  import \"%s\"\n", name)
	return nil
}

func cmdRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("quartz remove needs a package name")
	}
	root, m, err := projectHere()
	if err != nil {
		return err
	}
	name := args[0]
	if _, ok := m.Dependencies[name]; !ok {
		return fmt.Errorf("%q is not a dependency of this project", name)
	}
	delete(m.Dependencies, name)
	if err := saveManifest(root, m); err != nil {
		return err
	}
	lock := loadLock(root)
	delete(lock.Packages, name)
	if err := saveLock(root, lock); err != nil {
		return err
	}
	// The cached copy is left alone: another project may share it, and
	// a download is more expensive than the disk it sits on.
	fmt.Printf("removed %s\n", name)
	return nil
}

// cmdInstall fetches everything the manifest lists and checks each one
// against the lock. A hash that does not match means the code behind a
// version tag changed, which is worth stopping for.
func cmdInstall() error {
	root, m, err := projectHere()
	if err != nil {
		return err
	}
	if len(m.Dependencies) == 0 {
		fmt.Println("no dependencies to install")
		return nil
	}

	lock := loadLock(root)
	names := sortedKeys(m.Dependencies)
	var problems []string

	for _, name := range names {
		spec := m.Dependencies[name]
		s, err := parseSource(spec)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		dir, err := fetch(s, false)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if s.local != "" {
			fmt.Printf("  %-16s %s (local)\n", name, s.local)
			continue
		}

		sum, err := hashDir(dir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if want, ok := lock.Packages[name]; ok && want.SHA256 != sum {
			problems = append(problems, fmt.Sprintf(
				"%s: the contents of %s changed since it was locked.\n"+
					"    expected %s\n    got      %s\n"+
					"    The tag was moved. Check what changed before trusting it;\n"+
					"    'quartz add %s' again if the new code is what you want.",
				name, spec, short(want.SHA256), short(sum), spec))
			continue
		}
		lock.Packages[name] = LockEntry{Source: spec, Version: s.version, SHA256: sum}
		fmt.Printf("  %-16s %s\n", name, spec)
	}

	if err := saveLock(root, lock); err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	fmt.Printf("%d package(s) ready\n", len(names))
	return nil
}

func cmdPackages() error {
	root, m, err := projectHere()
	if err != nil {
		return err
	}
	if len(m.Dependencies) == 0 {
		fmt.Printf("%s has no dependencies\n", m.Name)
		return nil
	}
	lock := loadLock(root)
	fmt.Printf("%s %s\n\n", m.Name, m.Version)
	for _, name := range sortedKeys(m.Dependencies) {
		spec := m.Dependencies[name]
		s, perr := parseSource(spec)
		state := "not installed"
		if perr == nil {
			if dir, err := s.cacheDirOrLocal(); err == nil {
				if _, err := os.Stat(filepath.Join(dir, manifestName)); err == nil {
					state = "installed"
				}
			}
		}
		if e, ok := lock.Packages[name]; ok {
			state += "  " + short(e.SHA256)
		}
		fmt.Printf("  %-16s %-44s %s\n", name, spec, state)
	}
	return nil
}

// cacheDirOrLocal is where this source lives on disk, whichever kind
// of source it is.
func (s source) cacheDirOrLocal() (string, error) {
	if s.local != "" {
		return filepath.Abs(s.local)
	}
	return s.cacheDir()
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
