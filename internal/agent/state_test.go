package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePersistenceFailureDoesNotAdvanceAppliedVersion(t *testing.T) {
	dir := t.TempDir()
	block := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(block, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	s := LoadState(filepath.Join(block, "state.json"))
	if err := s.Save(2, "v2"); err == nil {
		t.Fatal("expected failure")
	}
	if s.Version() != 0 || s.FrpVersion != "" {
		t.Fatal("failed save advanced in-memory state")
	}
	if err := s.SaveHost(3); err == nil {
		t.Fatal("expected failure")
	}
	if s.Version() != 0 {
		t.Fatal("failed host save advanced version")
	}
	if err := s.SetConn("a", 2, 7400, "hash"); err == nil {
		t.Fatal("expected failure")
	}
	if s.HasConn("a") {
		t.Fatal("failed connection save advanced state")
	}
}
func TestPartialBundleProgressSurvivesRestart(t *testing.T) {
	s := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err := s.SaveHost(1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetConn("a", 2, 7400, "hash2"); err != nil {
		t.Fatal(err)
	}
	reloaded := LoadState(s.path)
	if reloaded.Version() != 1 || reloaded.ConnVersion("a") != 2 {
		t.Fatal("partial progress lost or host prematurely advanced")
	}
	if err := s.RemoveConn("a"); err != nil {
		t.Fatal(err)
	}
	if LoadState(s.path).HasConn("a") {
		t.Fatal("deleted connection restored after restart")
	}
}
