package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

type fakeProcesses struct {
	stopErr, disableErr error
	stopped, disabled   []string
	observe             func(context.Context, string) (bool, int, error)
}

func (f *fakeProcesses) Enable(string) error { return nil }
func (f *fakeProcesses) Disable(u string) error {
	f.disabled = append(f.disabled, u)
	return f.disableErr
}
func (f *fakeProcesses) Stop(u string) error  { f.stopped = append(f.stopped, u); return f.stopErr }
func (f *fakeProcesses) Restart(string) error { return nil }
func (f *fakeProcesses) IsActive(string) bool { return true }
func (f *fakeProcesses) MainPID(string) int   { return 123 }
func (f *fakeProcesses) ProcessStatus(ctx context.Context, u string) (bool, int, error) {
	if f.observe != nil {
		return f.observe(ctx, u)
	}
	return true, 123, nil
}

func TestRemoveConnectionPreservesOtherInstances(t *testing.T) {
	base := t.TempDir()
	for _, u := range []string{"a", "b"} {
		d, _ := frp.FRPCInstanceDir(base, u)
		if err := os.MkdirAll(filepath.Join(d, "config"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "config", "client.key"), []byte("key"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeProcesses{}
	if err := removeConnection(manager, base, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "instances", "a")); !os.IsNotExist(err) {
		t.Fatalf("instance a remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "instances", "b", "config", "client.key")); err != nil {
		t.Fatalf("sibling damaged: %v", err)
	}
	if len(manager.stopped) != 1 || manager.stopped[0] != frpcUnit("a") {
		t.Fatal(manager.stopped)
	}
}
func TestRemoveConnectionDoesNotForgetFailedStop(t *testing.T) {
	for _, failure := range []string{"stop", "disable"} {
		t.Run(failure, func(t *testing.T) {
			base := t.TempDir()
			dir, _ := frp.FRPCInstanceDir(base, "a")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			m := &fakeProcesses{}
			if failure == "stop" {
				m.stopErr = errors.New("failed")
			} else {
				m.disableErr = errors.New("failed")
			}
			if err := removeConnection(m, base, "a"); err == nil {
				t.Fatal("expected error")
			}
			if _, err := os.Stat(dir); err != nil {
				t.Fatal("files removed on failure")
			}
		})
	}
	for _, u := range []string{"", ".", "..", "../b", "a/b", `a\b`} {
		m := &fakeProcesses{}
		if err := removeConnection(m, t.TempDir(), u); err == nil {
			t.Fatalf("accepted %q", u)
		}
		if len(m.stopped) != 0 {
			t.Fatal("invalid identifier reached systemd")
		}
	}
}
func TestAppliedIdentityIncludesSharedBinaryAndCA(t *testing.T) {
	conn := protocol.ConnectionConfig{UUID: "a", ConfigVersion: 1, FrpConfig: "config", TLSCert: "cert", TLSKey: "key", AdminPort: 7400}
	state := LoadState(filepath.Join(t.TempDir(), "state.json"))
	old := connectionFingerprint(conn, "ca", "v0.61.1")
	if err := state.SetConn("a", 1, 7400, old); err != nil {
		t.Fatal(err)
	}
	loaded := LoadState(state.path).ConnEntries()["a"]
	if loaded.Fingerprint != old {
		t.Fatal("lost per-instance applied identity")
	}
	if loaded.Fingerprint == connectionFingerprint(conn, "ca", "v0.68.0") {
		t.Fatal("binary-only upgrade would be skipped")
	}
	if loaded.Fingerprint == connectionFingerprint(conn, "new-ca", "v0.61.1") {
		t.Fatal("CA-only update would be skipped")
	}
	conn.ConfigVersion++ // Display-only metadata bumps must not interrupt traffic.
	if old != connectionFingerprint(conn, "ca", "0.61.1") {
		t.Fatal("version prefix causes spurious restart")
	}
}
