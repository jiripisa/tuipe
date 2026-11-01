// Package upgrade implements `tuipe upgrade`: it fetches the latest GitHub
// release, verifies it and replaces the running binary.
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jiripisa/tuipe/internal/buildinfo"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	apiTimeout      = 10 * time.Second
	downloadTimeout = 2 * time.Minute
	maxDownload     = 100 << 20
	checksumsAsset  = "checksums.txt"
)

// Release is a published release; Assets maps asset names to download URLs.
type Release struct {
	Tag    string
	Assets map[string]string
}

// Client talks to the GitHub API and downloads release assets.
type Client struct {
	APIBase string
	HTTP    *http.Client
	Repo    string
}

// NewClient returns a client for the public tuipe repository.
func NewClient() *Client {
	return &Client{APIBase: "https://api.github.com", HTTP: &http.Client{}, Repo: "jiripisa/tuipe"}
}

// secure reports whether u may be fetched: HTTPS, or the scheme and host of
// APIBase itself (which is HTTPS in production and a local server in tests).
func (c *Client) secure(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	base, err := url.Parse(c.APIBase)
	return err == nil && u.Scheme == base.Scheme && u.Host == base.Host
}

func (c *Client) get(ctx context.Context, rawURL, accept string, timeout time.Duration, limit int64) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u, err := url.Parse(rawURL)
	if err != nil || !c.secure(u) {
		return nil, 0, fmt.Errorf(text.UpgradeInsecureURL, rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	client := *c.HTTP
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New(text.UpgradeTooManyRedirects)
		}
		if !c.secure(r.URL) {
			return fmt.Errorf(text.UpgradeInsecureURL, r.URL)
		}
		return nil
	}
	req.Header.Set("User-Agent", "tuipe/"+buildinfo.Version)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(body)) > limit {
		return nil, 0, fmt.Errorf(text.UpgradeTooBig, limit)
	}
	return body, resp.StatusCode, nil
}

// Latest returns the latest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	body, status, err := c.get(ctx, c.APIBase+"/repos/"+c.Repo+"/releases/latest", "application/vnd.github+json", apiTimeout, 4<<20)
	if err != nil {
		if ctx.Err() != nil {
			return Release{}, ctx.Err()
		}
		return Release{}, errors.New(text.UpgradeOffline)
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, errors.New(text.UpgradeNoRelease)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Release{}, errors.New(text.UpgradeRateLimited)
	default:
		return Release{}, fmt.Errorf(text.UpgradeBadResponse, status)
	}
	var raw struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Tag == "" {
		return Release{}, fmt.Errorf(text.UpgradeBadResponse, status)
	}
	r := Release{Tag: raw.Tag, Assets: map[string]string{}}
	for _, a := range raw.Assets {
		r.Assets[a.Name] = a.URL
	}
	return r, nil
}

// AssetName is the release archive name for a version (without the "v").
func AssetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("tuipe_%s_%s_%s%s", version, goos, goarch, ext)
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "tuipe.exe"
	}
	return "tuipe"
}

// parseVersion reads "vMAJOR.MINOR.PATCH[-pre][+build]".
func parseVersion(v string) (nums [3]int, prerelease, ok bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, prerelease = v[:i], true
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, false, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, false, false
		}
		nums[i] = n
	}
	return nums, prerelease, true
}

// Newer reports whether latest is a newer release than current. Prereleases
// and unparsable versions are never offered; "dev" is older than any release.
func Newer(current, latest string) bool {
	l, pre, ok := parseVersion(latest)
	if !ok || pre {
		return false
	}
	c, curPre, ok := parseVersion(current)
	if !ok {
		return true
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return curPre // 1.0.0-rc1 is older than 1.0.0
}

// Apply downloads the release archive for goos/goarch, verifies it and
// replaces the binary at exePath. Any failure leaves the old binary in place.
func (c *Client) Apply(ctx context.Context, r Release, exePath string, goos, goarch string) error {
	name := AssetName(strings.TrimPrefix(r.Tag, "v"), goos, goarch)
	archiveURL, ok := r.Assets[name]
	if !ok {
		return fmt.Errorf(text.UpgradeNoAsset, r.Tag, name)
	}
	sumsURL, ok := r.Assets[checksumsAsset]
	if !ok {
		return fmt.Errorf(text.UpgradeNoChecksum, r.Tag, name)
	}
	sums, err := c.download(ctx, sumsURL, 1<<20)
	if err != nil {
		return err
	}
	want, ok := checksumFor(string(sums), name)
	if !ok {
		return fmt.Errorf(text.UpgradeNoChecksum, r.Tag, name)
	}
	archive, err := c.download(ctx, archiveURL, maxDownload)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return errors.New(text.UpgradeChecksumFail)
	}
	bin, err := extract(archive, goos)
	if err != nil {
		return fmt.Errorf(text.UpgradeBadArchive, err)
	}
	if err := checkHeader(bin, goos, goarch); err != nil {
		return fmt.Errorf(text.UpgradeBadArchive, err)
	}
	return install(bin, exePath)
}

func (c *Client) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	body, status, err := c.get(ctx, url, "application/octet-stream", downloadTimeout, limit)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf(text.UpgradeBadStatus, status)
	}
	if err != nil {
		return nil, fmt.Errorf(text.UpgradeDownloadFail, err)
	}
	return body, nil
}

// checksumFor finds name in sha256sum-format text.
func checksumFor(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// safeName reports whether an archive entry name stays inside the archive root.
func safeName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// extract returns the single top-level binary of the archive. It rejects
// unsafe paths and links anywhere in the archive, and duplicate binaries.
func extract(archive []byte, goos string) ([]byte, error) {
	want := binaryName(goos)
	var bin []byte
	found := false
	take := func(name string, r io.Reader) error {
		if found {
			return fmt.Errorf(text.UpgradeDuplicate, want)
		}
		b, err := readLimited(r)
		if err != nil {
			return err
		}
		bin, found = b, true
		return nil
	}
	if goos == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if !safeName(f.Name) || f.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf(text.UpgradeUnsafeEntry, f.Name)
			}
			if f.FileInfo().IsDir() || path.Clean(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = take(f.Name, rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if !safeName(h.Name) {
				return nil, fmt.Errorf(text.UpgradeUnsafeEntry, h.Name)
			}
			switch h.Typeflag {
			case tar.TypeReg:
			case tar.TypeDir, tar.TypeXGlobalHeader:
				continue
			default:
				return nil, fmt.Errorf(text.UpgradeUnsupported, h.Name)
			}
			if path.Clean(h.Name) != want {
				continue
			}
			if err := take(h.Name, tr); err != nil {
				return nil, err
			}
		}
	}
	if !found {
		return nil, fmt.Errorf(text.UpgradeMissingBin, want)
	}
	return bin, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxDownload {
		return nil, fmt.Errorf(text.UpgradeTooBig, maxDownload)
	}
	return b, nil
}

// checkHeader verifies the binary is non-empty and is a real executable for
// goos/goarch, by parsing its headers.
func checkHeader(bin []byte, goos, goarch string) error {
	if len(bin) == 0 {
		return errors.New(text.UpgradeEmptyBin)
	}
	r := bytes.NewReader(bin)
	ok := false
	switch goos {
	case "windows":
		want := map[string]uint16{"amd64": pe.IMAGE_FILE_MACHINE_AMD64, "arm64": pe.IMAGE_FILE_MACHINE_ARM64}[goarch]
		if f, err := pe.NewFile(r); err == nil {
			ok = want != 0 && f.Machine == want
		}
	case "darwin":
		want := map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64}[goarch]
		if bytes.HasPrefix(bin, []byte{0xca, 0xfe, 0xba, 0xbe}) {
			if ff, err := macho.NewFatFile(r); err == nil {
				for _, a := range ff.Arches {
					ok = ok || (want != 0 && a.Cpu == want)
				}
			}
		} else if f, err := macho.NewFile(r); err == nil {
			ok = want != 0 && f.Cpu == want
		}
	default:
		want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[goarch]
		if f, err := elf.NewFile(r); err == nil {
			ok = want != 0 && f.Machine == want
		}
	}
	if !ok {
		return fmt.Errorf(text.UpgradeWrongPlatform, goos, goarch)
	}
	return nil
}

// install writes bin beside exePath (following symlinks) and swaps it in.
func install(bin []byte, exePath string) error {
	real, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	st, err := os.Stat(real)
	if err != nil {
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	dir := filepath.Dir(real)
	tmp, err := os.CreateTemp(dir, ".tuipe-new-*")
	if err != nil {
		switch {
		case errors.Is(err, syscall.EROFS):
			return fmt.Errorf(text.UpgradeReadOnly, dir)
		case errors.Is(err, os.ErrPermission) && runtime.GOOS == "windows":
			return fmt.Errorf(text.UpgradeNotWritableWin, dir)
		case errors.Is(err, os.ErrPermission):
			return fmt.Errorf(text.UpgradeNotWritable, dir)
		}
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	if _, err := tmp.Write(bin); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil { // the file being replaced is the only copy of the program
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	if err := swap(tmpName, real); err != nil {
		os.Remove(tmpName)
		var rf *restoreError
		if errors.As(err, &rf) {
			return err
		}
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	return nil
}

// restoreError reports that the old binary was moved aside and could not be
// moved back, so the install is left without its program at exe.
type restoreError struct{ old, exe string }

func (e *restoreError) Error() string { return fmt.Sprintf(text.UpgradeRestoreFail, e.old, e.exe) }

// CleanupOld removes the leftover of a previous Windows upgrade. It is
// harmless on other systems and ignores errors.
func CleanupOld(exePath string) {
	if runtime.GOOS != "windows" {
		return
	}
	if real, err := filepath.EvalSymlinks(exePath); err == nil {
		os.Remove(real + ".old")
	}
}

// Run executes `tuipe upgrade` (or `--check`), reporting to stdout. exePath
// is only used by the real upgrade.
func Run(ctx context.Context, c *Client, exePath string, check bool, stdout io.Writer) error {
	current := buildinfo.Effective()
	release := buildinfo.IsRelease()
	if !check && !release {
		return fmt.Errorf(text.UpgradeNotRelease, current)
	}
	r, err := c.Latest(ctx)
	if err != nil {
		return err
	}
	if _, _, ok := parseVersion(r.Tag); !ok {
		return fmt.Errorf(text.UpgradeBadTag, r.Tag)
	}
	newer := Newer(current, r.Tag)
	switch {
	case !newer:
		fmt.Fprintf(stdout, text.UpgradeAlreadyLast+"\n", current)
	case check && !release:
		fmt.Fprintf(stdout, text.UpgradeAvailableGoInstall+"\n", r.Tag, current)
	case check:
		fmt.Fprintf(stdout, text.UpgradeAvailable+"\n", r.Tag, current)
	default:
		if err := c.Apply(ctx, r, exePath, runtime.GOOS, runtime.GOARCH); err != nil {
			return err
		}
		fmt.Fprintf(stdout, text.UpgradeDone+"\n", current, r.Tag)
	}
	return nil
}
