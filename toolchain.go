package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Quartz compiles to Go source and then shells out to the Go toolchain,
// so a Go compiler has to exist somewhere on the machine. Until now the
// driver ran a bare `go build` and relied entirely on PATH, which meant
// a machine without Go failed with Windows' own
//
//	exec: "go": executable file not found in %PATH%
//
// That names the wrong problem. Someone who installed Quartz has no
// reason to know it needs Go at all, let alone that PATH is involved.
//
// The installer ships a private copy of the toolchain next to
// quartz.exe. It is deliberately kept off PATH: a developer who already
// has their own Go should keep using it, and two Go versions fighting
// over PATH is a support problem nobody wants. So the private copy is
// found by location instead.

// goToolchain is a located Go compiler and how it was found.
type goToolchain struct {
	exe    string // full path to go.exe
	source string // human-readable explanation of where it came from
}

// findGo locates a Go toolchain, in order of preference:
//
//  1. $QUARTZ_GO, for anyone who wants to force a specific one
//  2. a private toolchain shipped beside quartz.exe by the installer
//  3. PATH, which is the normal case on a development machine
//
// The bundled copy beats PATH so that an installed Quartz keeps working
// the same way regardless of what else the machine picks up later.
func findGo() (goToolchain, error) {
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}

	if forced := os.Getenv("QUARTZ_GO"); forced != "" {
		if isExecutable(forced) {
			return goToolchain{forced, "QUARTZ_GO"}, nil
		}
		return goToolchain{}, fmt.Errorf("QUARTZ_GO is set to %s, but there is no runnable Go compiler there", forced)
	}

	if dir, err := os.Executable(); err == nil {
		// Resolve symlinks so a linked quartz still finds its own files.
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		bundled := filepath.Join(filepath.Dir(dir), "go", "bin", name)
		if isExecutable(bundled) {
			return goToolchain{bundled, "the copy installed with Quartz"}, nil
		}
	}

	if found, err := exec.LookPath(name); err == nil {
		return goToolchain{found, "PATH"}, nil
	}

	return goToolchain{}, errNoGo()
}

// errNoGo explains the problem in terms of Quartz rather than PATH, and
// says what to do about it.
func errNoGo() error {
	return fmt.Errorf(`Quartz needs the Go toolchain to build programs, and could not find one.

Looked for it in three places:
  - the QUARTZ_GO environment variable            (not set)
  - a copy installed alongside quartz.exe         (not there)
  - PATH                                          (no 'go' on it)

Either install Go from https://go.dev/dl, or reinstall Quartz and leave
the bundled toolchain option ticked. Run 'quartz doctor' afterwards to
confirm it was found.`)
}

// isExecutable reports whether path is a file that could be run. On
// Windows the executable bit does not exist, so the name is the only
// signal available and a regular file is taken at its word.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// doctor reports what Quartz can see, so that "it doesn't work" on
// someone else's machine is a question with an answer.
func doctor() error {
	fmt.Printf("quartz %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)

	if exe, err := os.Executable(); err == nil {
		fmt.Printf("  installed at   %s\n", exe)
	}

	tc, err := findGo()
	if err != nil {
		fmt.Println("  Go toolchain   NOT FOUND")
		fmt.Println()
		return err
	}

	fmt.Printf("  Go toolchain   %s\n", tc.exe)
	fmt.Printf("  found via      %s\n", tc.source)

	out, err := exec.Command(tc.exe, "version").Output()
	if err != nil {
		return fmt.Errorf("found a Go compiler at %s but could not run it: %v", tc.exe, err)
	}
	fmt.Printf("  Go version     %s\n", strings.TrimSpace(string(out)))
	fmt.Println("\nEverything Quartz needs is present.")
	return nil
}
