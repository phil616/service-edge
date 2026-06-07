package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dreamreflex/service-edge/internal/model"
)

// parseFRPDistFilename extracts version, OS and arch from a filename of the
// form frp_{version}_{os}_{arch}.tar.gz (GitHub release convention).
func parseFRPDistFilename(filename string) (version, osName, arch string, ok bool) {
	name := strings.TrimSuffix(filename, ".tar.gz")
	if name == filename {
		return "", "", "", false
	}
	parts := strings.SplitN(name, "_", 4)
	if len(parts) != 4 || parts[0] != "frp" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

// ListFRPDists returns all uploaded frp release tarballs, newest first.
func (s *Service) ListFRPDists() ([]model.FRPDistFile, error) {
	var rows []model.FRPDistFile
	if err := s.Store.DB.Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// LatestFRPDistVersion returns the highest (by semantic version) version among the
// uploaded frp release tarballs, or "" if none have been uploaded.
func (s *Service) LatestFRPDistVersion() string {
	var versions []string
	if err := s.Store.DB.Model(&model.FRPDistFile{}).Distinct("version").Pluck("version", &versions).Error; err != nil {
		return ""
	}
	best := ""
	for _, v := range versions {
		if v == "" {
			continue
		}
		if best == "" || compareFrpVersion(v, best) > 0 {
			best = v
		}
	}
	return best
}

// defaultFrpVersion is the version used when a node/host is created without an
// explicit frp version: the latest uploaded release if any, else the configured
// fallback. Preferring an actually-present release avoids defaulting to a version
// no dist exists for (the configured default may not be downloadable).
func (s *Service) defaultFrpVersion() string {
	if v := s.LatestFRPDistVersion(); v != "" {
		return v
	}
	return s.Cfg.FrpRelease.DefaultVersion
}

// compareFrpVersion compares two frp versions ("0.61.1" / "v0.61.1") numerically
// component by component. Returns >0 if a>b, <0 if a<b, 0 if equal. Missing or
// non-numeric components are treated as 0.
func compareFrpVersion(a, b string) int {
	pa := versionParts(a)
	pb := versionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

// versionParts splits "v0.61.1" into [0,61,1]; any pre-release suffix on a
// component (e.g. "1-rc1") is reduced to its leading integer.
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	fields := strings.Split(v, ".")
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		num := 0
		for i := 0; i < len(f); i++ {
			if f[i] < '0' || f[i] > '9' {
				break
			}
			num = num*10 + int(f[i]-'0')
		}
		out = append(out, num)
	}
	return out
}

// UploadFRPDist saves the tarball to disk and upserts its metadata row. The
// filename must follow the GitHub release convention frp_{version}_{os}_{arch}.tar.gz.
func (s *Service) UploadFRPDist(filename string, r io.Reader) error {
	// Strip any path components from the client-supplied name before it ever
	// reaches filepath.Join, so a crafted filename can't escape the dist dir.
	filename = filepath.Base(filepath.Clean("/" + filename))
	version, osName, arch, ok := parseFRPDistFilename(filename)
	if !ok {
		return fmt.Errorf("invalid filename %q: expected frp_{version}_{os}_{arch}.tar.gz", filename)
	}

	if err := os.MkdirAll(s.Cfg.FRPDistDir, 0o755); err != nil {
		return fmt.Errorf("create frp dist dir: %w", err)
	}

	dst := filepath.Join(s.Cfg.FRPDistDir, filename)
	tmp := dst + ".upload"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	n, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("write file: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close file: %w", closeErr)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("install file: %w", err)
	}

	row := model.FRPDistFile{
		Filename:  filename,
		Version:   version,
		OS:        osName,
		Arch:      arch,
		Size:      n,
		CreatedAt: time.Now(),
	}
	// Delete existing record (if any) then insert fresh so CreatedAt reflects upload time.
	s.Store.DB.Where("filename = ?", filename).Delete(&model.FRPDistFile{})
	return s.Store.DB.Create(&row).Error
}

// DeleteFRPDist removes the file from disk and its metadata row.
func (s *Service) DeleteFRPDist(id uint) error {
	var row model.FRPDistFile
	if err := s.Store.DB.First(&row, id).Error; err != nil {
		return ErrNotFound
	}
	path := filepath.Join(s.Cfg.FRPDistDir, row.Filename)
	_ = os.Remove(path)
	return s.Store.DB.Delete(&row).Error
}
