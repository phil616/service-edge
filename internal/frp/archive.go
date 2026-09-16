package frp

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ValidateReleaseArchive checks container integrity and required executables
// without executing uploaded code or extracting paths from the archive.
func ValidateReleaseArchive(path, osName string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("expected gzip FRP archive: %w", err)
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: (1 << 30) + 1}
	tr := tar.NewReader(limited)
	suffix := ""
	if osName == "windows" {
		suffix = ".exe"
	}
	found := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid FRP tar archive: %w", err)
		}
		name := filepath.Base(hdr.Name)
		if hdr.Typeflag == tar.TypeReg && hdr.Size > 0 && (name == "frpc"+suffix || name == "frps"+suffix) {
			found[name] = true
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return fmt.Errorf("truncated FRP member: %w", err)
		}
	}
	// Read the gzip trailer as tar EOF alone does not verify its CRC.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return fmt.Errorf("invalid gzip checksum: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf("FRP archive exceeds 1 GiB expanded size")
	}
	if !found["frpc"+suffix] || !found["frps"+suffix] {
		return fmt.Errorf("FRP archive must contain non-empty frpc and frps executables")
	}
	return nil
}
