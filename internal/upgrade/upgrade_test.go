package upgrade_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/ui/text"
	"github.com/jiripisa/tuipe/internal/upgrade"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%q does not contain %q", s, sub)
	}
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = old })
}

func header(goos string) string {
	switch goos {
	case "windows":
		return "MZ"
	case "darwin":
		return "\xcf\xfa\xed\xfe"
	}
	return "\x7fELF"
}

func tarGz(t *testing.T, files map[string]string, mutate func(*tar.Header)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		h := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if mutate != nil {
			mutate(h)
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(body))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type fake struct {
	srv       *httptest.Server
	downloads int
	archive   []byte
	checksum  string // overrides the computed one when set
	apiStatus int
	dlStatus  int
	tag       string
}

func newFake(t *testing.T, tag string, archive []byte) *fake {
	t.Helper()
	f := &fake{archive: archive, tag: tag}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/jiripisa/tuipe/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Accept") != "application/vnd.github+json" {
			http.Error(w, "bad headers", 400)
			return
		}
		if f.apiStatus != 0 {
			http.Error(w, "x", f.apiStatus)
			return
		}
		name := upgrade.AssetName(strings.TrimPrefix(f.tag, "v"), runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":"checksums.txt","browser_download_url":%q}]}`,
			f.tag, name, f.srv.URL+"/dl/archive", f.srv.URL+"/dl/checksums.txt")
	})
	mux.HandleFunc("/dl/archive", func(w http.ResponseWriter, r *http.Request) {
		f.downloads++
		if f.dlStatus != 0 {
			http.Error(w, "x", f.dlStatus)
			return
		}
		w.Write(f.archive)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		name := upgrade.AssetName(strings.TrimPrefix(f.tag, "v"), runtime.GOOS, runtime.GOARCH)
		s := f.checksum
		if s == "" {
			s = sum(f.archive)
		}
		fmt.Fprintf(w, "%s  other_file\n%s  %s\n", strings.Repeat("0", 64), s, name)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) client() *upgrade.Client {
	return &upgrade.Client{APIBase: f.srv.URL, HTTP: f.srv.Client(), Repo: "jiripisa/tuipe"}
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "tuipe.exe"
	}
	return "tuipe"
}

func archiveFor(t *testing.T, body string) []byte {
	t.Helper()
	files := map[string]string{binName(): header(runtime.GOOS) + body, "README.md": "hi"}
	if runtime.GOOS == "windows" {
		return zipOf(t, files)
	}
	return tarGz(t, files, nil)
}

func installOld(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), binName())
	if err := os.WriteFile(exe, []byte("OLD"), 0o750); err != nil {
		t.Fatal(err)
	}
	return exe
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAssetNames(t *testing.T) {
	eq(t, upgrade.AssetName("1.2.3", "darwin", "arm64"), "tuipe_1.2.3_darwin_arm64.tar.gz")
	eq(t, upgrade.AssetName("1.2.3", "windows", "amd64"), "tuipe_1.2.3_windows_amd64.zip")
}

func TestNewer(t *testing.T) {
	eq(t, upgrade.Newer("v1.2.3", "v1.10.0"), true)
	eq(t, upgrade.Newer("v1.2.3", "v1.2.3"), false)
	eq(t, upgrade.Newer("v1.10.0", "v1.2.3"), false)
	eq(t, upgrade.Newer("dev", "v0.1.0"), true)
	eq(t, upgrade.Newer("v1.2.3", "v1.3.0-rc1"), false)
	eq(t, upgrade.Newer("v1.2.3", "garbage"), false)
	eq(t, upgrade.Newer("dev", "dev"), false)
}

func TestHappyPathReplacesBinary(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, false, &out); err != nil {
		t.Fatal(err)
	}
	eq(t, read(t, exe), header(runtime.GOOS)+"NEW")
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(exe)
		eq(t, st.Mode().Perm(), os.FileMode(0o750))
	}
	contains(t, out.String(), "v1.0.0")
	contains(t, out.String(), "v1.1.0")
	entries, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range entries {
		if e.Name() != binName() && !strings.HasSuffix(e.Name(), ".old") {
			t.Fatalf("leftover file %s", e.Name())
		}
	}
}

func TestFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	link := filepath.Join(t.TempDir(), "tuipe-link")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	if err := upgrade.Run(context.Background(), f.client(), link, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(link)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	eq(t, read(t, exe), header(runtime.GOOS)+"NEW")
}

func TestChecksumMismatchLeavesBinaryUntouched(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	f.checksum = strings.Repeat("a", 64)
	exe := installOld(t)
	err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "Kontrolní součet")
	eq(t, read(t, exe), "OLD")
}

func TestDownloadFailureLeavesBinaryUntouched(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	f.dlStatus = 500
	exe := installOld(t)
	err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "Stažení")
	eq(t, read(t, exe), "OLD")
}

func TestAlreadyLatestDoesNothing(t *testing.T) {
	setVersion(t, "v1.1.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, false, &out); err != nil {
		t.Fatal(err)
	}
	eq(t, f.downloads, 0)
	eq(t, read(t, exe), "OLD")
	contains(t, out.String(), "nejnovější")
}

func TestCheckFlagDoesNotDownload(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, true, &out); err != nil {
		t.Fatal(err)
	}
	eq(t, f.downloads, 0)
	eq(t, read(t, exe), "OLD")
	contains(t, out.String(), "v1.1.0")
}

func TestUnwritableBinarySaysWhatToDo(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("cannot make a directory unwritable")
	}
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	dir := filepath.Dir(exe)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "sudo")
	eq(t, read(t, exe), "OLD")
}

func TestGoInstallBuildAdvisesGoInstall(t *testing.T) {
	setVersion(t, "dev")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "go install github.com/jiripisa/tuipe/cmd/tuipe@latest")
	eq(t, f.downloads, 0)
	eq(t, read(t, exe), "OLD")
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, true, &out); err != nil {
		t.Fatal(err)
	}
	contains(t, out.String(), "v1.1.0")
}

func TestOfflineMessage(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", nil)
	c := f.client()
	f.srv.Close()
	err := upgrade.Run(context.Background(), c, installOld(t), false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), text.UpgradeOffline)
}

func TestRateLimitedMessage(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", nil)
	f.apiStatus = 403
	err := upgrade.Run(context.Background(), f.client(), installOld(t), false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), text.UpgradeRateLimited)
}

func TestNoReleaseYetMessage(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", nil)
	f.apiStatus = 404
	err := upgrade.Run(context.Background(), f.client(), installOld(t), false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), text.UpgradeNoRelease)
}

func TestArchiveWithoutBinaryIsRejected(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", tarGz(t, map[string]string{"README.md": "x"}, nil))
	if runtime.GOOS == "windows" {
		f.archive = zipOf(t, map[string]string{"README.md": "x"})
	}
	exe := installOld(t)
	err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "archiv")
	eq(t, read(t, exe), "OLD")
}

func TestHostileArchivesAreRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar archives are used on unix only")
	}
	good := header(runtime.GOOS) + "NEW"
	cases := map[string][]byte{
		"traversal": tarGz(t, map[string]string{"../tuipe": good, "tuipe": good}, nil),
		"absolute":  tarGz(t, map[string]string{"/etc/tuipe": good, "tuipe": good}, nil),
		"symlink": tarGz(t, map[string]string{"tuipe": ""}, func(h *tar.Header) {
			h.Typeflag = tar.TypeSymlink
			h.Linkname = "/bin/sh"
			h.Size = 0
		}),
		"nested":     tarGz(t, map[string]string{"dir/tuipe": good}, nil),
		"empty":      tarGz(t, map[string]string{"tuipe": ""}, nil),
		"wrongmagic": tarGz(t, map[string]string{"tuipe": "#!/bin/sh\necho hi"}, nil),
	}
	for name, archive := range cases {
		t.Run(name, func(t *testing.T) {
			setVersion(t, "v1.0.0")
			f := newFake(t, "v1.1.0", archive)
			exe := installOld(t)
			if err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{}); err == nil {
				t.Fatal("expected error")
			}
			eq(t, read(t, exe), "OLD")
		})
	}
}
