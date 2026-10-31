package upgrade_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
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

// header returns a minimal but real executable header for the platform.
func header(goos, goarch string) string {
	le := binary.LittleEndian
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, le, v) }
	switch goos {
	case "windows":
		machine := uint16(0x8664)
		if goarch == "arm64" {
			machine = 0xAA64
		}
		dos := make([]byte, 0x40)
		copy(dos, "MZ")
		le.PutUint32(dos[0x3c:], 0x40)
		b.Write(dos)
		b.WriteString("PE\x00\x00")
		w(machine)
		w(uint16(0))    // sections
		w(uint32(0))    // timestamp
		w(uint32(0))    // symbols pointer
		w(uint32(0))    // symbols count
		w(uint16(0))    // optional header size
		w(uint16(0x22)) // characteristics
	case "darwin":
		cpu := uint32(0x01000007)
		if goarch == "arm64" {
			cpu = 0x0100000c
		}
		w(uint32(0xfeedfacf))
		w(cpu)
		w([]uint32{0, 2, 0, 0, 0, 0})
	default:
		machine := uint16(62)
		if goarch == "arm64" {
			machine = 183
		}
		b.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		w(uint16(2))
		w(machine)
		w(uint32(1))
		w([]uint64{0, 0, 0})
		w(uint32(0))
		w([]uint16{64, 56, 0, 64, 0, 0})
	}
	return b.String()
}

func otherArch() string {
	if runtime.GOARCH == "amd64" {
		return "arm64"
	}
	return "amd64"
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
	sums      int
	truncate  bool
	archive   []byte
	checksum  string // overrides the computed one when set
	apiStatus int
	dlStatus  int
	tag       string
	requests  int
}

func newFake(t *testing.T, tag string, archive []byte) *fake {
	t.Helper()
	f := &fake{archive: archive, tag: tag}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/jiripisa/tuipe/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
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
		if f.truncate {
			w.Header().Set("Content-Length", "100000")
			w.Write(f.archive[:10])
			panic(http.ErrAbortHandler)
		}
		if f.dlStatus != 0 {
			http.Error(w, "x", f.dlStatus)
			return
		}
		w.Write(f.archive)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		f.sums++
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
	files := map[string]string{binName(): header(runtime.GOOS, runtime.GOARCH) + body, "README.md": "hi"}
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
	eq(t, upgrade.Newer("v1.0.0-rc1", "v1.0.0"), true)
	eq(t, upgrade.Newer("v1.0.0", "v1.0.0-rc1"), false)
}

func TestHappyPathReplacesBinary(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	exe := installOld(t)
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, false, &out); err != nil {
		t.Fatal(err)
	}
	eq(t, read(t, exe), header(runtime.GOOS, runtime.GOARCH)+"NEW")
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
	eq(t, read(t, exe), header(runtime.GOOS, runtime.GOARCH)+"NEW")
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
	eq(t, f.sums, 0)
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
	contains(t, err.Error(), "dev")
	eq(t, f.requests, 0)
	eq(t, f.downloads, 0)
	eq(t, read(t, exe), "OLD")
	var out bytes.Buffer
	if err := upgrade.Run(context.Background(), f.client(), exe, true, &out); err != nil {
		t.Fatal(err)
	}
	contains(t, out.String(), "v1.1.0")
	contains(t, out.String(), "go install github.com/jiripisa/tuipe/cmd/tuipe@latest")
	if strings.Contains(out.String(), "tuipe upgrade") {
		t.Fatal("must not advise tuipe upgrade on a go install build")
	}
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
	good := header(runtime.GOOS, runtime.GOARCH) + "NEW"
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
		"wrongarch":  tarGz(t, map[string]string{"tuipe": header(runtime.GOOS, otherArch()) + "NEW"}, nil),
		"fatjunk":    tarGz(t, map[string]string{"tuipe": "\xca\xfe\xba\xbe" + strings.Repeat("junk", 20)}, nil),
		"elfjunk":    tarGz(t, map[string]string{"tuipe": "\x7fELF" + strings.Repeat("junk", 20)}, nil),
		"duplicate":  dupTar(t, good),
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

func dupTar(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i := 0; i < 2; i++ {
		tw.WriteHeader(&tar.Header{Name: "tuipe", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// serve publishes archive under assetName with the given checksums.txt body
// (empty means: omit the asset).
func serve(t *testing.T, assetName string, archive []byte, sums string, withSums bool) (*upgrade.Client, upgrade.Release) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	mux.HandleFunc("/s", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sums) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	rel := upgrade.Release{Tag: "v1.1.0", Assets: map[string]string{assetName: srv.URL + "/a"}}
	if withSums {
		rel.Assets["checksums.txt"] = srv.URL + "/s"
	}
	return &upgrade.Client{APIBase: srv.URL, HTTP: srv.Client(), Repo: "jiripisa/tuipe"}, rel
}

func TestZipHostileArchivesAreRejected(t *testing.T) {
	good := header("windows", "amd64") + "NEW"
	cases := map[string][]byte{
		"traversal": zipOf(t, map[string]string{"../tuipe.exe": good, "tuipe.exe": good}),
		"backslash": zipOf(t, map[string]string{`..\tuipe.exe`: good, "tuipe.exe": good}),
		"absolute":  zipOf(t, map[string]string{"/tuipe.exe": good}),
		"nested":    zipOf(t, map[string]string{"dir/tuipe.exe": good}),
		"empty":     zipOf(t, map[string]string{"tuipe.exe": ""}),
		"wrongarch": zipOf(t, map[string]string{"tuipe.exe": header("windows", "arm64") + "NEW"}),
		"junk":      zipOf(t, map[string]string{"tuipe.exe": "MZ" + strings.Repeat("junk", 20)}),
	}
	name := upgrade.AssetName("1.1.0", "windows", "amd64")
	for n, archive := range cases {
		t.Run(n, func(t *testing.T) {
			c, rel := serve(t, name, archive, sum(archive)+"  "+name+"\n", true)
			exe := installOld(t)
			if err := c.Apply(context.Background(), rel, exe, "windows", "amd64"); err == nil {
				t.Fatal("expected error")
			}
			eq(t, read(t, exe), "OLD")
		})
	}
}

func TestChecksumListProblems(t *testing.T) {
	name := upgrade.AssetName("1.1.0", "linux", "amd64")
	archive := tarGz(t, map[string]string{"tuipe": header("linux", "amd64") + "NEW"}, nil)
	h := sum(archive)
	cases := map[string]struct {
		sums     string
		withSums bool
	}{
		"missing file": {"", false},
		"no line":      {h + "  other.tar.gz\n", true},
		"lookalike":    {h + "  x" + name + "\n" + h + "  " + name + ".sig\n", true},
	}
	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			c, rel := serve(t, name, archive, tc.sums, tc.withSums)
			exe := installOld(t)
			if err := c.Apply(context.Background(), rel, exe, "linux", "amd64"); err == nil {
				t.Fatal("expected error")
			}
			eq(t, read(t, exe), "OLD")
		})
	}
}

func TestTruncatedDownloadLeavesBinaryUntouched(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "v1.1.0", archiveFor(t, "NEW"))
	f.truncate = true
	exe := installOld(t)
	if err := upgrade.Run(context.Background(), f.client(), exe, false, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
	eq(t, read(t, exe), "OLD")
}

func TestSwapFailureRemovesTempFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename over a directory behaves differently")
	}
	name := upgrade.AssetName("1.1.0", "linux", "amd64")
	archive := tarGz(t, map[string]string{"tuipe": header("linux", "amd64") + "NEW"}, nil)
	c, rel := serve(t, name, archive, sum(archive)+"  "+name+"\n", true)
	dir := t.TempDir()
	target := filepath.Join(dir, "tuipe")
	if err := os.Mkdir(target, 0o755); err != nil { // a directory cannot be replaced by a file
		t.Fatal(err)
	}
	if err := c.Apply(context.Background(), rel, target, "linux", "amd64"); err == nil {
		t.Fatal("expected error")
	}
	entries, _ := os.ReadDir(dir)
	eq(t, len(entries), 1)
}

func TestBadTagIsAnHonestError(t *testing.T) {
	setVersion(t, "v1.0.0")
	f := newFake(t, "nightly", archiveFor(t, "NEW"))
	var out bytes.Buffer
	err := upgrade.Run(context.Background(), f.client(), installOld(t), true, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(out.String(), "nejnovější") {
		t.Fatal("claimed to be up to date")
	}
}

func TestRefusesPlainHTTPAssets(t *testing.T) {
	name := upgrade.AssetName("1.1.0", "linux", "amd64")
	archive := tarGz(t, map[string]string{"tuipe": header("linux", "amd64") + "NEW"}, nil)
	_, rel := serve(t, name, archive, sum(archive)+"  "+name+"\n", true)
	c := &upgrade.Client{APIBase: "https://api.github.com", HTTP: &http.Client{}, Repo: "jiripisa/tuipe"}
	exe := installOld(t)
	if err := c.Apply(context.Background(), rel, exe, "linux", "amd64"); err == nil {
		t.Fatal("expected error")
	}
	eq(t, read(t, exe), "OLD")
}

func TestRefusesRedirectDowngradeToHTTP(t *testing.T) {
	name := upgrade.AssetName("1.1.0", "linux", "amd64")
	archive := tarGz(t, map[string]string{"tuipe": header("linux", "amd64") + "NEW"}, nil)
	hits := 0
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(archive) // a perfectly valid archive, behind the downgrade
	}))
	t.Cleanup(plain.Close)
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/s" {
			fmt.Fprintf(w, "%s  %s\n", sum(archive), name) // a valid checksum line
			return
		}
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(tls.Close)
	c := &upgrade.Client{APIBase: tls.URL, HTTP: tls.Client(), Repo: "jiripisa/tuipe"}
	rel := upgrade.Release{Tag: "v1.1.0", Assets: map[string]string{name: tls.URL + "/a", "checksums.txt": tls.URL + "/s"}}
	exe := installOld(t)
	err := c.Apply(context.Background(), rel, exe, "linux", "amd64")
	if err == nil {
		t.Fatal("expected error")
	}
	contains(t, err.Error(), "HTTPS")
	eq(t, hits, 0)
	eq(t, read(t, exe), "OLD")
}
