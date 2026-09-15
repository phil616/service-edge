package agent

import (
	"context"
	"encoding/json"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigRejectsInvalidTimersAndHonorsPollTimeout(t *testing.T) {
	base := "agent_type: frpc\nuuid: host\napi_endpoint: http://localhost\napi_token: token\n"
	for _, tc := range []struct {
		extra string
		valid bool
	}{
		{"heartbeat_interval: '-1s'\n", false},
		{"status_report_interval: '-1s'\n", false},
		{"config_poll_timeout: '30s'\n", false},
		{"config_poll_timeout: '45s'\n", true},
	} {
		file := filepath.Join(t.TempDir(), "agent.yaml")
		if err := os.WriteFile(file, []byte(base+tc.extra), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(file)
		if !tc.valid {
			if err == nil {
				t.Fatal("invalid timer accepted", tc.extra)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if NewClient(cfg).pollHTTP.Timeout != 45*time.Second {
			t.Fatal("configured timeout ignored")
		}
	}
}
func TestHostStartupReconcilesEvenWhenVersionUnchanged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	versions := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/config":
			versions <- r.URL.Query().Get("current_version")
			json.NewEncoder(w).Encode(protocol.HostConfigResponse{ConfigVersion: 7})
		case "/api/v1/agent/config/ack":
			cancel()
		}
	}))
	defer srv.Close()
	state := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err := state.SaveHost(7); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{AgentType: "frpc", APIEndpoint: srv.URL}
	runner := &Runner{cfg: cfg, state: state, client: NewClient(cfg), systemd: &fakeProcesses{}}
	done := make(chan struct{})
	go func() { runner.configSyncLoop(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("startup sync stalled")
	}
	if got := <-versions; got != "0" {
		t.Fatal("startup did not request full snapshot", got)
	}
	if state.Version() != 7 {
		t.Fatal("snapshot changed applied host revision")
	}
}
