package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/model"
)

func validFRPArchive(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"frpc", "frps"} {
		body := []byte("#!/bin/sh\n")
		if err := tw.WriteHeader(&tar.Header{Name: "frp_0.71.0_linux_amd64/" + name, Mode: 0755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestUploadFRPDistValidatesArchiveAndPublishesMetadata(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg = &config.Config{}
	svc.Cfg.FRPDistDir = filepath.Join(t.TempDir(), "dist")
	if err := svc.UploadFRPDist("frp_0.71.0_linux_amd64.tar.gz", bytes.NewReader(validFRPArchive(t))); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListFRPDists()
	if err != nil || len(rows) != 1 || rows[0].SHA256 == "" {
		t.Fatalf("metadata missing: %+v %v", rows, err)
	}
	if got := svc.localFRPDist(rows[0].Filename); got == nil {
		t.Fatal("uploaded archive was not usable")
	}
}

func TestUploadFRPDistRejectsHTML(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg = &config.Config{}
	svc.Cfg.FRPDistDir = t.TempDir()
	if err := svc.UploadFRPDist("frp_0.71.0_linux_amd64.tar.gz", bytes.NewReader([]byte("<!doctype html>"))); err == nil {
		t.Fatal("HTML upload accepted")
	}
}

func TestFRPBinaryRequiresUploadedArchive(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg = &config.Config{}
	svc.Cfg.Server.ExternalURL = "https://edge.example.com"
	svc.Cfg.FRPDistDir = t.TempDir()

	got, err := svc.frpBinary("v0.71.0", "linux", "amd64")
	if err == nil || !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "administrator must upload") {
		t.Fatalf("frpBinary() error = %v, want actionable missing-upload conflict", err)
	}
	if got.DownloadURL != "" {
		t.Fatalf("frpBinary() returned download URL for missing archive: %q", got.DownloadURL)
	}
}

func TestFRPBinaryUsesUploadedArchiveOnly(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg = &config.Config{}
	svc.Cfg.Server.ExternalURL = "https://edge.example.com/"
	svc.Cfg.FRPDistDir = filepath.Join(t.TempDir(), "dist")
	if err := svc.UploadFRPDist("frp_0.71.0_linux_amd64.tar.gz", bytes.NewReader(validFRPArchive(t))); err != nil {
		t.Fatal(err)
	}

	got, err := svc.frpBinary("0.71.0", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://edge.example.com/api/v1/frp-dist/frp_0.71.0_linux_amd64.tar.gz"
	if got.DownloadURL != want || got.SHA256 == "" {
		t.Fatalf("frpBinary() = %+v, want local URL with checksum", got)
	}
}

func TestCompareFrpVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign only
	}{
		{"0.62.0", "0.61.1", 1},
		{"v0.61.1", "0.61.1", 0},
		{"0.61.1", "0.61.10", -1},
		{"0.9.0", "0.10.0", -1}, // numeric, not lexicographic
		{"1.0.0", "0.61.1", 1},
		{"0.61", "0.61.0", 0},
	}
	for _, c := range cases {
		got := compareFrpVersion(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) {
			t.Errorf("compareFrpVersion(%q,%q)=%d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLatestFRPDistVersion(t *testing.T) {
	svc := newTestService(t)
	if got := svc.LatestFRPDistVersion(); got != "" {
		t.Fatalf("empty dist: want \"\", got %q", got)
	}

	for _, v := range []string{"0.61.1", "0.62.0", "0.9.0"} {
		row := model.FRPDistFile{Filename: "frp_" + v + "_linux_amd64.tar.gz", Version: v, OS: "linux", Arch: "amd64", CreatedAt: time.Now()}
		if err := svc.Store.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed dist %s: %v", v, err)
		}
	}
	if got := svc.LatestFRPDistVersion(); got != "0.62.0" {
		t.Fatalf("want latest 0.62.0, got %q", got)
	}
}
