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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

func (c *Client) get(ctx context.Context, url, accept string, timeout time.Duration, limit int64) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "tuipe/"+buildinfo.Version)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
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
		return nil, 0, fmt.Errorf("download exceeds %d bytes", limit)
	}
	return body, resp.StatusCode, nil
}

// Latest returns the latest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	url := c.APIBase + "/repos/" + c.Repo + "/releases/latest"
	body, status, err := c.get(ctx, url, "application/vnd.github+json", apiTimeout, 4<<20)
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
	c, _, ok := parseVersion(current)
	if !ok {
		return true
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
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
	if err := checkHeader(bin, goos); err != nil {
		return fmt.Errorf(text.UpgradeBadArchive, err)
	}
	return install(bin, exePath)
}

func (c *Client) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	body, status, err := c.get(ctx, url, "application/octet-stream", downloadTimeout, limit)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("HTTP %d", status)
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
// unsafe paths and links anywhere in the archive.
func extract(archive []byte, goos string) ([]byte, error) {
	want := binaryName(goos)
	var bin []byte
	found := false
	if goos == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if !safeName(f.Name) || f.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("unsafe entry %q", f.Name)
			}
			if f.FileInfo().IsDir() || path.Clean(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			bin, err = readLimited(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			found = true
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
				return nil, fmt.Errorf("unsafe entry %q", h.Name)
			}
			switch h.Typeflag {
			case tar.TypeReg:
			case tar.TypeDir:
				continue
			case tar.TypeXGlobalHeader:
				continue
			default:
				return nil, fmt.Errorf("unsupported entry %q (type %c)", h.Name, h.Typeflag)
			}
			if path.Clean(h.Name) != want {
				continue
			}
			bin, err = readLimited(tr)
			if err != nil {
				return nil, err
			}
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("no %s in the archive", want)
	}
	return bin, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxDownload {
		return nil, errors.New("binary too large")
	}
	return b, nil
}

// checkHeader verifies the binary is non-empty and looks like an executable
// of the platform.
func checkHeader(bin []byte, goos string) error {
	if len(bin) == 0 {
		return errors.New("the binary is empty")
	}
	var magics []string
	switch goos {
	case "windows":
		magics = []string{"MZ"}
	case "darwin":
		magics = []string{"\xfe\xed\xfa\xce", "\xfe\xed\xfa\xcf", "\xce\xfa\xed\xfe", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe"}
	default:
		magics = []string{"\x7fELF"}
	}
	for _, m := range magics {
		if bytes.HasPrefix(bin, []byte(m)) {
			return nil
		}
	}
	return fmt.Errorf("the binary does not look like a %s executable", goos)
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
		if errors.Is(err, os.ErrPermission) {
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
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	if err := swap(tmpName, real); err != nil {
		os.Remove(tmpName)
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf(text.UpgradeNotWritable, dir)
		}
		return fmt.Errorf(text.UpgradeReplaceFail, err)
	}
	return nil
}

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

// Run executes `tuipe upgrade` (or `--check`), reporting to stdout.
func Run(ctx context.Context, c *Client, exePath string, check bool, stdout io.Writer) error {
	r, err := c.Latest(ctx)
	if err != nil {
		return err
	}
	current := buildinfo.Version
	newer := Newer(current, r.Tag)
	if check {
		if newer {
			fmt.Fprintf(stdout, text.UpgradeAvailable+"\n", r.Tag, current)
		} else {
			fmt.Fprintf(stdout, text.UpgradeAlreadyLast+"\n", current)
		}
		return nil
	}
	if !buildinfo.IsRelease() {
		return errors.New(text.UpgradeNotRelease)
	}
	if !newer {
		fmt.Fprintf(stdout, text.UpgradeAlreadyLast+"\n", current)
		return nil
	}
	if err := c.Apply(ctx, r, exePath, runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	fmt.Fprintf(stdout, text.UpgradeDone+"\n", current, r.Tag)
	return nil
}
