package frp

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureBinary makes sure the frp binary (frps or frpc, inferred from the
// destination filename) at binaryPath matches wantVersion, downloading and
// extracting it from downloadURL if missing or out of date. If wantSHA256 is
// set, the downloaded tarball is verified against it.
func EnsureBinary(binaryPath, downloadURL, wantVersion, wantSHA256 string) error {
	wantBin := filepath.Base(binaryPath) // "frps" or "frpc"

	if currentMatches(binaryPath, wantVersion) {
		slog.Debug("frp binary up to date", "path", binaryPath, "version", wantVersion)
		return nil
	}

	slog.Info("downloading frp binary", "url", downloadURL, "dest", binaryPath)
	tmp, err := os.CreateTemp("", "frp-*.tar.gz")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := download(downloadURL, tmp); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	if wantSHA256 != "" {
		if err := verifySHA256(tmpPath, wantSHA256); err != nil {
			return err
		}
	}

	if err := extractBinary(tmpPath, wantBin, binaryPath); err != nil {
		return err
	}
	slog.Info("frp binary installed", "path", binaryPath)
	return nil
}

func currentMatches(binaryPath, wantVersion string) bool {
	if _, err := os.Stat(binaryPath); err != nil {
		return false
	}
	if wantVersion == "" {
		return true
	}
	v := FrpVersion(binaryPath)
	want := strings.TrimPrefix(wantVersion, "v")
	return strings.Contains(v, want)
}

func download(url string, dst io.Writer) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return fmt.Errorf("write tarball: %w", err)
	}
	return nil
}

func verifySHA256(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}
	return nil
}

// extractBinary pulls the frps/frpc executable out of the release tarball and
// atomically installs it at destPath with 0755.
func extractBinary(tarballPath, wantBin, destPath string) error {
	f, err := os.Open(tarballPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != wantBin {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}
		tmp := destPath + ".download"
		if err := writeReaderToFile(tmp, tr, 0o755); err != nil {
			os.Remove(tmp)
			return err
		}
		return os.Rename(tmp, destPath)
	}
	return fmt.Errorf("binary %q not found in tarball", wantBin)
}

// writeReaderToFile copies r into a new file at path with the given mode. The
// close (flush) error is surfaced via the named return so a failed final write
// can't silently leave a truncated binary in place. If the copy itself fails,
// that error wins and the close error is dropped (the file is discarded anyway).
func writeReaderToFile(path string, r io.Reader, mode os.FileMode) (err error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, r)
	return err
}
