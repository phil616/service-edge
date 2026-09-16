package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dreamreflex/service-edge/internal/api/handler"
	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"github.com/dreamreflex/service-edge/internal/service"
	"github.com/dreamreflex/service-edge/internal/store"
)

func TestAgentIdentityAndDurableRetirement(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AgentAPIToken: "master-secret-only-on-control-plane", EnrollmentTokenTTL: config.Duration(time.Hour)}
	svc := service.New(st, nil, cfg)
	router := NewRouter(Options{Cfg: cfg, Handler: &handler.Handler{Svc: svc}, JWT: middleware.NewJWTManager("jwt", time.Hour)})
	for _, kind := range []string{"frpc", "frps"} {
		t.Run(kind, func(t *testing.T) {
			uuid := kind + "-host"
			var node any = &model.FRPCHost{UUID: uuid, ConfigVersion: 3}
			if kind == "frps" {
				node = &model.FRPSNode{UUID: uuid, ConfigVersion: 3}
			}
			if err := st.DB.Create(node).Error; err != nil {
				t.Fatal(err)
			}
			token := protocol.AgentToken(cfg.AgentAPIToken, kind, uuid)
			call := func(method, path, key, identity, role string, body any) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(body)
				req := httptest.NewRequest(method, path, bytes.NewReader(raw))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Agent-Token", key)
				req.Header.Set("X-Agent-UUID", identity)
				req.Header.Set("X-Agent-Type", role)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w
			}
			heartbeat := "/api/v1/agent/heartbeat"
			if got := call("POST", heartbeat, token, uuid, kind, protocol.HeartbeatRequest{}); got.Code != 401 {
				t.Fatal("unenrolled accepted", got.Code)
			}
			enrollment, err := svc.CreateEnrollment(kind, uuid)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				w := call("POST", "/api/v1/agent/enroll?token="+enrollment.Token, token, uuid, kind, protocol.EnrollRequest{UUID: uuid, AgentType: kind})
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if got := call("POST", heartbeat, token, uuid, kind, protocol.HeartbeatRequest{}); got.Code != 200 {
				t.Fatal(got.Code, got.Body.String())
			}
			for _, key := range []string{cfg.AgentAPIToken, protocol.AgentToken(cfg.AgentAPIToken, kind, "other"), protocol.AgentToken(cfg.AgentAPIToken, "wrong-role", uuid)} {
				if got := call("POST", heartbeat, key, uuid, kind, protocol.HeartbeatRequest{}); got.Code != 401 {
					t.Fatal("identity boundary crossed", got.Code)
				}
			}
			if kind == "frpc" {
				err = svc.DeleteFRPCHost(uuid)
			} else {
				err = svc.DeleteFRPS(uuid)
			}
			if err != nil {
				t.Fatal(err)
			}
			// A new Service instance stands in for control-plane restart. Retirement is durable.
			restarted := service.New(st, nil, cfg)
			row, err := restarted.AgentRetirement(kind, uuid)
			if err != nil || row == nil || row.ConfigVersion != 4 || row.CompletedAt != nil {
				t.Fatal(row, err)
			}
			w := call("GET", "/api/v1/agent/config?current_version=3", token, uuid, kind, nil)
			var bundle protocol.ConfigResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &bundle) != nil || !bundle.Decommission {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, ack := range []protocol.AckRequest{{ConfigVersion: 4, Error: "stop failed"}, {ConfigVersion: 4, Success: true}, {ConfigVersion: 4, Error: "late failure"}} {
				w = call("POST", "/api/v1/agent/config/ack", token, uuid, kind, ack)
				if w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
			row, err = restarted.AgentRetirement(kind, uuid)
			if err != nil || row.CompletedAt == nil || row.LastError != "" {
				t.Fatal(row, err)
			}
			if _, err := svc.ConsumeEnrollment(enrollment.Token, uuid, kind); err == nil {
				t.Fatal("deleted identity re-enrolled")
			}
		})
	}
}
