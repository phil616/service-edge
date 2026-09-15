package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func adminPort(t *testing.T, s *httptest.Server) int {
	t.Helper()
	return s.Listener.Addr().(*net.TCPAddr).Port
}
func TestAdminFailureIsNotEmptySuccess(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
			defer srv.Close()
			runner := &Runner{cfg: &Config{APIToken: "token"}}
			if _, err := runner.queryProxyStatusesFor(context.Background(), "a", adminPort(t, srv)); err == nil {
				t.Fatal("lost admin API error")
			}
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tcp":[{"name":"ssh","status":"running"}]}`)
	}))
	defer srv.Close()
	r := &Runner{cfg: &Config{APIToken: "token"}}
	statuses, err := r.queryProxyStatusesFor(context.Background(), "a", adminPort(t, srv))
	if err != nil || len(statuses) != 1 || statuses[0].Status != "running" {
		t.Fatalf("%v %v", statuses, err)
	}
}
func TestCollectionDeadlineProducesPartialStatus(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tcp":[{"name":"ssh","status":"running"}]}`)
	}))
	defer healthy.Close()
	s := LoadState(filepath.Join(t.TempDir(), "state.json"))
	for _, u := range []string{"healthy", "slow1", "slow2"} {
		if err := s.SetConn(u, 1, adminPort(t, healthy), "hash"); err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeProcesses{observe: func(ctx context.Context, u string) (bool, int, error) {
		if u == frpcUnit("healthy") {
			return true, 42, nil
		}
		<-ctx.Done()
		return false, 0, ctx.Err()
	}}
	runner := &Runner{cfg: &Config{}, state: s, systemd: manager}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	statuses := runner.collectConnections(ctx)
	if len(statuses) != 3 {
		t.Fatal(statuses)
	}
	for _, cs := range statuses {
		if cs.UUID == "healthy" {
			if !cs.ProxyStatusAvailable {
				t.Fatal(cs)
			}
		} else if cs.StatusError == "" || cs.ProcessStatusAvailable {
			t.Fatal("slow instance not reported as unknown", cs)
		}
	}
}
func TestReportUploadsAfterCollectionTimesOut(t *testing.T) {
	received := make(chan protocol.StatusRequest, 1)
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req protocol.StatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		received <- req
	}))
	defer control.Close()
	cfg := &Config{APIEndpoint: control.URL, AgentType: "frpc"}
	s := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err := s.SetConn("slow", 1, 7400, "hash"); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{cfg: cfg, state: s, client: NewClient(cfg), systemd: &fakeProcesses{observe: func(ctx context.Context, u string) (bool, int, error) { <-ctx.Done(); return false, 0, ctx.Err() }}}
	runner.reportConnections(context.Background(), true)
	select {
	case req := <-received:
		if len(req.Connections) != 1 || req.Connections[0].StatusError == "" || !req.ConnectionsOnly {
			t.Fatal(req)
		}
	default:
		t.Fatal("collection timeout suppressed upload")
	}
}
