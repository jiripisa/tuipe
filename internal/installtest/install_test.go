// Package installtest checks the release installers and the release archive
// layout against the rest of the code base. It has no production code.
package installtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/upgrade"
)

const fakeBinary = "#!/bin/sh\necho fake tuipe\n"

func skipUnlessUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available")
	}
}

func scriptPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeRelease serves a GitHub-style "latest release" for the given asset name.
func fakeRelease(t *testing.T, goos, goarch string, badSum bool) *httptest.Server {
	t.Helper()
	archive := tarGz(t, map[string]string{"tuipe": fakeBinary, "README.md": "hi\n"})
	name := upgrade.AssetName("1.2.3", goos, goarch)
	checksum := sum(archive)
	if badSum {
		checksum = strings.Repeat("0", 64)
	}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/jiripisa/tuipe/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		// Compact JSON on purpose: the installer must not rely on line breaks.
		fmt.Fprintf(w, `{"id":1,"tag_name":"v1.2.3","assets":[`+
			`{"name":"checksums.txt","url":"x","browser_download_url":"%[1]s/dl/checksums.txt"},`+
			`{"name":%[2]q,"url":"x","browser_download_url":"%[1]s/dl/%[2]s"},`+
			`{"name":"other","url":"x","browser_download_url":"%[1]s/dl/other"}]}`, srv.URL, name)
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  other\n%s  %s\n", strings.Repeat("a", 64), checksum, name)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func hostPlatform() (string, string) {
	arch := runtime.GOARCH
	return runtime.GOOS, arch
}

type result struct {
	out  string
	err  error
	dir  string
	home string
}

// run executes install.sh with a scratch HOME. extraPath dirs are put first
// on PATH (used for fake uname/sysctl); sh selects the interpreter.
func run(t *testing.T, sh string, api string, env []string, extraPath string) result {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "bin-here")
	cmd := exec.Command(sh, scriptPath(t))
	path := os.Getenv("PATH")
	if extraPath != "" {
		path = extraPath + string(os.PathListSeparator) + path
	}
	cmd.Env = append([]string{
		"PATH=" + path, "HOME=" + home, "TUIPE_API_BASE=" + api, "TUIPE_INSTALL_DIR=" + dir, "SHELL=/bin/zsh",
	}, env...)
	out, err := cmd.CombinedOutput()
	return result{out: string(out), err: err, dir: dir, home: home}
}

func TestInstallScriptAgainstFakeRelease(t *testing.T) {
	skipUnlessUnix(t)
	goos, goarch := hostPlatform()
	if (goos != "darwin" && goos != "linux") || (goarch != "arm64" && goarch != "amd64") {
		t.Skip("unsupported host")
	}
	srv := fakeRelease(t, goos, goarch, false)
	r := run(t, "sh", srv.URL, nil, "")
	if r.err != nil {
		t.Fatalf("install failed: %v\n%s", r.err, r.out)
	}
	bin := filepath.Join(r.dir, "tuipe")
	st, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("binary not installed: %v\n%s", err, r.out)
	}
	if st.Mode()&0o111 == 0 {
		t.Fatalf("binary not executable: %v", st.Mode())
	}
	got, _ := os.ReadFile(bin)
	if string(got) != fakeBinary {
		t.Fatalf("wrong content: %q", got)
	}
	entries, _ := os.ReadDir(r.dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files in install dir: %v", entries)
	}
	if !strings.Contains(r.out, "tuipe nainstalováno. Spusťte: ") {
		t.Fatalf("missing final line:\n%s", r.out)
	}
}

func TestInstallScriptRejectsBadChecksum(t *testing.T) {
	skipUnlessUnix(t)
	goos, goarch := hostPlatform()
	if (goos != "darwin" && goos != "linux") || (goarch != "arm64" && goarch != "amd64") {
		t.Skip("unsupported host")
	}
	srv := fakeRelease(t, goos, goarch, true)
	r := run(t, "sh", srv.URL, nil, "")
	if r.err == nil {
		t.Fatalf("expected failure, got success:\n%s", r.out)
	}
	if !strings.Contains(r.out, "ontroln") {
		t.Fatalf("expected a Czech checksum message:\n%s", r.out)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "tuipe")); err == nil {
		t.Fatal("binary was installed despite the checksum failure")
	}
	if entries, _ := os.ReadDir(r.dir); len(entries) != 0 {
		t.Fatalf("install dir not clean: %v", entries)
	}
}

func TestInstallScriptPrintsPathHelpWhenNeeded(t *testing.T) {
	skipUnlessUnix(t)
	goos, goarch := hostPlatform()
	if (goos != "darwin" && goos != "linux") || (goarch != "arm64" && goarch != "amd64") {
		t.Skip("unsupported host")
	}
	srv := fakeRelease(t, goos, goarch, false)

	cases := []struct{ shell, rc, line string }{
		{"/bin/zsh", "~/.zshrc", `export PATH="%s:$PATH"`},
		{"/bin/bash", "~/.bashrc", `export PATH="%s:$PATH"`},
		{"/usr/bin/fish", "~/.config/fish/config.fish", `fish_add_path "%s"`},
	}
	for _, c := range cases {
		t.Run(filepath.Base(c.shell), func(t *testing.T) {
			r := run(t, "sh", srv.URL, []string{"SHELL=" + c.shell}, "")
			if r.err != nil {
				t.Fatalf("%v\n%s", r.err, r.out)
			}
			if !strings.Contains(r.out, c.rc) || !strings.Contains(r.out, fmt.Sprintf(c.line, r.dir)) {
				t.Fatalf("missing PATH help for %s:\n%s", c.shell, r.out)
			}
			if !strings.Contains(r.out, "Spusťte: "+filepath.Join(r.dir, "tuipe")) {
				t.Fatalf("expected the full path in the last line:\n%s", r.out)
			}
			// No rc file is touched.
			if _, err := os.Stat(filepath.Join(r.home, ".zshrc")); err == nil {
				t.Fatal("installer wrote an rc file")
			}
		})
	}

	t.Run("on PATH", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, "bin-here")
		cmd := exec.Command("sh", scriptPath(t))
		cmd.Env = []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + home,
			"TUIPE_API_BASE=" + srv.URL, "TUIPE_INSTALL_DIR=" + dir, "SHELL=/bin/zsh"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "export PATH") {
			t.Fatalf("PATH help printed although dir is on PATH:\n%s", out)
		}
		if !strings.HasSuffix(strings.TrimSpace(string(out)), "tuipe nainstalováno. Spusťte: tuipe") {
			t.Fatalf("wrong last line:\n%s", out)
		}
	})
}

// stubs writes fake uname/sysctl executables into a fresh dir.
func stubs(t *testing.T, osName, machine, sysctlOut string) string {
	t.Helper()
	dir := t.TempDir()
	uname := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in -s) echo %s;; -m) echo %s;; *) echo %s;; esac\n", osName, machine, osName)
	if err := os.WriteFile(filepath.Join(dir, "uname"), []byte(uname), 0o755); err != nil {
		t.Fatal(err)
	}
	sysctl := "#!/bin/sh\n"
	if sysctlOut == "" {
		sysctl += "exit 1\n"
	} else {
		sysctl += "echo " + sysctlOut + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "sysctl"), []byte(sysctl), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallScriptDetectsPlatform(t *testing.T) {
	skipUnlessUnix(t)
	cases := []struct {
		name, osName, machine, sysctl, wantOS, wantArch string
	}{
		{"linux x86_64", "Linux", "x86_64", "", "linux", "amd64"},
		{"linux aarch64", "Linux", "aarch64", "", "linux", "arm64"},
		{"mac arm64", "Darwin", "arm64", "", "darwin", "arm64"},
		{"mac intel", "Darwin", "x86_64", "0", "darwin", "amd64"},
		{"mac rosetta", "Darwin", "x86_64", "1", "darwin", "arm64"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := fakeRelease(t, c.wantOS, c.wantArch, false)
			r := run(t, "sh", srv.URL, nil, stubs(t, c.osName, c.machine, c.sysctl))
			if r.err != nil {
				t.Fatalf("%v\n%s", r.err, r.out)
			}
			if _, err := os.Stat(filepath.Join(r.dir, "tuipe")); err != nil {
				t.Fatalf("not installed: %v", err)
			}
		})
	}
	t.Run("unsupported", func(t *testing.T) {
		srv := fakeRelease(t, "linux", "amd64", false)
		for _, m := range []struct{ os, arch string }{{"FreeBSD", "x86_64"}, {"Linux", "riscv64"}} {
			r := run(t, "sh", srv.URL, nil, stubs(t, m.os, m.arch, ""))
			if r.err == nil || !strings.Contains(r.out, "epodporovan") {
				t.Fatalf("%v: expected a Czech unsupported message:\n%s", m, r.out)
			}
		}
	})
}

func TestInstallScriptFailsCleanlyWithoutRelease(t *testing.T) {
	skipUnlessUnix(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	r := run(t, "sh", srv.URL, nil, "")
	if r.err == nil || !strings.Contains(r.out, "tuipe") {
		t.Fatalf("expected a non-zero exit with a message:\n%s", r.out)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "tuipe")); err == nil {
		t.Fatal("installed without a release")
	}
}

func TestInstallScriptIsPOSIX(t *testing.T) {
	skipUnlessUnix(t)
	for _, sh := range []string{"sh", "dash"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		if out, err := exec.Command(sh, "-n", scriptPath(t)).CombinedOutput(); err != nil {
			t.Fatalf("%s -n: %v\n%s", sh, err, out)
		}
	}
	if _, err := exec.LookPath("dash"); err == nil {
		srv := fakeRelease(t, runtime.GOOS, runtime.GOARCH, false)
		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			r := run(t, "dash", srv.URL, nil, "")
			if r.err != nil {
				t.Fatalf("dash run: %v\n%s", r.err, r.out)
			}
		}
	}
	b, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if last := lines[len(lines)-1]; last != `main "$@"` {
		t.Fatalf("last line must call main, got %q", last)
	}
	if !strings.Contains(string(b), "set -eu") {
		t.Fatal("missing set -eu")
	}
}

// TestUpgradeAcceptsGoReleaserArchive proves internal/upgrade accepts the
// archive layout GoReleaser produces. It needs a snapshot build in dist/
// (goreleaser release --snapshot --clean) and skips without one.
func TestUpgradeAcceptsGoReleaserArchive(t *testing.T) {
	dist, _ := filepath.Abs("../../dist")
	goos, goarch := runtime.GOOS, runtime.GOARCH
	matches, _ := filepath.Glob(filepath.Join(dist, fmt.Sprintf("tuipe_*_%s_%s.tar.gz", goos, goarch)))
	if goos == "windows" {
		matches, _ = filepath.Glob(filepath.Join(dist, fmt.Sprintf("tuipe_*_%s_%s.zip", goos, goarch)))
	}
	if len(matches) != 1 {
		t.Skip("no snapshot archive in dist/; run goreleaser release --snapshot --clean")
	}
	archivePath := matches[0]
	sums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(archivePath)
	version := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(name, "tuipe_"), ".tar.gz"), ".zip")
	version = strings.TrimSuffix(strings.TrimSuffix(version, "_"+goarch), "_"+goos)
	if name != upgrade.AssetName(version, goos, goarch) {
		t.Fatalf("archive name %q does not match upgrade.AssetName(%q, %q, %q)", name, version, goos, goarch)
	}

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/a", func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/s", func(w http.ResponseWriter, _ *http.Request) { w.Write(sums) })
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := &upgrade.Client{APIBase: srv.URL, HTTP: srv.Client(), Repo: "jiripisa/tuipe"}
	rel := upgrade.Release{Tag: "v" + version, Assets: map[string]string{name: srv.URL + "/a", "checksums.txt": srv.URL + "/s"}}
	exe := filepath.Join(t.TempDir(), "tuipe")
	if goos == "windows" {
		exe += ".exe"
	}
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.Apply(context.Background(), rel, exe, goos, goarch); err != nil {
		t.Fatalf("upgrade rejected the GoReleaser archive: %v", err)
	}
	// The installed binary is the one from the archive, with ldflags applied.
	out, err := exec.Command(exe, "version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(out), "tuipe v") {
		t.Fatalf("installed binary does not report a release version: %v\n%s", err, out)
	}
}
