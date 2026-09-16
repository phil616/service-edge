package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dreamreflex/service-edge/internal/protocol"
)

func TestReconcileRecoversOrphansAndRetriesFailedRemoval(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "instances", "orphan", "config")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// State was lost after FRP activation; inventory on disk must still be managed.
	state := LoadState(filepath.Join(root, "state.json"))
	var acks []protocol.AckRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ack protocol.AckRequest
		if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
			t.Error(err)
		}
		acks = append(acks, ack)
	}))
	defer server.Close()
	manager := &fakeProcesses{stopErr: errors.New("systemd unavailable")}
	runner := &Runner{cfg: &Config{AgentType: "frpc"}, instanceBase: root, state: state, systemd: manager, client: NewClient(&Config{APIEndpoint: server.URL})}
	bundle := &protocol.HostConfigResponse{ConfigVersion: 9, Connections: []protocol.ConnectionConfig{}}
	if runner.reconcile(context.Background(), bundle) {
		t.Fatal("failed removal acknowledged")
	}
	if state.Version() != 0 {
		t.Fatal("advanced revision on failed stop")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("lost retry inventory", err)
	}
	manager.stopErr = nil
	if !runner.reconcile(context.Background(), bundle) {
		t.Fatal("retry failed")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("orphan remains", err)
	}
	if state.Version() != 9 || len(acks) != 2 || acks[0].Success || !acks[1].Success {
		t.Fatal(state.Version(), acks)
	}
}

func TestMalformedHostSnapshotCannotDeleteInstances(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"config_version":1}`, `{"config_version":1,"connections":null}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		client := NewClient(&Config{APIEndpoint: server.URL})
		_, _, err := client.PollHostConfig(context.Background(), 0, "linux", "amd64")
		server.Close()
		if err == nil {
			t.Fatalf("accepted non-authoritative snapshot %s", body)
		}
	}
}

func TestRetirementDoesNotAcknowledgeUntilAllUnitsDisabled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "instances", "a"), 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	state := LoadState(filepath.Join(root, "state.json"))
	manager := &fakeProcesses{disableErr: errors.New("disable failed")}
	runner := &Runner{cfg: &Config{AgentType: "frpc"}, instanceBase: root, state: state, systemd: manager, client: NewClient(&Config{APIEndpoint: server.URL})}
	if runner.decommission(context.Background(), 2) || state.Version() != 0 {
		t.Fatal("partial retirement committed")
	}
	manager.disableErr = nil
	if !runner.decommission(context.Background(), 2) || state.Version() != 2 {
		t.Fatal("retirement retry failed")
	}
}
