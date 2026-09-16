package frp

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareBinaryPreservesInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	installed := filepath.Join(root, "frpc")
	old := []byte("#!/bin/sh\necho 1.0.0\n")
	if err := os.WriteFile(installed, old, 0755); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz := gzip.NewWriter(w)
		defer gz.Close()
		tarw := tar.NewWriter(gz)
		defer tarw.Close()
		body := []byte("#!/bin/sh\necho 2.0.0\n")
		if err := tarw.WriteHeader(&tar.Header{Name: "release/frpc", Mode: 0755, Size: int64(len(body))}); err != nil {
			t.Error(err)
			return
		}
		_, _ = tarw.Write(body)
	}))
	defer server.Close()
	if _, err := PrepareBinary(context.Background(), installed, server.URL, "v3.0.0", ""); err == nil {
		t.Fatal("wrong version accepted")
	}
	candidate, err := PrepareBinary(context.Background(), installed, server.URL, "v2.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if candidate == installed || !currentMatches(candidate, "2.0.0") || !currentMatches(installed, "1.0.0") {
		t.Fatal("installed executable was replaced")
	}
	server.Close()
	if _, err := PrepareBinary(context.Background(), installed, server.URL, "v2.0.0", ""); err != nil {
		t.Fatalf("cached release unavailable offline: %v", err)
	}
}

func TestPrepareBinaryCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := PrepareBinary(ctx, filepath.Join(t.TempDir(), "frpc"), server.URL, "v2.0.0", ""); err == nil {
		t.Fatal("cancelled download accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("download ignored cancellation")
	}
}
