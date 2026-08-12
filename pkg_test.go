package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSource(t *testing.T) {
	ok := []struct{ spec, want string }{
		{"github.com/owner/repo@v1.0.0", "github.com/owner/repo@v1.0.0"},
		{"github.com/owner/repo@main", "github.com/owner/repo@main"},
		{"github.com/a-b/c.d@v1.2.3-rc1", "github.com/a-b/c.d@v1.2.3-rc1"},
		{"../mylib", "../mylib"},
		{"./mylib", "./mylib"},
	}
	for _, c := range ok {
		s, err := parseSource(c.spec)
		if err != nil {
			t.Errorf("parseSource(%q) failed: %v", c.spec, err)
			continue
		}
		if s.String() != c.want {
			t.Errorf("parseSource(%q) = %q, want %q", c.spec, s, c.want)
		}
	}

	bad := []string{
		"",
		"github.com/owner/repo",    // no version
		"owner/repo@v1",            // no host
		"gitlab.com/owner/repo@v1", // unsupported host, for now
		"github.com/owner@v1",      // no repo
		"not a real source",
	}
	for _, spec := range bad {
		if _, err := parseSource(spec); err == nil {
			t.Errorf("parseSource(%q) should have failed", spec)
		}
	}
}

func TestTarballURLPrefersTagOverBranch(t *testing.T) {
	s, err := parseSource("github.com/owner/repo@v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	urls := s.tarballURLs()
	if len(urls) != 2 {
		t.Fatalf("got %d urls, want 2", len(urls))
	}
	if !strings.Contains(urls[0], "/refs/tags/v1.0.0") {
		t.Errorf("first url should be the tag, got %s", urls[0])
	}
	if !strings.Contains(urls[1], "/refs/heads/v1.0.0") {
		t.Errorf("second url should be the branch, got %s", urls[1])
	}
}

// makeTarGz builds an archive in memory. Entry names include the
// wrapper directory GitHub puts around a repository, because stripping
// that is part of what is being tested.
func makeTarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range entries {
		hdr := &tar.Header{
			Name: name, Mode: 0o644,
			Size: int64(len(body)), Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestExtractStripsWrapperDirectory(t *testing.T) {
	dest := t.TempDir()
	data := makeTarGz(t, map[string]string{
		"repo-1.0.0/quartz.json":  `{"name":"x"}`,
		"repo-1.0.0/src/thing.qz": "pub fn f() {}",
	})
	if err := extractTarGz(data, dest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	for _, want := range []string{"quartz.json", filepath.Join("src", "thing.qz")} {
		if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
			t.Errorf("%s was not extracted: %v", want, err)
		}
	}
}

// An archive can name any path it likes, including one that climbs out
// of the destination. Writing that would let a package overwrite files
// anywhere the user can write, which is the whole of tarball safety.
func TestExtractRefusesEscapingPaths(t *testing.T) {
	dest := t.TempDir()
	data := makeTarGz(t, map[string]string{
		"repo-1.0.0/../../../evil.txt": "pwned",
	})
	if err := extractTarGz(data, dest); err == nil {
		t.Fatal("extracting a path that climbs out of the destination should fail")
	}

	outside := filepath.Join(filepath.Dir(dest), "evil.txt")
	if _, err := os.Stat(outside); err == nil {
		t.Errorf("a file was written outside the destination: %s", outside)
	}
}

func TestHashDirIsContentAddressed(t *testing.T) {
	write := func(dir string, files map[string]string) {
		for name, body := range files {
			p := filepath.Join(dir, name)
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	a, b := t.TempDir(), t.TempDir()
	files := map[string]string{"quartz.json": `{"name":"x"}`, "x.qz": "pub fn f() {}"}
	write(a, files)
	write(b, files)

	ha, err := hashDir(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := hashDir(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Error("identical contents should hash the same")
	}

	// The hash is what notices a moved tag, so a one-byte edit has to
	// change it.
	write(b, map[string]string{"x.qz": "pub fn f() { }"})
	hb2, err := hashDir(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha == hb2 {
		t.Error("changed contents should hash differently")
	}

	// So does a renamed file with the same bytes.
	c := t.TempDir()
	write(c, map[string]string{"quartz.json": `{"name":"x"}`, "renamed.qz": "pub fn f() {}"})
	hc, err := hashDir(c)
	if err != nil {
		t.Fatal(err)
	}
	if ha == hc {
		t.Error("a renamed file should change the hash")
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "src", "inner")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestName), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Found from a nested directory, so `quartz run src/main.qz` works
	// from anywhere inside a project.
	if got := findProjectRoot(deep); got != root {
		t.Errorf("from a nested dir got %q, want %q", got, root)
	}
	// And from a file rather than a directory.
	f := filepath.Join(deep, "main.qz")
	if err := os.WriteFile(f, []byte("print(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findProjectRoot(f); got != root {
		t.Errorf("from a file got %q, want %q", got, root)
	}
	// Nothing above a directory with no manifest anywhere in its parents.
	if got := findProjectRoot(t.TempDir()); got != "" {
		t.Errorf("expected no project root, got %q", got)
	}
}
